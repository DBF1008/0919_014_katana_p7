package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// CrawlStep carries the per-action state flowing through the crawl
// middleware chain. Middlewares read and mutate it as the action is
// processed.
type CrawlStep struct {
	// Ctx bounds the whole crawl, including every browser operation.
	Ctx context.Context
	// Action is the crawl action currently being processed.
	Action *types.Action
	// Page is the browser page the action is executed against.
	Page *browser.BrowserPage

	// CurrentPageHash is the hash of the page at the start of the step,
	// refreshed after any origin restoration.
	CurrentPageHash string
	// PageState is built after the action, once the post-action DOM is
	// available. Nil until the Scope middleware runs.
	PageState *types.PageState
	// Navigations are the actions discovered on the post-action page.
	Navigations []*types.Action
}

// CrawlHandler is one link in the crawl middleware chain.
type CrawlHandler func(step *CrawlStep) error

// CrawlMiddleware wraps a handler with pre/post processing, mirroring the
// engine/parser ResponseParser registration style: each cross-cutting
// concern (captcha, auth, scope, discovery, graph) is an independently
// composable unit instead of a linear block inside crawlFn.
type CrawlMiddleware interface {
	Name() string
	Wrap(next CrawlHandler) CrawlHandler
}

// CrawlMiddlewareFunc adapts a plain wrapper function into a CrawlMiddleware.
type CrawlMiddlewareFunc struct {
	name    string
	wrapper func(CrawlHandler) CrawlHandler
}

// NewCrawlMiddleware returns a named CrawlMiddleware from a wrapper func.
func NewCrawlMiddleware(name string, wrapper func(CrawlHandler) CrawlHandler) CrawlMiddleware {
	return &CrawlMiddlewareFunc{name: name, wrapper: wrapper}
}

func (m *CrawlMiddlewareFunc) Name() string { return m.name }

func (m *CrawlMiddlewareFunc) Wrap(next CrawlHandler) CrawlHandler {
	return m.wrapper(next)
}

// BuildCrawlChain composes middlewares around terminal in the declared
// order. The first middleware in the slice is the outermost handler, i.e.
// it runs first before the action is executed.
func BuildCrawlChain(terminal CrawlHandler, middlewares ...CrawlMiddleware) CrawlHandler {
	handler := terminal
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i].Wrap(handler)
	}
	return handler
}

// defaultCrawlMiddlewares is the standard crawl pipeline:
//
//	action    – restore origin page + dispatch the action (executors)
//	captcha   – detect/solve captchas, stop the chain on a captcha page
//	auth      – attempt automatic login when enabled
//	scope     – build post-action page state and enforce scope
//	discovery – find navigations and enqueue new actions
//	graph     – record the page state in the crawl graph
func (c *Crawler) defaultCrawlMiddlewares() []CrawlMiddleware {
	return []CrawlMiddleware{
		NewCrawlMiddleware("action", c.actionMiddleware),
		NewCrawlMiddleware("captcha", c.captchaMiddleware),
		NewCrawlMiddleware("auth", c.authMiddleware),
		NewCrawlMiddleware("scope", c.scopeMiddleware),
		NewCrawlMiddleware("discovery", c.discoveryMiddleware),
		NewCrawlMiddleware("graph", c.graphMiddleware),
	}
}
