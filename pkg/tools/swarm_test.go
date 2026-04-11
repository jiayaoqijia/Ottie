package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/config"
)

func TestSwarmManager_SpawnBasic(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	ctx := context.Background()
	msg, err := sm.Spawn(ctx, SpawnParams{
		Task:          "say hello",
		Label:         "greeter",
		AgentID:       "agent-1",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, "greeter") {
		t.Errorf("expected label in message, got: %s", msg)
	}
	if !strings.Contains(msg, "swarm-1") {
		t.Errorf("expected run ID in message, got: %s", msg)
	}

	// Wait briefly for the goroutine to complete
	time.Sleep(100 * time.Millisecond)

	record, ok := sm.GetRun("swarm-1")
	if !ok {
		t.Fatal("expected run record to exist")
	}
	if record.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", record.AgentID)
	}
	if record.Label != "greeter" {
		t.Errorf("expected greeter, got %s", record.Label)
	}
	if record.Status != "completed" {
		t.Errorf("expected completed, got %s", record.Status)
	}
}

func TestSwarmManager_DepthLimit(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	// Default max depth is 3
	ctx := context.Background()
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:  "deep task",
		Depth: 3,
	})
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !strings.Contains(err.Error(), "max spawn depth") {
		t.Errorf("expected depth error, got: %v", err)
	}

	// Depth 2 should work (< 3)
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "ok task",
		Depth:         2,
		OriginChannel: "cli",
		OriginChatID:  "test",
	})
	if err != nil {
		t.Fatalf("depth 2 should be allowed: %v", err)
	}
}

func TestSwarmManager_ChildrenLimit(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	// Set max children to 2 for testing
	two := 2
	sm.SetDefaults(&config.SubagentsConfig{
		MaxChildrenPer: two,
	})

	ctx := context.Background()

	// Spawn a parent first
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "parent task",
		Label:         "parent",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("parent spawn failed: %v", err)
	}
	parentRunID := "swarm-1"

	// Spawn children under the parent
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child 1",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err != nil {
		t.Fatalf("child 1 spawn failed: %v", err)
	}

	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child 2",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err != nil {
		t.Fatalf("child 2 spawn failed: %v", err)
	}

	// Third child should fail
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child 3",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err == nil {
		t.Fatal("expected children limit error")
	}
	if !strings.Contains(err.Error(), "max children per parent") {
		t.Errorf("expected children limit error, got: %v", err)
	}
}

func TestSwarmManager_Kill(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	ctx := context.Background()

	// Spawn parent
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "parent",
		Label:         "parent",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	parentRunID := "swarm-1"

	// Spawn a child under the parent
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child",
		Label:         "child",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err != nil {
		t.Fatalf("child spawn failed: %v", err)
	}

	// Kill parent (should cascade to child)
	err = sm.Kill(parentRunID)
	if err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	parent, _ := sm.GetRun(parentRunID)
	if parent.Status != "canceled" {
		t.Errorf("expected parent canceled, got %s", parent.Status)
	}

	child, _ := sm.GetRun("swarm-2")
	if child.Status != "canceled" {
		t.Errorf("expected child canceled, got %s", child.Status)
	}

	// Kill non-existent run
	err = sm.Kill("swarm-999")
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

func TestSwarmManager_DrainAnnouncements(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	ctx := context.Background()

	// Spawn a task that will complete and auto-announce
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "announce me",
		Label:         "announcer",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	announcements := sm.DrainAnnouncements()
	if len(announcements) == 0 {
		t.Fatal("expected at least one announcement")
	}
	if !strings.Contains(announcements[0], "announcer") {
		t.Errorf("expected announcement to contain label, got: %s", announcements[0])
	}

	// Drain again should be empty
	second := sm.DrainAnnouncements()
	if len(second) != 0 {
		t.Errorf("expected empty after drain, got %d", len(second))
	}
}

func TestSwarmManager_DrainAnnouncements_Disabled(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	// Disable auto-announce
	f := false
	sm.SetDefaults(&config.SubagentsConfig{
		AutoAnnounce: &f,
	})

	ctx := context.Background()
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "quiet task",
		Label:         "quiet",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	announcements := sm.DrainAnnouncements()
	if len(announcements) != 0 {
		t.Errorf("expected no announcements when disabled, got %d", len(announcements))
	}
}

func TestSwarmManager_ListRuns(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	ctx := context.Background()

	// Spawn parent
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "parent",
		Label:         "parent",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("parent spawn failed: %v", err)
	}

	parentRunID := "swarm-1"

	// Spawn children under parent
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child 1",
		Label:         "c1",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err != nil {
		t.Fatalf("child1 spawn failed: %v", err)
	}

	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "child 2",
		Label:         "c2",
		ParentRunID:   parentRunID,
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         1,
	})
	if err != nil {
		t.Fatalf("child2 spawn failed: %v", err)
	}

	// Spawn unrelated task (no parent)
	_, err = sm.Spawn(ctx, SpawnParams{
		Task:          "orphan",
		Label:         "orphan",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("orphan spawn failed: %v", err)
	}

	// List all runs
	all := sm.ListRuns("")
	if len(all) != 4 {
		t.Errorf("expected 4 total runs, got %d", len(all))
	}

	// List runs for parent
	children := sm.ListRuns(parentRunID)
	if len(children) != 2 {
		t.Errorf("expected 2 children for parent, got %d", len(children))
	}
	for _, c := range children {
		if c.ParentRunID != parentRunID {
			t.Errorf("expected parent %s, got %s", parentRunID, c.ParentRunID)
		}
	}
}

func TestSwarmManager_SetAgentConfigs(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	configs := map[string]*config.AgentConfig{
		"researcher": {
			ID:       "researcher",
			Identity: "You are a research specialist.",
		},
	}
	sm.SetAgentConfigs(configs)

	prompt := sm.buildSystemPrompt("researcher")
	if !strings.Contains(prompt, "research specialist") {
		t.Errorf("expected identity in prompt, got: %s", prompt)
	}

	// Unknown agent should get generic prompt
	prompt = sm.buildSystemPrompt("unknown")
	if !strings.Contains(prompt, "sub-agent 'unknown'") {
		t.Errorf("expected generic prompt for unknown agent, got: %s", prompt)
	}

	// Empty agent ID
	prompt = sm.buildSystemPrompt("")
	if !strings.Contains(prompt, "You are a sub-agent") {
		t.Errorf("expected default prompt for empty agent, got: %s", prompt)
	}
}

func TestSwarmManager_Close(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	ctx := context.Background()
	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "long task",
		Label:         "worker",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	// Close should cancel all running tasks
	sm.Close()

	record, _ := sm.GetRun("swarm-1")
	// The task may have completed before Close() or been canceled
	if record.Status != "completed" && record.Status != "canceled" {
		t.Errorf("expected completed or canceled after close, got %s", record.Status)
	}
}

func TestSwarmManager_Callback(t *testing.T) {
	provider := &MockLLMProvider{}
	sm := NewSwarmManager(provider, "test-model", "/tmp/test")

	callbackCh := make(chan *ToolResult, 1)
	ctx := context.Background()

	_, err := sm.Spawn(ctx, SpawnParams{
		Task:          "callback task",
		Label:         "cb-test",
		OriginChannel: "cli",
		OriginChatID:  "test",
		Depth:         0,
		Callback: func(_ context.Context, result *ToolResult) {
			callbackCh <- result
		},
	})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	select {
	case result := <-callbackCh:
		if result.IsError {
			t.Errorf("expected success callback, got error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "cb-test") {
			t.Errorf("expected label in callback result, got: %s", result.ForLLM)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("callback not received within timeout")
	}
}

func TestApplyFilter_AllowList(t *testing.T) {
	reg := NewToolRegistry()

	// Register some tools
	reg.Register(&dummyTool{name: "read_file"})
	reg.Register(&dummyTool{name: "write_file"})
	reg.Register(&dummyTool{name: "web_search"})
	reg.Register(&dummyTool{name: "exec"})

	// Apply allow filter
	reg.ApplyFilter([]string{"read_file", "web_search"}, nil)

	// Only allowed tools should remain
	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should be allowed")
	}
	if _, ok := reg.Get("web_search"); !ok {
		t.Error("web_search should be allowed")
	}
	if _, ok := reg.Get("write_file"); ok {
		t.Error("write_file should be filtered out")
	}
	if _, ok := reg.Get("exec"); ok {
		t.Error("exec should be filtered out")
	}
}

func TestApplyFilter_DenyList(t *testing.T) {
	reg := NewToolRegistry()

	reg.Register(&dummyTool{name: "read_file"})
	reg.Register(&dummyTool{name: "exec"})
	reg.Register(&dummyTool{name: "web_search"})

	// Deny exec only
	reg.ApplyFilter(nil, []string{"exec"})

	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should remain")
	}
	if _, ok := reg.Get("web_search"); !ok {
		t.Error("web_search should remain")
	}
	if _, ok := reg.Get("exec"); ok {
		t.Error("exec should be denied")
	}
}

func TestApplyFilter_AllowAndDeny(t *testing.T) {
	reg := NewToolRegistry()

	reg.Register(&dummyTool{name: "read_file"})
	reg.Register(&dummyTool{name: "write_file"})
	reg.Register(&dummyTool{name: "exec"})

	// Allow read_file and exec, but deny exec
	reg.ApplyFilter([]string{"read_file", "exec"}, []string{"exec"})

	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should be allowed")
	}
	if _, ok := reg.Get("exec"); ok {
		t.Error("exec should be denied even though allowed")
	}
	if _, ok := reg.Get("write_file"); ok {
		t.Error("write_file should be filtered out by allow")
	}
}

func TestApplyFilter_DoesNotAffectHiddenTools(t *testing.T) {
	reg := NewToolRegistry()

	reg.Register(&dummyTool{name: "read_file"})
	reg.RegisterHidden(&dummyTool{name: "hidden_tool"})

	// Only allow read_file — hidden_tool should not be deleted
	reg.ApplyFilter([]string{"read_file"}, nil)

	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should remain")
	}

	// Hidden tools are not visible via Get (TTL=0), but they should still be in registry
	names := reg.List()
	found := false
	for _, name := range names {
		if name == "hidden_tool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("hidden_tool should not be removed by ApplyFilter")
	}
}

func TestApplyFilter_SharedToolsFiltered(t *testing.T) {
	// Simulates the real scenario: tools registered in NewAgentInstance, then
	// shared tools registered in registerSharedTools, then filter re-applied.
	reg := NewToolRegistry()

	// Core tools (from NewAgentInstance)
	reg.Register(&dummyTool{name: "read_file"})
	reg.Register(&dummyTool{name: "write_file"})

	// Shared tools (from registerSharedTools - added later)
	reg.Register(&dummyTool{name: "web_search"})
	reg.Register(&dummyTool{name: "delegate"})
	reg.Register(&dummyTool{name: "project_board"})

	// Re-apply filter (as done after shared tools)
	reg.ApplyFilter([]string{"read_file", "web_search"}, nil)

	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should be allowed")
	}
	if _, ok := reg.Get("web_search"); !ok {
		t.Error("web_search should be allowed")
	}
	if _, ok := reg.Get("write_file"); ok {
		t.Error("write_file should be filtered out")
	}
	if _, ok := reg.Get("delegate"); ok {
		t.Error("sessions_spawn should be filtered out")
	}
	if _, ok := reg.Get("project_board"); ok {
		t.Error("project_board should be filtered out")
	}
}

// dummyTool is a minimal Tool implementation for filter tests.
type dummyTool struct {
	name string
}

func (t *dummyTool) Name() string                                            { return t.name }
func (t *dummyTool) Description() string                                     { return "test tool" }
func (t *dummyTool) Parameters() map[string]any                              { return nil }
func (t *dummyTool) Execute(_ context.Context, _ map[string]any) *ToolResult { return nil }
