package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/jiayaoqijia/ottie/pkg/media"
)

// RenderHTMLConfig holds configuration for the RenderHTMLTool.
type RenderHTMLConfig struct {
	ViewportWidth  int
	ViewportHeight int
	Timeout        time.Duration
}

// RenderHTMLTool renders HTML content to a PNG image using headless Chromium
// and sends it to the user via the media pipeline. Chromium is a runtime-only
// dependency — the tool returns a clear error if it is not installed.
type RenderHTMLTool struct {
	mediaStore     media.MediaStore
	workspace      string
	viewportWidth  int
	viewportHeight int
	timeout        time.Duration
}

func NewRenderHTMLTool(workspace string, cfg RenderHTMLConfig) *RenderHTMLTool {
	if cfg.ViewportWidth <= 0 {
		cfg.ViewportWidth = 800
	}
	if cfg.ViewportHeight <= 0 {
		cfg.ViewportHeight = 600
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &RenderHTMLTool{
		workspace:      workspace,
		viewportWidth:  cfg.ViewportWidth,
		viewportHeight: cfg.ViewportHeight,
		timeout:        cfg.Timeout,
	}
}

func (t *RenderHTMLTool) Name() string { return "render_html" }

func (t *RenderHTMLTool) Description() string {
	return "Render HTML/CSS content to a PNG image and send it to the user. " +
		"Use this for rich visual content: dashboards, charts, formatted cards, or styled layouts. " +
		"The HTML must be self-contained (inline CSS, no external resources unless via CDN). " +
		"Requires Chromium to be installed on the system."
}

func (t *RenderHTMLTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"html": map[string]any{
				"type":        "string",
				"description": "Self-contained HTML/CSS content to render. Include all styles inline or via CDN links.",
			},
			"width": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Viewport width in pixels. Default: %d.", t.viewportWidth),
			},
			"height": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Viewport height in pixels. Default: %d.", t.viewportHeight),
			},
		},
		"required": []string{"html"},
	}
}

func (t *RenderHTMLTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

func (t *RenderHTMLTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	html, _ := args["html"].(string)
	if html == "" {
		return ErrorResult("html is required")
	}

	channel := ToolChannel(ctx)
	chatID := ToolChatID(ctx)
	if channel == "" || chatID == "" {
		return ErrorResult("no target channel/chat available")
	}

	if t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}

	// Detect Chromium at runtime
	browserPath := findBrowser()
	if browserPath == "" {
		return ErrorResult(
			"render_html requires Chromium or Google Chrome to be installed. " +
				"Install it with: apk add chromium (Alpine) or apt install chromium-browser (Debian). " +
				"Or use the full Docker image which includes Chromium.",
		)
	}

	// Viewport dimensions (allow per-call overrides)
	width := t.viewportWidth
	height := t.viewportHeight
	if w, ok := args["width"].(float64); ok && w > 0 {
		width = int(w)
	}
	if h, ok := args["height"].(float64); ok && h > 0 {
		height = int(h)
	}

	// Render HTML to PNG
	buf, err := renderHTMLToPNG(ctx, browserPath, html, width, height, t.timeout)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to render HTML: %v", err))
	}

	// Write PNG to temp file
	tmpFile, err := os.CreateTemp("", "render-*.png")
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create temp file: %v", err))
	}
	tmpPath := tmpFile.Name()
	// Do NOT defer os.Remove here — the media store holds a reference to this
	// path and channels (Telegram, Slack, etc.) will read the file later.
	// The media store's periodic cleanup will delete it after max_age.

	if _, writeErr := tmpFile.Write(buf); writeErr != nil {
		tmpFile.Close()
		return ErrorResult(fmt.Sprintf("failed to write image: %v", writeErr))
	}
	tmpFile.Close()

	// Store in media pipeline
	scope := fmt.Sprintf("tool:render_html:%s:%s", channel, chatID)
	ref, err := t.mediaStore.Store(tmpPath, media.MediaMeta{
		Filename:    "rendered.png",
		ContentType: "image/png",
		Source:      "tool:render_html",
	}, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to register media: %v", err))
	}

	return MediaResult("Image rendered and sent to user", []string{ref})
}

// HasBrowser returns true if a supported browser (Chromium or Chrome) is found
// on the system. Use this to gate registration of the render_html tool so that
// models are not offered a tool they cannot use.
func HasBrowser() bool {
	return findBrowser() != ""
}

// findBrowser searches for a Chromium or Chrome binary on the system.
func findBrowser() string {
	for _, name := range []string{
		"headless-shell",
		"chromium-browser",
		"chromium",
		"google-chrome",
		"google-chrome-stable",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	// macOS: check well-known .app bundle paths
	for _, path := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// renderHTMLToPNG uses chromedp to render HTML content in a headless browser
// and capture a full-page screenshot as a PNG buffer.
func renderHTMLToPNG(
	ctx context.Context,
	browserPath, html string,
	width, height int,
	timeout time.Duration,
) ([]byte, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.ExecPath(browserPath),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(width, height),
	)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	taskCtx, timeoutCancel := context.WithTimeout(taskCtx, timeout)
	defer timeoutCancel()

	var buf []byte
	err := chromedp.Run(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set HTML content directly via CDP
			return chromedp.Evaluate(
				fmt.Sprintf(`document.open(); document.write(%q); document.close();`, html),
				nil,
			).Do(ctx)
		}),
		chromedp.EmulateViewport(int64(width), int64(height)),
		// Wait briefly for any CSS/fonts to load
		chromedp.Sleep(200*time.Millisecond),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return nil, err
	}

	return buf, nil
}
