package crawler

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// ActionExecutor executes a single crawl action type. Each action type is
// handled by its own executor implementation (Command pattern), so adding a
// new action type only requires registering a new executor — the core
// dispatch logic in Crawler.dispatchCrawlAction stays untouched.
type ActionExecutor interface {
	// Type returns the primary action type this executor handles.
	Type() types.ActionType
	// Execute performs the action on the given page.
	Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error
}

// ActionRegistry maps action types to their executors.
type ActionRegistry struct {
	executors map[types.ActionType]ActionExecutor
}

// NewActionRegistry returns a registry pre-populated with the default
// executors for all supported action types.
func NewActionRegistry() *ActionRegistry {
	registry := &ActionRegistry{executors: make(map[types.ActionType]ActionExecutor)}
	registry.Register(LoadURLExecutor{})
	registry.Register(NavigateExecutor{})
	registry.Register(ClickExecutor{}, types.ActionTypeLeftClick, types.ActionTypeLeftClickDown)
	registry.Register(ScrollExecutor{})
	registry.Register(WaitVisibleExecutor{})
	registry.Register(WaitHiddenExecutor{})
	registry.Register(KeyPressExecutor{})
	registry.Register(FillFormExecutor{})
	return registry
}

// Register adds an executor to the registry under its primary type and any
// additional alias types. Registering an executor for an already-registered
// type replaces the previous executor.
func (r *ActionRegistry) Register(executor ActionExecutor, aliases ...types.ActionType) {
	r.executors[executor.Type()] = executor
	for _, alias := range aliases {
		r.executors[alias] = executor
	}
}

// Lookup returns the executor registered for the given action type.
func (r *ActionRegistry) Lookup(actionType types.ActionType) (ActionExecutor, bool) {
	executor, ok := r.executors[actionType]
	return executor, ok
}

// navigateAndWait navigates the page to the action input URL and waits for
// the page to settle. Shared by LoadURLExecutor and NavigateExecutor.
func navigateAndWait(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	if err := pTimeout.Navigate(action.Input); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}

// LoadURLExecutor handles types.ActionTypeLoadURL: the initial page load.
type LoadURLExecutor struct{}

func (LoadURLExecutor) Type() types.ActionType { return types.ActionTypeLoadURL }

func (LoadURLExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	return navigateAndWait(c, action, page)
}

// NavigateExecutor handles types.ActionTypeNavigate: navigation to a URL
// after the initial load.
type NavigateExecutor struct{}

func (NavigateExecutor) Type() types.ActionType { return types.ActionTypeNavigate }

func (NavigateExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	return navigateAndWait(c, action, page)
}

// ClickExecutor handles types.ActionTypeClick (and the legacy
// types.ActionTypeLeftClick / types.ActionTypeLeftClickDown aliases): it
// scrolls the target element into view, verifies it is visible and
// interactable, and left-clicks it.
type ClickExecutor struct{}

func (ClickExecutor) Type() types.ActionType { return types.ActionTypeClick }

func (ClickExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	element, err := pTimeout.ElementX(action.Element.XPath)
	if err != nil {
		return err
	}

	elementTimeout := element.Timeout(c.options.PageMaxTimeout)
	if err := elementTimeout.ScrollIntoView(); err != nil {
		return err
	}
	visible, err := element.Visible()
	if err != nil {
		return err
	}
	if !visible {
		return ErrElementNotVisible
	}

	// Check if element is interactable (not blocked by overlays)
	interactable, err := element.Interactable()
	if err != nil {
		var ce *rod.CoveredError
		if errors.As(err, &ce) {
			return ErrElementNotVisible
		}
		return err
	}
	if interactable == nil {
		return ErrElementNotVisible
	}

	if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}

// resolveActionElement locates the action's target element, preferring its
// XPath and falling back to its CSS selector.
func resolveActionElement(c *Crawler, action *types.Action, page *browser.BrowserPage) (*rod.Element, error) {
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	if action.Element != nil {
		if action.Element.XPath != "" {
			return pTimeout.ElementX(action.Element.XPath)
		}
		if action.Element.CSSSelector != "" {
			return pTimeout.Element(action.Element.CSSSelector)
		}
	}
	return nil, fmt.Errorf("action %s has no element selector", action.Type)
}

// ScrollExecutor handles types.ActionTypeScroll: it scrolls the target
// element into view, or scrolls the window one viewport down when the
// action carries no element.
type ScrollExecutor struct{}

func (ScrollExecutor) Type() types.ActionType { return types.ActionTypeScroll }

func (ScrollExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	if action.Element != nil && (action.Element.XPath != "" || action.Element.CSSSelector != "") {
		element, err := resolveActionElement(c, action, page)
		if err != nil {
			return err
		}
		return element.Timeout(c.options.PageMaxTimeout).ScrollIntoView()
	}
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	_, err := pTimeout.Eval(`() => window.scrollBy(0, window.innerHeight)`)
	return err
}

// WaitVisibleExecutor handles types.ActionTypeWaitVisible: it blocks until
// the target element becomes visible.
type WaitVisibleExecutor struct{}

func (WaitVisibleExecutor) Type() types.ActionType { return types.ActionTypeWaitVisible }

func (WaitVisibleExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	element, err := resolveActionElement(c, action, page)
	if err != nil {
		return err
	}
	return element.Timeout(c.options.PageMaxTimeout).WaitVisible()
}

// WaitHiddenExecutor handles types.ActionTypeWaitHidden: it blocks until the
// target element becomes invisible.
type WaitHiddenExecutor struct{}

func (WaitHiddenExecutor) Type() types.ActionType { return types.ActionTypeWaitHidden }

func (WaitHiddenExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	element, err := resolveActionElement(c, action, page)
	if err != nil {
		return err
	}
	return element.Timeout(c.options.PageMaxTimeout).WaitInvisible()
}

// namedKeys maps case-insensitive key names to rod input keys.
var namedKeys = map[string]input.Key{
	"enter":      input.Enter,
	"return":     input.Enter,
	"tab":        input.Tab,
	"escape":     input.Escape,
	"esc":        input.Escape,
	"backspace":  input.Backspace,
	"delete":     input.Delete,
	"space":      input.Space,
	"home":       input.Home,
	"end":        input.End,
	"pageup":     input.PageUp,
	"pagedown":   input.PageDown,
	"arrowup":    input.ArrowUp,
	"arrowdown":  input.ArrowDown,
	"arrowleft":  input.ArrowLeft,
	"arrowright": input.ArrowRight,
	"up":         input.ArrowUp,
	"down":       input.ArrowDown,
	"left":       input.ArrowLeft,
	"right":      input.ArrowRight,
}

// parseKey resolves a key name ("enter", "ArrowDown", ...) or a single
// character ("a", "1", ...) to a rod input key.
func parseKey(name string) (input.Key, error) {
	if key, ok := namedKeys[strings.ToLower(name)]; ok {
		return key, nil
	}
	runes := []rune(name)
	if len(runes) == 1 {
		return input.Key(runes[0]), nil
	}
	return 0, fmt.Errorf("unknown key: %q", name)
}

// KeyPressExecutor handles types.ActionTypeKeyPress: it types the key named
// by the action input (a named key like "enter" or a single character) on
// the page.
type KeyPressExecutor struct{}

func (KeyPressExecutor) Type() types.ActionType { return types.ActionTypeKeyPress }

func (KeyPressExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	key, err := parseKey(action.Input)
	if err != nil {
		return err
	}
	pTimeout := page.Timeout(c.options.PageMaxTimeout)
	return pTimeout.Keyboard.Type(key)
}

// FillFormExecutor handles types.ActionTypeFillForm: it fills and submits
// the action's form.
type FillFormExecutor struct{}

func (FillFormExecutor) Type() types.ActionType { return types.ActionTypeFillForm }

func (FillFormExecutor) Execute(c *Crawler, action *types.Action, page *browser.BrowserPage) error {
	if err := c.processForm(page, action.Form); err != nil {
		return err
	}
	return page.WaitPageLoadHeurisitics()
}
