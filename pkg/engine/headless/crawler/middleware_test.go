package crawler

import (
	"errors"
	"testing"

	"github.com/adrianbrad/queue"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingMiddleware returns a middleware that appends name+"-before" and
// name+"-after" around the rest of the chain.
func recordingMiddleware(tr *trace, name string) CrawlMiddleware {
	return func(next CrawlHandler) CrawlHandler {
		return func(c *Crawler, cc *CrawlContext) error {
			tr.add(name + "-before")
			err := next(c, cc)
			tr.add(name + "-after")
			return err
		}
	}
}

func TestChain_ExecutionOrder(t *testing.T) {
	var tr trace
	handler := Chain(
		recordingMiddleware(&tr, "first"),
		recordingMiddleware(&tr, "second"),
	)(func(_ *Crawler, _ *CrawlContext) error {
		tr.add("terminal")
		return nil
	})

	err := handler(nil, &CrawlContext{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"first-before", "second-before", "terminal", "second-after", "first-after",
	}, tr.steps, "middlewares must wrap in list order, outermost first")
}

func TestChain_ShortCircuit(t *testing.T) {
	var tr trace
	stop := func(_ CrawlHandler) CrawlHandler {
		return func(_ *Crawler, _ *CrawlContext) error {
			tr.add("stop")
			return nil
		}
	}
	handler := Chain(
		recordingMiddleware(&tr, "first"),
		stop,
		recordingMiddleware(&tr, "never"),
	)(func(_ *Crawler, _ *CrawlContext) error {
		tr.add("terminal")
		return nil
	})

	err := handler(nil, &CrawlContext{})

	require.NoError(t, err)
	assert.Equal(t, []string{"first-before", "stop", "first-after"}, tr.steps,
		"a middleware that skips next must prevent the rest of the chain from running")
}

func TestChain_ErrorPropagation(t *testing.T) {
	sentinel := errors.New("terminal failed")
	handler := Chain(recordingMiddleware(&trace{}, "first"))(
		func(_ *Crawler, _ *CrawlContext) error { return sentinel },
	)

	err := handler(nil, &CrawlContext{})

	require.ErrorIs(t, err, sentinel)
}

func TestDefaultCrawlMiddlewares_Order(t *testing.T) {
	middlewares := DefaultCrawlMiddlewares()
	require.Len(t, middlewares, 5,
		"default pipeline must be Captcha → Auth → Scope → Discovery → Graph")
}

func TestCaptchaMiddleware_NoHandlerPassesThrough(t *testing.T) {
	var tr trace
	crawler := &Crawler{}
	handler := CaptchaMiddleware()(func(_ *Crawler, _ *CrawlContext) error {
		tr.add("next")
		return nil
	})

	err := handler(crawler, &CrawlContext{})

	require.NoError(t, err)
	assert.Equal(t, []string{"next"}, tr.steps,
		"without a captcha handler the chain must continue untouched")
}

func TestAuthMiddleware_NoCredentialsPassesThrough(t *testing.T) {
	var tr trace
	crawler := &Crawler{}
	handler := AuthMiddleware()(func(_ *Crawler, _ *CrawlContext) error {
		tr.add("next")
		return nil
	})

	err := handler(crawler, &CrawlContext{})

	require.NoError(t, err)
	assert.Equal(t, []string{"next"}, tr.steps,
		"without auth credentials the chain must continue untouched")
}

func TestTerminalCrawlHandler_EmptyQueue(t *testing.T) {
	crawler := &Crawler{crawlQueue: queue.NewLinked([]*types.Action{})}

	err := terminalCrawlHandler(crawler, &CrawlContext{})

	require.ErrorIs(t, err, ErrNoCrawlingAction,
		"no navigations and an empty queue must signal the end of the crawl")
}
