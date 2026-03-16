package tools

import (
	"context"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/media"
)

func TestRenderHTMLTool_Name(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	if tool.Name() != "render_html" {
		t.Errorf("expected name 'render_html', got %q", tool.Name())
	}
}

func TestRenderHTMLTool_Defaults(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	if tool.viewportWidth != 800 {
		t.Errorf("expected default width 800, got %d", tool.viewportWidth)
	}
	if tool.viewportHeight != 600 {
		t.Errorf("expected default height 600, got %d", tool.viewportHeight)
	}
	if tool.timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", tool.timeout)
	}
}

func TestRenderHTMLTool_CustomConfig(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{
		ViewportWidth:  1024,
		ViewportHeight: 768,
		Timeout:        60 * time.Second,
	})
	if tool.viewportWidth != 1024 {
		t.Errorf("expected width 1024, got %d", tool.viewportWidth)
	}
	if tool.viewportHeight != 768 {
		t.Errorf("expected height 768, got %d", tool.viewportHeight)
	}
	if tool.timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", tool.timeout)
	}
}

func TestRenderHTMLTool_Parameters(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	params := tool.Parameters()

	props, propsOK := params["properties"].(map[string]any)
	if !propsOK {
		t.Fatal("expected properties map")
	}
	if _, found := props["html"]; !found {
		t.Error("expected 'html' property")
	}
	if _, found := props["width"]; !found {
		t.Error("expected 'width' property")
	}
	if _, found := props["height"]; !found {
		t.Error("expected 'height' property")
	}

	required, reqOK := params["required"].([]string)
	if !reqOK {
		t.Fatal("expected required slice")
	}
	if len(required) != 1 || required[0] != "html" {
		t.Errorf("expected required=['html'], got %v", required)
	}
}

func TestRenderHTMLTool_MissingHTML(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Fatal("expected error for missing html")
	}
}

func TestRenderHTMLTool_EmptyHTML(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	result := tool.Execute(context.Background(), map[string]any{"html": ""})
	if !result.IsError {
		t.Fatal("expected error for empty html")
	}
}

func TestRenderHTMLTool_NoChannel(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	// No channel/chatID in context
	result := tool.Execute(context.Background(), map[string]any{"html": "<h1>Hello</h1>"})
	if !result.IsError {
		t.Fatal("expected error when no channel context")
	}
}

func TestRenderHTMLTool_NoMediaStore(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	ctx := WithToolContext(context.Background(), "telegram", "chat123")
	result := tool.Execute(ctx, map[string]any{"html": "<h1>Hello</h1>"})
	if !result.IsError {
		t.Fatal("expected error when no media store")
	}
}

func TestRenderHTMLTool_SetMediaStore(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	if tool.mediaStore != nil {
		t.Fatal("expected nil mediaStore initially")
	}
	store := media.NewFileMediaStore()
	tool.SetMediaStore(store)
	if tool.mediaStore == nil {
		t.Fatal("expected mediaStore to be set")
	}
}

func TestRenderHTMLTool_NoBrowser(t *testing.T) {
	// This test verifies graceful error when no browser is installed.
	// In most CI environments, Chromium is not available.
	if findBrowser() != "" {
		t.Skip("browser found — skipping no-browser test")
	}

	store := media.NewFileMediaStore()
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	tool.SetMediaStore(store)

	ctx := WithToolContext(context.Background(), "telegram", "chat123")
	result := tool.Execute(ctx, map[string]any{"html": "<h1>Hello</h1>"})
	if !result.IsError {
		t.Fatal("expected error when no browser installed")
	}
	if result.ForLLM == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestFindBrowser(t *testing.T) {
	// Just verify the function doesn't panic; actual result depends on environment.
	_ = findBrowser()
}

func TestRenderHTMLTool_Description(t *testing.T) {
	tool := NewRenderHTMLTool("/tmp", RenderHTMLConfig{})
	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
}
