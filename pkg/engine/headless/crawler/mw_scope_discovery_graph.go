package crawler

import (
	"log/slog"

	"github.com/go-rod/rod/lib/proto"
	"github.com/projectdiscovery/katana/pkg/engine/headless/crawler/diagnostics"
)

// scopeMiddleware builds the post-action page state and enforces the scope
// policy. Out-of-scope pages skip discovery: if the queue is also empty the
// crawl terminates with ErrNoCrawlingAction, otherwise processing simply
// ends for the current action.
func (c *Crawler) scopeMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		pageState, err := newPageState(step.Page, step.Action)
		if err != nil {
			return err
		}
		if c.diagnostics != nil {
			if err := c.diagnostics.LogPageState(pageState, diagnostics.PostActionPageState); err != nil {
				return err
			}
		}
		pageState.OriginID = step.CurrentPageHash
		step.PageState = pageState

		if c.options.ScopeValidator != nil && !c.options.ScopeValidator(pageState.URL) {
			c.logger.Debug("Skipping navigation collection - current page is out of scope",
				slog.String("url", pageState.URL),
			)
			if c.crawlQueue.Size() == 0 {
				return ErrNoCrawlingAction
			}
			return nil
		}
		return next(step)
	}
}

// discoveryMiddleware extracts navigations from the post-action page,
// records diagnostics, deduplicates them against previously offered actions,
// skips logout links, and enqueues the rest.
func (c *Crawler) discoveryMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		navigations, err := step.Page.FindNavigations()
		if err != nil {
			return err
		}
		step.Navigations = navigations

		if c.diagnostics != nil {
			screenshotState, err := step.Page.Screenshot(false, &proto.PageCaptureScreenshot{
				Format: proto.PageCaptureScreenshotFormatPng,
			})
			if err != nil {
				c.logger.Error("Failed to take screenshot", slog.String("error", err.Error()))
			}
			if err := c.diagnostics.LogPageStateScreenshot(step.PageState.UniqueID, screenshotState); err != nil {
				c.logger.Error("Failed to log page state screenshot", slog.String("error", err.Error()))
			}
			if err := c.diagnostics.LogNavigations(step.PageState.UniqueID, navigations); err != nil {
				c.logger.Error("Failed to log navigations", slog.String("error", err.Error()))
			}
		}

		for _, nav := range navigations {
			actionHash := nav.Hash()
			if _, ok := c.uniqueActions[actionHash]; ok {
				continue
			}
			c.uniqueActions[actionHash] = struct{}{}

			if nav.Element != nil && isLogoutPage(nav.Element) {
				c.logger.Debug("Skipping Found logout page",
					slog.String("url", nav.Element.Attributes["href"]),
				)
				continue
			}
			nav.OriginID = step.PageState.UniqueID

			c.logger.Debug("Got new navigation", slog.Any("navigation", nav))
			if err := c.crawlQueue.Offer(nav); err != nil {
				return err
			}
		}
		return next(step)
	}
}

// graphMiddleware records the reached page state in the crawl graph. When no
// navigations were discovered and the queue is drained it terminates the
// crawl with ErrNoCrawlingAction.
func (c *Crawler) graphMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		if err := c.crawlGraph.AddPageState(*step.PageState); err != nil {
			return err
		}
		if len(step.Navigations) == 0 && c.crawlQueue.Size() == 0 {
			return ErrNoCrawlingAction
		}
		return next(step)
	}
}
