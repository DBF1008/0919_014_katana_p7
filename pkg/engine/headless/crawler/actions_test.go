package crawler

import (
	"testing"

	"github.com/go-rod/rod/lib/input"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewActionRegistry_DefaultExecutors(t *testing.T) {
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
		executor, ok := registry.Lookup(actionType)
		assert.True(t, ok, "executor must be registered for action type %q", actionType)
		assert.NotNil(t, executor)
	}
}

func TestActionRegistry_ClickAliasesShareExecutor(t *testing.T) {
	registry := NewActionRegistry()
	click, ok := registry.Lookup(types.ActionTypeClick)
	require.True(t, ok)
	for _, alias := range []types.ActionType{types.ActionTypeLeftClick, types.ActionTypeLeftClickDown} {
		executor, ok := registry.Lookup(alias)
		require.True(t, ok, "alias %q must be registered", alias)
		assert.Equal(t, click, executor, "alias %q must share the click executor", alias)
	}
}

func TestActionRegistry_UnknownType(t *testing.T) {
	registry := NewActionRegistry()
	_, ok := registry.Lookup(types.ActionTypeUnknown)
	assert.False(t, ok, "no executor must be registered for the unknown action type")
}

func TestActionRegistry_RegisterReplacesExisting(t *testing.T) {
	registry := NewActionRegistry()
	custom := LoadURLExecutor{}
	registry.Register(custom)
	executor, ok := registry.Lookup(types.ActionTypeLoadURL)
	require.True(t, ok)
	assert.Equal(t, custom, executor)
}

func TestDispatchCrawlAction_UnknownType(t *testing.T) {
	crawler := &Crawler{actionRegistry: NewActionRegistry()}
	err := crawler.dispatchCrawlAction(&types.Action{Type: types.ActionTypeUnknown}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action type")
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    input.Key
		wantErr bool
	}{
		{name: "named enter", input: "enter", want: input.Enter},
		{name: "named case insensitive", input: "Enter", want: input.Enter},
		{name: "named tab", input: "tab", want: input.Tab},
		{name: "named escape", input: "escape", want: input.Escape},
		{name: "named esc alias", input: "esc", want: input.Escape},
		{name: "named arrow down", input: "ArrowDown", want: input.ArrowDown},
		{name: "named arrow alias", input: "down", want: input.ArrowDown},
		{name: "single character", input: "a", want: input.Key('a')},
		{name: "single digit", input: "1", want: input.Key('1')},
		{name: "unknown multi-char", input: "notakey", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := parseKey(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, key)
		})
	}
}
