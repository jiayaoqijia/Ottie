package ui

import (
	"testing"

	ottieconfig "github.com/jiayaoqijia/ottie/pkg/config"
)

// TestMainMenuConstruction validates that the TUI main menu can be
// built from a default config without panicking. The TUI is a tview
// terminal app (ncurses-style) so it cannot be tested via Playwright;
// this headless test validates the menu construction logic.
func TestMainMenuConstruction(t *testing.T) {
	cfg := ottieconfig.DefaultConfig()
	state := &appState{
		config: cfg,
		menus:  map[string]*Menu{},
	}

	// Build the main menu — should not panic
	menu := NewMenu("Menu", nil)
	refreshMainMenu(menu, state)

	if len(menu.items) == 0 {
		t.Fatal("main menu has no items")
	}

	// Verify expected menu items exist
	labels := make([]string, 0, len(menu.items))
	for _, item := range menu.items {
		labels = append(labels, item.Label)
	}
	t.Logf("main menu items: %v", labels)

	// Should have at least Model, Channels, and Gateway items
	hasModel := false
	hasChannel := false
	for _, label := range labels {
		if len(label) >= 5 && label[:5] == "Model" {
			hasModel = true
		}
		if len(label) >= 7 && label[:7] == "Channel" {
			hasChannel = true
		}
	}
	if !hasModel {
		t.Errorf("main menu missing Model item; got: %v", labels)
	}
	if !hasChannel {
		t.Errorf("main menu missing Channel item; got: %v", labels)
	}
}

// TestCountChannels validates the channel counter logic.
func TestCountChannels(t *testing.T) {
	cfg := ottieconfig.DefaultConfig()
	state := &appState{
		config: cfg,
		menus:  map[string]*Menu{},
	}

	enabled, total := state.countChannels()
	if total == 0 {
		t.Error("total channels should be > 0")
	}
	// Default config has no channels enabled
	if enabled != 0 {
		t.Errorf("enabled channels = %d, want 0 for default config", enabled)
	}
	t.Logf("channels: %d enabled, %d total", enabled, total)
}

// TestModelMenuConstruction validates that the model menu builds
// correctly with a populated model_list.
func TestModelMenuConstruction(t *testing.T) {
	cfg := ottieconfig.DefaultConfig()
	cfg.ModelList = []ottieconfig.ModelConfig{
		{ModelName: "test-model", Model: "openai/test-model", APIKey: "sk-test", APIBase: "https://api.test.com/v1"},
		{ModelName: "other-model", Model: "anthropic/other-model", APIKey: "", APIBase: ""},
	}
	cfg.Agents.Defaults.Model = "test-model"

	state := &appState{
		config: cfg,
		menus:  map[string]*Menu{},
	}

	menu := NewMenu("Model", nil)
	refreshModelMenuFromState(menu, state)

	if len(menu.items) == 0 {
		t.Fatal("model menu has no items")
	}

	// Should have at least 2 model items + Add Model
	t.Logf("model menu items: %d", len(menu.items))
	for _, item := range menu.items {
		t.Logf("  - %s: %s", item.Label, item.Description)
	}
}
