package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// WaitVisibleExecutor handles ActionTypeWaitVisible: it resolves the action
// element and blocks until the element becomes visible.
type WaitVisibleExecutor struct{}

func (e *WaitVisibleExecutor) Type() types.ActionType { return types.ActionTypeWaitVisible }

func (e *WaitVisibleExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	element, err := resolveElement(c, page, action)
	if err != nil {
		return err
	}
	return element.Timeout(c.options.PageMaxTimeout).WaitVisible()
}

// WaitHiddenExecutor handles ActionTypeWaitHidden: it resolves the action
// element and blocks until the element becomes hidden or is removed.
type WaitHiddenExecutor struct{}

func (e *WaitHiddenExecutor) Type() types.ActionType { return types.ActionTypeWaitHidden }

func (e *WaitHiddenExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	element, err := resolveElement(c, page, action)
	if err != nil {
		return err
	}
	return element.Timeout(c.options.PageMaxTimeout).WaitInvisible()
}
