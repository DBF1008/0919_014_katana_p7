package crawler

import (
	"context"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// KeyPressExecutor handles ActionTypeKeyPress. action.Input carries either a
// single key ("Enter", "a") or a chord joined with "+" ("Control+Enter").
// When the action carries an element, the element is focused first.
type KeyPressExecutor struct{}

func (e *KeyPressExecutor) Type() types.ActionType { return types.ActionTypeKeyPress }

func (e *KeyPressExecutor) Execute(_ context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	keys, err := parseKeyChord(action.Input)
	if err != nil {
		return err
	}

	if action.Element != nil && action.Element.XPath != "" {
		element, err := resolveElement(c, page, action)
		if err != nil {
			return err
		}
		if err := element.Timeout(c.options.PageMaxTimeout).Focus(); err != nil {
			return err
		}
	}

	if err := page.KeyActions().Press(keys...).Do(); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}
