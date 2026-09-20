package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adrianbrad/queue"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/happyhackingspace/dit"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/katana/pkg/engine/headless/browser"
	"github.com/projectdiscovery/katana/pkg/engine/headless/captcha"
	"github.com/projectdiscovery/katana/pkg/engine/headless/crawler/diagnostics"
	"github.com/projectdiscovery/katana/pkg/engine/headless/crawler/normalizer"
	"github.com/projectdiscovery/katana/pkg/engine/headless/crawler/normalizer/simhash"
	"github.com/projectdiscovery/katana/pkg/engine/headless/graph"
	"github.com/projectdiscovery/katana/pkg/engine/headless/types"
	"github.com/projectdiscovery/katana/pkg/output"
)

type Crawler struct {
	logger        *slog.Logger
	launcher      *browser.Launcher
	options       Options
	crawlQueue    queue.Queue[*types.Action]
	crawlGraph    *graph.CrawlGraph
	actions       *ActionRegistry
	crawlChain    CrawlHandler
	simhashOracle *simhash.Oracle
	uniqueActions map[string]struct{}
	diagnostics   diagnostics.Writer
	loggedIn      bool
}

type Options struct {
	Context             context.Context
	ChromiumPath        string
	MaxBrowsers         int
	MaxDepth            int
	PageMaxTimeout      time.Duration
	NoSandbox           bool
	NoIncognito         bool
	ShowBrowser         bool
	SlowMotion          bool
	MaxCrawlDuration    time.Duration
	MaxFailureCount     int
	Trace               bool
	CookieConsentBypass bool
	AutomaticFormFill   bool
	PageLoadStrategy    string
	ChromeWSUrl         string
	DOMWaitTime         int
	UserDataDir         string

	// EnableDiagnostics enables the diagnostics mode
	// which writes diagnostic information to a directory
	// specified by the DiagnosticsDir optionally.
	EnableDiagnostics bool
	DiagnosticsDir    string

	Proxy           string
	Logger          *slog.Logger
	ScopeValidator  browser.ScopeValidator
	RequestCallback func(*output.Result)
	ChromeUser      *user.User
	CaptchaHandler  *captcha.Handler
	UserArguments   map[string]string

	AuthUsername  string
	AuthPassword  string
	DitClassifier *dit.Classifier

	// Hooks installs optional lifecycle callbacks. See Hooks for semantics.
	// The zero value disables all callbacks.
	Hooks Hooks
}

var domNormalizer *normalizer.Normalizer
var initOnce sync.Once
var initError error

func init() {
	initOnce.Do(func() {
		var err error
		domNormalizer, err = normalizer.New()
		if err != nil {
			initError = errors.Wrap(err, "failed to create domnormalizer")
		}
	})
}

func New(opts Options) (*Crawler, error) {
	if initError != nil {
		return nil, initError
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	launcher, err := browser.NewLauncher(browser.LauncherOptions{
		ChromiumPath:        opts.ChromiumPath,
		MaxBrowsers:         opts.MaxBrowsers,
		PageMaxTimeout:      opts.PageMaxTimeout,
		ShowBrowser:         opts.ShowBrowser,
		RequestCallback:     opts.RequestCallback,
		SlowMotion:          opts.SlowMotion,
		ScopeValidator:      opts.ScopeValidator,
		ChromeUser:          opts.ChromeUser,
		Trace:               opts.Trace,
		CookieConsentBypass: opts.CookieConsentBypass,
		NoSandbox:           opts.NoSandbox,
		NoIncognito:         opts.NoIncognito,
		PageLoadStrategy:    opts.PageLoadStrategy,
		ChromeWSUrl:         opts.ChromeWSUrl,
		DOMWaitTime:         opts.DOMWaitTime,
		UserDataDir:         opts.UserDataDir,
		Proxy:               opts.Proxy,
		UserArguments:       opts.UserArguments,
	})
	if err != nil {
		return nil, err
	}

	var diagnosticsWriter diagnostics.Writer
	if opts.EnableDiagnostics {
		directory := opts.DiagnosticsDir
		if directory == "" {
			cwd, _ := os.Getwd()
			directory = filepath.Join(cwd, fmt.Sprintf("katana-diagnostics-%s", time.Now().Format(time.RFC3339)))
		}

		writer, err := diagnostics.NewWriter(directory)
		if err != nil {
			return nil, err
		}
		diagnosticsWriter = writer
		opts.DiagnosticsDir = directory
		opts.Logger.Info("Diagnostics enabled", slog.String("directory", directory))
	}

	crawler := &Crawler{
		launcher:      launcher,
		options:       opts,
		logger:        opts.Logger,
		actions:       NewActionRegistry(),
		uniqueActions: make(map[string]struct{}),
		diagnostics:   diagnosticsWriter,
		simhashOracle: simhash.NewOracle(),
	}
	crawler.crawlChain = BuildCrawlChain(
		func(*CrawlStep) error { return nil },
		crawler.defaultCrawlMiddlewares()...,
	)
	return crawler, nil
}

func (c *Crawler) Close() {
	c.launcher.Close()
	if c.diagnostics != nil {
		if err := c.diagnostics.Close(); err != nil {
			c.logger.Warn("Failed to close diagnostics", slog.String("error", err.Error()))
		}
	}
}

func (c *Crawler) GetCrawlGraph() *graph.CrawlGraph {
	return c.crawlGraph
}

func (c *Crawler) Crawl(URL string) error {
	defer func() {
		if c.diagnostics == nil {
			return
		}
		err := c.crawlGraph.DrawGraph(filepath.Join(c.options.DiagnosticsDir, "crawl-graph.dot"))
		if err != nil {
			c.logger.Error("Failed to draw crawl graph", slog.String("error", err.Error()))
		}
	}()

	actions := []*types.Action{{
		Type:     types.ActionTypeLoadURL,
		Input:    URL,
		Depth:    0,
		OriginID: emptyPageHash,
	}}

	crawlQueue := queue.NewLinked(actions)
	c.crawlQueue = crawlQueue

	crawlGraph := graph.NewCrawlGraph()
	c.crawlGraph = crawlGraph

	// Add the initial blank state
	err := crawlGraph.AddPageState(types.PageState{
		UniqueID: emptyPageHash,
		URL:      "about:blank",
		Depth:    0,
	})
	if err != nil {
		return err
	}

	// Create a master context that will automatically cancel all page operations
	// once the per-URL crawl deadline is reached.
	parentCtx := c.options.Context
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		localDeadline bool
	)
	if c.options.MaxCrawlDuration > 0 {
		ctx, cancel = context.WithTimeout(parentCtx, c.options.MaxCrawlDuration)
		localDeadline = true
	} else {
		ctx, cancel = context.WithCancel(parentCtx)
	}
	defer cancel()

	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			// Distinguish internal max-duration from external parent cancellation
			if localDeadline && parentCtx.Err() == nil {
				c.logger.Debug("Max crawl duration reached, stopping crawl")
				return nil
			}
			c.logger.Debug("Context cancelled, stopping headless crawl")
			return ctx.Err()
		default:
			// Check for too many failures
			if c.options.MaxFailureCount > 0 && consecutiveFailures >= c.options.MaxFailureCount {
				c.logger.Warn("Too many consecutive failures, stopping crawl",
					slog.Int("failures", consecutiveFailures),
					slog.Int("max_allowed", c.options.MaxFailureCount),
					slog.Int("remaining_actions", c.crawlQueue.Size()),
				)
				return nil
			}

			action, err := crawlQueue.Get()
			if err == queue.ErrNoElementsAvailable {
				c.logger.Debug("No more actions to process")
				return nil
			}
			if err != nil {
				return err
			}

			if c.options.MaxDepth > 0 && action.Depth > c.options.MaxDepth {
				continue
			}

			page, err := c.launcher.GetPageFromPool()
			if err != nil {
				return err
			}

			page.Page = page.Context(ctx)

			c.logger.Debug("Processing action",
				slog.String("action", action.String()),
			)

			if err := c.crawlFn(ctx, action, page); err != nil {
				if err == ErrNoCrawlingAction {
					return nil
				}
				if errors.Is(err, ErrElementNotVisible) {
					consecutiveFailures++
					continue
				}
				var npe *rod.NoPointerEventsError
				var ish *rod.InvisibleShapeError
				if errors.As(err, &npe) || errors.As(err, &ish) {
					c.logger.Debug("Skipping action as it is not visible",
						slog.String("action", action.String()),
						slog.String("error", err.Error()),
					)
					consecutiveFailures++
					continue
				}
				var ne *rod.NavigationError
				if errors.As(err, &ne) {
					c.logger.Debug("Skipping action as navigation failed",
						slog.String("action", action.String()),
						slog.String("error", err.Error()),
					)
					consecutiveFailures++
					continue
				}
				if errors.Is(err, ErrNoNavigationPossible) {
					c.logger.Debug("Skipping action as no navigation possible", slog.String("action", action.String()))
					consecutiveFailures++
					continue
				}
				var msce *utils.MaxSleepCountError
				if errors.As(err, &msce) {
					c.logger.Debug("Skipping action as it is taking too long", slog.String("action", action.String()))
					consecutiveFailures++
					continue
				}

				c.logger.Debug("Skipping action due to site-specific error",
					slog.String("error", err.Error()),
					slog.String("action", action.String()),
				)
				consecutiveFailures++
				continue
			}

			consecutiveFailures = 0
		}
	}
}

func (c *Crawler) crawlFn(ctx context.Context, action *types.Action, page *browser.BrowserPage) error {
	defer func() {
		c.launcher.PutBrowserToPool(page)
	}()

	return c.crawlChain(&CrawlStep{
		Ctx:    ctx,
		Action: action,
		Page:   page,
	})
}

var ErrNoCrawlingAction = errors.New("no more actions to crawl")

// executeCrawlStateAction runs action through the lifecycle hooks and the
// executor registry.
func (c *Crawler) executeCrawlStateAction(ctx context.Context, action *types.Action, page *browser.BrowserPage) error {
	return runWithActionHooks(c.options.Hooks, page, action, func() error {
		return c.dispatchCrawlAction(ctx, action, page)
	})
}

func (c *Crawler) tryAutoLogin(page *browser.BrowserPage, html string) bool {
	pageResult, err := c.options.DitClassifier.ExtractPageType(html)
	if err != nil || pageResult == nil {
		return false
	}

	for _, form := range pageResult.Forms {
		if form.Type != "login" {
			continue
		}

		pageURL := ""
		if info, err := page.Info(); err == nil {
			pageURL = info.URL
		}
		c.logger.Info("Login form detected, attempting auto-login",
			slog.String("url", pageURL),
		)

		filled := false
		for fieldName, fieldType := range form.Fields {
			var value string
			switch fieldType {
			case "password":
				value = c.options.AuthPassword
			default:
				value = c.options.AuthUsername
			}

			escapedName := strings.ReplaceAll(fieldName, `\`, `\\`)
			escapedName = strings.ReplaceAll(escapedName, `'`, `\'`)
			el, err := page.Element("input[name='" + escapedName + "']")
			if err != nil {
				c.logger.Debug("Could not find login field", slog.String("field", fieldName))
				continue
			}
			if err := el.Input(value); err != nil {
				c.logger.Debug("Could not fill login field", slog.String("field", fieldName))
				continue
			}
			filled = true
		}

		if !filled {
			continue
		}

		if submitted := c.submitLoginForm(page); submitted {
			c.loggedIn = true
			c.logger.Info("Auto-login submitted successfully")
			return true
		}
	}
	return false
}

func (c *Crawler) submitLoginForm(page *browser.BrowserPage) bool {
	selectors := []string{
		"form button[type='submit']",
		"form input[type='submit']",
		"form button:not([type])",
	}
	for _, sel := range selectors {
		if el, err := page.Element(sel); err == nil {
			if err := el.Click(proto.InputMouseButtonLeft, 1); err == nil {
				return true
			}
		}
	}
	return false
}

var logoutPattern = regexp.MustCompile(`(?i)(log[\s-]?out|sign[\s-]?out|signout|deconnexion|cerrar[\s-]?sesion|sair|abmelden|uitloggen|ausloggen|exit|disconnect|terminate|end[\s-]?session|salir|desconectar|afmelden|wyloguj|logout|sign[\s-]?off)`)

func isLogoutPage(element *types.HTMLElement) bool {
	return logoutPattern.MatchString(element.TextContent) ||
		logoutPattern.MatchString(element.Attributes["href"])
}
