package crawler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCrawlChain_OuterToInnerOrder(t *testing.T) {
	var steps []string
	record := func(name string) CrawlMiddleware {
		return NewCrawlMiddleware(name, func(next CrawlHandler) CrawlHandler {
			return func(step *CrawlStep) error {
				steps = append(steps, "before:"+name)
				err := next(step)
				steps = append(steps, "after:"+name)
				return err
			}
		})
	}

	terminalCalled := false
	chain := BuildCrawlChain(
		func(*CrawlStep) error { terminalCalled = true; return nil },
		record("a"), record("b"), record("c"),
	)

	require.NoError(t, chain(&CrawlStep{}))
	assert.True(t, terminalCalled)
	assert.Equal(t, []string{
		"before:a", "before:b", "before:c",
		"after:c", "after:b", "after:a",
	}, steps)
}

func TestBuildCrawlChain_MiddlewareErrorStopsChain(t *testing.T) {
	var reachedInner bool
	sentinel := errors.New("stop")
	chain := BuildCrawlChain(
		func(*CrawlStep) error { reachedInner = true; return nil },
		NewCrawlMiddleware("outer", func(next CrawlHandler) CrawlHandler {
			return func(step *CrawlStep) error { return sentinel }
		}),
		NewCrawlMiddleware("inner", func(next CrawlHandler) CrawlHandler {
			return func(step *CrawlStep) error { return next(step) }
		}),
	)

	err := chain(&CrawlStep{})
	require.ErrorIs(t, err, sentinel)
	assert.False(t, reachedInner, "an outer middleware error must not reach the terminal handler")
}

func TestBuildCrawlChain_NoMiddlewares(t *testing.T) {
	called := false
	terminal := func(*CrawlStep) error { called = true; return nil }
	chain := BuildCrawlChain(terminal)

	require.NoError(t, chain(&CrawlStep{}))
	assert.True(t, called)
}

// TestDefaultCrawlMiddlewareOrder pins the required pipeline order:
// action -> captcha -> auth -> scope -> discovery -> graph.
func TestDefaultCrawlMiddlewareOrder(t *testing.T) {
	c := &Crawler{}
	middlewares := c.defaultCrawlMiddlewares()

	names := make([]string, 0, len(middlewares))
	for _, mw := range middlewares {
		names = append(names, mw.Name())
	}
	assert.Equal(t, []string{"action", "captcha", "auth", "scope", "discovery", "graph"}, names)
}
