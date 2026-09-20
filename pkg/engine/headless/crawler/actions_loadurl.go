package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// navigateToURL performs a top-level navigation to url and waits for the
// page to settle. It is shared by the LoadURL and Navigate executors.
func navigateToURL(c *Crawler, page *browser.BrowserPage, url string) error {
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	if err := pTimeout.Navigate(url); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}

// LoadURLExecutor handles ActionTypeLoadURL: the top-level initial load of a
// target URL into the current page.
type LoadURLExecutor struct{}

func (e *LoadURLExecutor) Type() types.ActionType { return types.ActionTypeLoadURL }

func (e *LoadURLExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	return navigateToURL(c, page, action.Input)
}

// NavigateExecutor handles ActionTypeNavigate: an in-session navigation to
// an explicit URL (history traversal / SPA-hostile redirect), distinct from
// the seed LoadURL action.
type NavigateExecutor struct{}

func (e *NavigateExecutor) Type() types.ActionType { return types.ActionTypeNavigate }

func (e *NavigateExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	return navigateToURL(c, page, action.Input)
}
