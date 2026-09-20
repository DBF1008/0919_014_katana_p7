package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// ScrollExecutor handles ActionTypeScroll. Without an element it scrolls the
// page down by the vertical delta in action.Input (default 800 CSS pixels);
// with an element it scrolls the element into view.
type ScrollExecutor struct{}

func (e *ScrollExecutor) Type() types.ActionType { return types.ActionTypeScroll }

func (e *ScrollExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	if action.Element != nil && action.Element.XPath != "" {
		element, err := resolveElement(c, page, action)
		if err != nil {
			return err
		}
		if err := element.Timeout(c.options.PageMaxTimeout).ScrollIntoView(); err != nil {
			return err
		}
	} else {
		offset := parseScrollOffset(action.Input)
		if err := page.Mouse.Scroll(0, offset, 1); err != nil {
			return err
		}
	}
	return page.WaitPageLoadHeurisitics()
}
