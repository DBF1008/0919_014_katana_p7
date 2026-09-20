package crawler

import (
	"testing"

	"github.com/go-rod/rod/lib/input"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScrollOffset(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want float64
	}{
		{"empty defaults", "", 800},
		{"spaces default", "   ", 800},
		{"garbage defaults", "abc", 800},
		{"explicit pixels", "1200", 1200},
		{"fractional pixels", "320.5", 320.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseScrollOffset(tt.raw))
		})
	}
}

func TestParseKeyChord(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []input.Key
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: true},
		{name: "whitespace only", raw: "   ", wantErr: true},
		{name: "single character", raw: "a", want: []input.Key{input.Key('a')}},
		{name: "alias enter", raw: "Enter", want: []input.Key{input.Enter}},
		{name: "alias case insensitive", raw: "ESCAPE", want: []input.Key{input.Escape}},
		{name: "chord control enter", raw: "Control+Enter", want: []input.Key{input.ControlLeft, input.Enter}},
		{name: "chord with spaces", raw: "ctrl + c", want: []input.Key{input.ControlLeft, input.Key('c')}},
		{name: "unknown token", raw: "NotARealKey", wantErr: true},
		{name: "empty chord segment", raw: "Control+", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKeyChord(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuiltinExecutors_DeclaredTypes(t *testing.T) {
	tests := []struct {
		executor   ActionExecutor
		actionType types.ActionType
	}{
		{&LoadURLExecutor{}, types.ActionTypeLoadURL},
		{&NavigateExecutor{}, types.ActionTypeNavigate},
		{&ClickExecutor{}, types.ActionTypeClick},
		{&ScrollExecutor{}, types.ActionTypeScroll},
		{&WaitVisibleExecutor{}, types.ActionTypeWaitVisible},
		{&WaitHiddenExecutor{}, types.ActionTypeWaitHidden},
		{&KeyPressExecutor{}, types.ActionTypeKeyPress},
		{&FillFormExecutor{}, types.ActionTypeFillForm},
	}
	for _, tt := range tests {
		t.Run(string(tt.actionType), func(t *testing.T) {
			assert.Equal(t, tt.actionType, tt.executor.Type())
		})
	}
}
