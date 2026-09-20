package crawler

import (
	"github.com/projectdiscovery/gologger"
)

// captchaMiddleware checks the post-action page for captchas. When one is
// detected it is handed to the configured captcha handler; after a
// successful solve the page is allowed to settle, but discovery is skipped
// because the links/forms on a captcha widget belong to the widget rather
// than the real page.
func (c *Crawler) captchaMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		if c.options.CaptchaHandler == nil {
			return next(step)
		}
		html, htmlErr := step.Page.HTML()
		if htmlErr != nil {
			return next(step)
		}
		handled, solveErr := c.options.CaptchaHandler.HandleIfCaptcha(step.Ctx, step.Page.Page, html)
		if solveErr != nil {
			gologger.Warning().Msgf("captcha solving failed: %s", solveErr)
		}
		if handled && solveErr == nil {
			_ = step.Page.WaitPageLoadHeurisitics()
		}
		if handled {
			return nil
		}
		return next(step)
	}
}

// authMiddleware attempts automatic login once, when credentials and a DIT
// classifier are configured and the current page is in scope.
func (c *Crawler) authMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		if !c.loggedIn && c.options.AuthUsername != "" && c.options.DitClassifier != nil {
			if info, err := step.Page.Info(); err == nil &&
				(c.options.ScopeValidator == nil || c.options.ScopeValidator(info.URL)) {
				if html, htmlErr := step.Page.HTML(); htmlErr == nil {
					if c.tryAutoLogin(step.Page, html) {
						_ = step.Page.WaitPageLoadHeurisitics()
					}
				}
			}
		}
		return next(step)
	}
}
