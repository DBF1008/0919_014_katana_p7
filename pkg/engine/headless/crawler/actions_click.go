package crawler

import (
	"context"

	"github.com/go-rod/rod/lib/proto"
	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// ClickExecutor handles generic and left-button click actions. It resolves
// the element (visibility + interactability), clicks it, and waits for the
// page to settle.
type ClickExecutor struct{}

func (e *ClickExecutor) Type() types.ActionType { return types.ActionTypeClick }

func (e *ClickExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	element, err := resolveClickable(c, page, action)
	if err != nil {
		return err
	}
	if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}
