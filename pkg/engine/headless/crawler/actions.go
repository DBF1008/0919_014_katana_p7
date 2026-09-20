package crawler

import (
	"strconv"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// ErrElementNotVisible is returned when an element cannot be interacted with
// because it is not visible or is covered by another element.
var ErrElementNotVisible = errors.New("element not visible")

// registerDefaultActionExecutors installs the built-in executors.
func registerDefaultActionExecutors(registry *ActionRegistry) {
	loadURL := &LoadURLExecutor{}
	registry.Register(loadURL)
	registry.RegisterType(types.ActionTypeNavigate, &NavigateExecutor{})

	click := &ClickExecutor{}
	registry.RegisterType(types.ActionTypeClick, click)
	registry.RegisterType(types.ActionTypeLeftClick, click)
	registry.RegisterType(types.ActionTypeLeftClickDown, click)

	registry.Register(&ScrollExecutor{})
	registry.Register(&WaitVisibleExecutor{})
	registry.Register(&WaitHiddenExecutor{})
	registry.Register(&KeyPressExecutor{})
	registry.Register(&FillFormExecutor{})
}

// resolveClickable locates the action element on page, scrolls it into view
// and verifies it is visible and interactable.
func resolveClickable(c *Crawler, page *browser.BrowserPage, action *types.Action) (*rod.Element, error) {
	if action.Element == nil || action.Element.XPath == "" {
		return nil, errors.New("click action requires an element")
	}
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	element, err := pTimeout.ElementX(action.Element.XPath)
	if err != nil {
		return nil, err
	}

	elementTimeout := element.Timeout(c.options.PageMaxTimeout)
	if err := elementTimeout.ScrollIntoView(); err != nil {
		return nil, err
	}
	visible, err := element.Visible()
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrElementNotVisible
	}

	interactable, err := element.Interactable()
	if err != nil {
		var ce *rod.CoveredError
		if errors.As(err, &ce) {
			return nil, ErrElementNotVisible
		}
		return nil, err
	}
	if interactable == nil {
		return nil, ErrElementNotVisible
	}
	return element, nil
}

// resolveElement locates the action element without visibility checks, used
// by wait actions that poll for visibility themselves.
func resolveElement(c *Crawler, page *browser.BrowserPage, action *types.Action) (*rod.Element, error) {
	if action.Element == nil || action.Element.XPath == "" {
		return nil, errors.New("action requires an element")
	}
	return page.Timeout(c.options.PageMaxTimeout).ElementX(action.Element.XPath)
}

// parseScrollOffset parses a vertical scroll delta (in CSS pixels) from
// action.Input. Empty or unparsable input yields the default 800 pixels.
func parseScrollOffset(raw string) float64 {
	const defaultOffset = 800
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultOffset
	}
	offset, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultOffset
	}
	return offset
}

// parseKeyChord parses action.Input into one or more rod keys. Keys in a
// chord are separated by "+" (e.g. "Control+Enter"). Common aliases are
// normalised to rod's key names.
func parseKeyChord(raw string) ([]input.Key, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("key press action requires an input key")
	}
	parts := strings.Split(raw, "+")
	keys := make([]input.Key, 0, len(parts))
	for _, part := range parts {
		key, err := parseKey(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

var keyAliases = map[string]input.Key{
	"enter":     input.Enter,
	"return":    input.Enter,
	"tab":       input.Tab,
	"escape":    input.Escape,
	"esc":       input.Escape,
	"space":     input.Space,
	"backspace": input.Backspace,
	"delete":    input.Delete,
	"del":       input.Delete,
	"up":        input.ArrowUp,
	"down":      input.ArrowDown,
	"left":      input.ArrowLeft,
	"right":     input.ArrowRight,
	"ctrl":      input.ControlLeft,
	"control":   input.ControlLeft,
	"shift":     input.ShiftLeft,
	"alt":       input.AltLeft,
	"meta":      input.MetaLeft,
}

func parseKey(token string) (input.Key, error) {
	if token == "" {
		return input.Key(0), errors.New("empty key in chord")
	}
	if key, ok := keyAliases[strings.ToLower(token)]; ok {
		return key, nil
	}
	// Single characters map directly to themselves in rod's key table.
	if len([]rune(token)) == 1 {
		return input.Key([]rune(strings.ToLower(token))[0]), nil
	}
	return input.Key(0), errors.Errorf("unsupported key: %s", token)
}
