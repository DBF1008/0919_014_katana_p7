package crawler

import (
	"context"
	"log/slog"

	"github.com/go-rod/rod/lib/proto"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/crawler/diagnostics"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// CrawlContext carries the per-action state through the crawl middleware
// chain. Middlewares may read fields set by earlier middlewares (e.g.
// PageState is populated by ScopeMiddleware and consumed by
// DiscoveryMiddleware and GraphMiddleware).
type CrawlContext struct {
	Context         context.Context
	Action          *types.Action
	Page            *browser.BrowserPage
	CurrentPageHash string
	PageState       *types.PageState
	Navigations     []*types.Action
}

// CrawlHandler is a single step in the crawl pipeline.
type CrawlHandler func(c *Crawler, cc *CrawlContext) error

// CrawlMiddleware wraps a CrawlHandler with additional behavior, mirroring
// the composable style of Hooks: each middleware may run logic before and/or
// after the rest of the chain, or short-circuit it by not calling next.
type CrawlMiddleware func(next CrawlHandler) CrawlHandler

// Chain composes middlewares into a single middleware that runs them in
// order: the first middleware in the list is the outermost one.
func Chain(middlewares ...CrawlMiddleware) CrawlMiddleware {
	return func(final CrawlHandler) CrawlHandler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// DefaultCrawlMiddlewares returns the standard crawl pipeline in execution
// order: CaptchaMiddleware → AuthMiddleware → ScopeMiddleware →
// DiscoveryMiddleware → GraphMiddleware. Callers may reorder, extend, or
// trim this list to customize the pipeline before building a handler.
func DefaultCrawlMiddlewares() []CrawlMiddleware {
	return []CrawlMiddleware{
		CaptchaMiddleware(),
		AuthMiddleware(),
		ScopeMiddleware(),
		DiscoveryMiddleware(),
		GraphMiddleware(),
	}
}

// crawlPipeline builds the crawl handler from the default middleware chain
// terminated by terminalCrawlHandler.
func (c *Crawler) crawlPipeline() CrawlHandler {
	return Chain(DefaultCrawlMiddlewares()...)(terminalCrawlHandler)
}

// terminalCrawlHandler is the innermost handler of the chain. It reports
// ErrNoCrawlingAction when neither this step nor the queue hold any further
// work.
func terminalCrawlHandler(c *Crawler, cc *CrawlContext) error {
	if len(cc.Navigations) == 0 && c.crawlQueue.Size() == 0 {
		return ErrNoCrawlingAction
	}
	return nil
}

// CaptchaMiddleware checks for captcha pages after navigation and attempts
// to solve them. When a captcha is handled, the chain is short-circuited:
// navigation discovery is skipped because the discovered links/forms belong
// to the captcha widget, not the real page.
func CaptchaMiddleware() CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			if c.options.CaptchaHandler != nil {
				html, htmlErr := cc.Page.HTML()
				if htmlErr == nil {
					handled, solveErr := c.options.CaptchaHandler.HandleIfCaptcha(cc.Context, cc.Page.Page, html)
					if solveErr != nil {
						gologger.Warning().Msgf("captcha solving failed: %s", solveErr)
					}
					if handled && solveErr == nil {
						_ = cc.Page.WaitPageLoadHeurisitics()
					}
					if handled {
						return nil
					}
				}
			}
			return next(c, cc)
		}
	}
}

// AuthMiddleware attempts automatic login when credentials and a page
// classifier are configured and the crawler has not logged in yet.
func AuthMiddleware() CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			if !c.loggedIn && c.options.AuthUsername != "" && c.options.DitClassifier != nil {
				if info, err := cc.Page.Info(); err == nil && (c.options.ScopeValidator == nil || c.options.ScopeValidator(info.URL)) {
					if html, htmlErr := cc.Page.HTML(); htmlErr == nil {
						if c.tryAutoLogin(cc.Page, html) {
							_ = cc.Page.WaitPageLoadHeurisitics()
						}
					}
				}
			}
			return next(c, cc)
		}
	}
}

// ScopeMiddleware builds the post-action page state and skips the rest of
// the chain when the current page falls outside the crawl scope.
func ScopeMiddleware() CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			pageState, err := newPageState(cc.Page, cc.Action)
			if err != nil {
				return err
			}
			if c.diagnostics != nil {
				if err := c.diagnostics.LogPageState(pageState, diagnostics.PostActionPageState); err != nil {
					return err
				}
			}
			pageState.OriginID = cc.CurrentPageHash
			cc.PageState = pageState

			if c.options.ScopeValidator != nil && !c.options.ScopeValidator(pageState.URL) {
				c.logger.Debug("Skipping navigation collection - current page is out of scope",
					slog.String("url", pageState.URL),
				)
				if c.crawlQueue.Size() == 0 {
					return ErrNoCrawlingAction
				}
				return nil
			}
			return next(c, cc)
		}
	}
}

// DiscoveryMiddleware collects new navigations from the current page and
// enqueues the ones not seen before, skipping logout links.
func DiscoveryMiddleware() CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			navigations, err := cc.Page.FindNavigations()
			if err != nil {
				return err
			}
			cc.Navigations = navigations

			// Log navigations for diagnostics
			if c.diagnostics != nil {
				screenshotState, err := cc.Page.Screenshot(false, &proto.PageCaptureScreenshot{
					Format: proto.PageCaptureScreenshotFormatPng,
				})
				if err != nil {
					c.logger.Error("Failed to take screenshot", slog.String("error", err.Error()))
				}
				if err := c.diagnostics.LogPageStateScreenshot(cc.PageState.UniqueID, screenshotState); err != nil {
					c.logger.Error("Failed to log page state screenshot", slog.String("error", err.Error()))
				}
				if err := c.diagnostics.LogNavigations(cc.PageState.UniqueID, navigations); err != nil {
					c.logger.Error("Failed to log navigations", slog.String("error", err.Error()))
				}
			}

			for _, nav := range navigations {
				actionHash := nav.Hash()
				if _, ok := c.uniqueActions[actionHash]; ok {
					continue
				}
				c.uniqueActions[actionHash] = struct{}{}

				// Check if the element we have is a logout page
				if nav.Element != nil && isLogoutPage(nav.Element) {
					c.logger.Debug("Skipping Found logout page",
						slog.String("url", nav.Element.Attributes["href"]),
					)
					continue
				}
				nav.OriginID = cc.PageState.UniqueID

				c.logger.Debug("Got new navigation",
					slog.Any("navigation", nav),
				)
				if err := c.crawlQueue.Offer(nav); err != nil {
					return err
				}
			}
			return next(c, cc)
		}
	}
}

// GraphMiddleware records the current page state as a vertex of the crawl
// graph.
func GraphMiddleware() CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			if err := c.crawlGraph.AddPageState(*cc.PageState); err != nil {
				return err
			}
			return next(c, cc)
		}
	}
}
