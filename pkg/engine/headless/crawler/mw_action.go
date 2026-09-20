package crawler

import "log/slog"

// actionMiddleware restores the page to the action's origin state, records
// the action in diagnostics, and dispatches it through the action executor
// registry wrapped by the lifecycle hooks.
func (c *Crawler) actionMiddleware(next CrawlHandler) CrawlHandler {
	return func(step *CrawlStep) error {
		currentPageHash, _, err := getPageHash(step.Page)
		if err != nil {
			return err
		}
		step.CurrentPageHash = currentPageHash

		c.logger.Debug("Processing action - current state",
			slog.String("current_page_hash", currentPageHash),
			slog.String("action_origin_id", step.Action.OriginID),
			slog.String("action", step.Action.String()),
		)

		if step.Action.OriginID != "" && step.Action.OriginID != currentPageHash {
			c.logger.Debug("Need to navigate back to origin",
				slog.String("from", currentPageHash),
				slog.String("to", step.Action.OriginID),
			)
			newPageHash, err := c.navigateBackToStateOrigin(step.Action, step.Page, currentPageHash)
			if err != nil {
				return err
			}
			step.CurrentPageHash = newPageHash
		}

		if c.diagnostics != nil {
			if err := c.diagnostics.LogAction(step.Action); err != nil {
				return err
			}
		}
		if err := c.executeCrawlStateAction(step.Ctx, step.Action, step.Page); err != nil {
			return err
		}
		return next(step)
	}
}
