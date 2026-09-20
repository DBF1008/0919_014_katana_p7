package crawler

import (
	"context"
	"fmt"

	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
)

// ActionExecutor executes a single headless action against a browser page.
//
// Each action type owns one ActionExecutor implementation. Executors are
// looked up by type through an ActionRegistry, so adding a new action type is
// a matter of implementing this interface and registering it — the core
// dispatch never needs to change (Command pattern).
type ActionExecutor interface {
	// Type returns the action type this executor handles.
	Type() types.ActionType
	// Execute performs the action against page. It must not return until the
	// page has settled enough for the next crawler step.
	Execute(ctx context.Context, c *Crawler, action *types.Action, page *browser.BrowserPage) error
}

// ErrActionExecutorNotFound is returned when no executor is registered for
// an action type.
var ErrActionExecutorNotFound = fmt.Errorf("action executor not found")

// ActionRegistry maps action types to their ActionExecutor implementations.
//
// The zero value is an empty registry; use NewActionRegistry for a registry
// pre-populated with the built-in executors.
type ActionRegistry struct {
	executors map[types.ActionType]ActionExecutor
}

// NewActionRegistry creates a registry pre-populated with the built-in
// action executors.
func NewActionRegistry() *ActionRegistry {
	registry := &ActionRegistry{executors: make(map[types.ActionType]ActionExecutor)}
	registerDefaultActionExecutors(registry)
	return registry
}

// Register installs executor for its declared action type, replacing any
// previously registered executor of the same type.
func (r *ActionRegistry) Register(executor ActionExecutor) {
	r.executors[executor.Type()] = executor
}

// RegisterType installs executor under an explicit action type, allowing an
// executor to be aliased under more than one type.
func (r *ActionRegistry) RegisterType(actionType types.ActionType, executor ActionExecutor) {
	r.executors[actionType] = executor
}

// Lookup returns the executor registered for actionType, or
// ErrActionExecutorNotFound when none is registered.
func (r *ActionRegistry) Lookup(actionType types.ActionType) (ActionExecutor, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: %s", ErrActionExecutorNotFound, actionType)
	}
	executor, ok := r.executors[actionType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrActionExecutorNotFound, actionType)
	}
	return executor, nil
}

// Types returns the action types currently registered, in no particular
// order.
func (r *ActionRegistry) Types() []types.ActionType {
	actionTypes := make([]types.ActionType, 0, len(r.executors))
	for actionType := range r.executors {
		actionTypes = append(actionTypes, actionType)
	}
	return actionTypes
}

// RegisterActionExecutor installs a custom executor on the crawler's
// registry. This is the extension point for new action types: registration
// alone is enough, the dispatch core does not need modification.
func (c *Crawler) RegisterActionExecutor(executor ActionExecutor) {
	if c.actions == nil {
		c.actions = NewActionRegistry()
	}
	c.actions.Register(executor)
}

// dispatchCrawlAction looks up the executor registered for the action type
// and runs it.
func (c *Crawler) dispatchCrawlAction(ctx context.Context, action *types.Action, page *browser.BrowserPage) error {
	registry := c.actions
	if registry == nil {
		registry = defaultActionRegistry()
	}
	executor, err := registry.Lookup(action.Type)
	if err != nil {
		return err
	}
	return executor.Execute(ctx, c, action, page)
}

// defaultActionRegistry returns a registry built from the built-in
// executors. It is only used as a safety net for zero-value Crawler
// instances (e.g. in unit tests).
func defaultActionRegistry() *ActionRegistry {
	return NewActionRegistry()
}
