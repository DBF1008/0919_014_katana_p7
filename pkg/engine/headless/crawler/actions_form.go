package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// FillFormExecutor handles ActionTypeFillForm: it fills and submits the form
// carried by the action via the automatic form-fill logic, then waits for
// the page to settle.
type FillFormExecutor struct{}

func (e *FillFormExecutor) Type() types.ActionType { return types.ActionTypeFillForm }

func (e *FillFormExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	if err := c.processForm(page, action.Form); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}
