package crawler

import (
	"context"
	"testing"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExecutor is a minimal ActionExecutor used to exercise the registry
// without a browser.
type stubExecutor struct {
	actionType types.ActionType
	called     bool
}

func (s *stubExecutor) Type() types.ActionType { return s.actionType }

func (s *stubExecutor) Execute(context.Context, *Crawler, *types.Action, *browser.BrowserPage) error {
	s.called = true
	return nil
}

func TestNewActionRegistry_RegistersAllBuiltinTypes(t *testing.T) {
	registry := NewActionRegistry()

	expected := []types.ActionType{
		types.ActionTypeLoadURL,
		types.ActionTypeNavigate,
		types.ActionTypeClick,
		types.ActionTypeLeftClick,
		types.ActionTypeLeftClickDown,
		types.ActionTypeScroll,
		types.ActionTypeWaitVisible,
		types.ActionTypeWaitHidden,
		types.ActionTypeKeyPress,
		types.ActionTypeFillForm,
	}
	for _, actionType := range expected {
		executor, err := registry.Lookup(actionType)
		require.NoError(t, err, "expected built-in executor for %s", actionType)
		assert.NotNil(t, executor)
	}
}

func TestActionRegistry_LookupUnknown(t *testing.T) {
	registry := NewActionRegistry()

	_, err := registry.Lookup(types.ActionTypeUnknown)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrActionExecutorNotFound)

	var nilRegistry *ActionRegistry
	_, err = nilRegistry.Lookup(types.ActionTypeLoadURL)
	assert.ErrorIs(t, err, ErrActionExecutorNotFound,
		"a nil registry must return the not-found error instead of panicking")
}

func TestActionRegistry_Register_ReplacesPrevious(t *testing.T) {
	registry := NewActionRegistry()
	stub := &stubExecutor{actionType: types.ActionTypeLoadURL}
	registry.Register(stub)

	executor, err := registry.Lookup(types.ActionTypeLoadURL)
	require.NoError(t, err)
	assert.Same(t, stub, executor)
}

func TestCrawler_RegisterActionExecutor_OnZeroValueCrawler(t *testing.T) {
	c := &Crawler{}
	stub := &stubExecutor{actionType: types.ActionTypeExecuteJS}
	c.RegisterActionExecutor(stub)

	executor, err := c.actions.Lookup(types.ActionTypeExecuteJS)
	require.NoError(t, err)
	assert.Same(t, stub, executor)
}

func TestCrawler_DispatchCrawlAction_UnknownType(t *testing.T) {
	c := &Crawler{actions: NewActionRegistry()}
	err := c.dispatchCrawlAction(context.Background(),
		&types.Action{Type: types.ActionTypeUnknown}, &browser.BrowserPage{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrActionExecutorNotFound)
}

func TestCrawler_DispatchCrawlAction_ZeroValueCrawlerUsesDefaults(t *testing.T) {
	c := &Crawler{}
	// No executor for the type even in the default registry: must surface the
	// not-found error instead of a nil-pointer panic.
	err := c.dispatchCrawlAction(context.Background(),
		&types.Action{Type: types.ActionTypeRedirect}, &browser.BrowserPage{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrActionExecutorNotFound)
}

func TestCrawler_DispatchCrawlAction_KeyPressValidation(t *testing.T) {
	c := &Crawler{actions: NewActionRegistry()}
	err := c.dispatchCrawlAction(context.Background(),
		&types.Action{Type: types.ActionTypeKeyPress, Input: ""}, &browser.BrowserPage{})
	require.Error(t, err, "empty key input must fail before touching the page")
}

func TestCrawler_DispatchCrawlAction_WaitRequiresElement(t *testing.T) {
	c := &Crawler{actions: NewActionRegistry()}

	for _, actionType := range []types.ActionType{
		types.ActionTypeWaitVisible,
		types.ActionTypeWaitHidden,
		types.ActionTypeClick,
	} {
		err := c.dispatchCrawlAction(context.Background(),
			&types.Action{Type: actionType}, &browser.BrowserPage{})
		require.Error(t, err, "%s must require an element", actionType)
	}
}
