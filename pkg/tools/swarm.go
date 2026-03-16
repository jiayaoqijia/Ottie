package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/config"
	"github.com/jiayaoqijia/ottie/pkg/logger"
	"github.com/jiayaoqijia/ottie/pkg/providers"
)

// SpawnParams holds parameters for spawning a sub-agent via SwarmManager.
type SpawnParams struct {
	Task          string
	Label         string
	AgentID       string
	OriginChannel string
	OriginChatID  string
	ParentRunID   string
	Depth         int
	Callback      AsyncCallback
}

// SubagentRunRecord tracks a single sub-agent execution.
type SubagentRunRecord struct {
	RunID       string
	ParentRunID string
	AgentID     string
	Label       string
	Task        string
	Status      string // running, completed, failed, canceled
	Outcome     string
	Depth       int
	Children    []string
	CancelFunc  context.CancelFunc
	CreatedAt   time.Time
	CompletedAt time.Time
}

// SwarmManager orchestrates sub-agent spawning with depth limits,
// children limits, cascading cancellation, and announcement queuing.
type SwarmManager struct {
	runs       map[string]*SubagentRunRecord
	childIndex map[string][]string // parentRunID -> childRunIDs
	mu         sync.RWMutex

	provider       providers.LLMProvider
	defaultModel   string
	workspace      string
	tools          *ToolRegistry
	maxIterations  int
	maxTokens      int
	temperature    float64
	hasMaxTokens   bool
	hasTemperature bool
	forceStream    bool
	nextID         int

	// Agent configs for identity/role lookup when spawning sub-agents
	agentConfigs map[string]*config.AgentConfig

	// Default spawn limits (from orchestrator's SubagentsConfig)
	defaultMaxDepth     int
	defaultMaxChildren  int
	defaultAutoAnnounce bool

	// Pending announcements for parent agent's next LLM turn
	pendingAnnouncements []string
	announceMu           sync.Mutex
}

func NewSwarmManager(provider providers.LLMProvider, defaultModel, workspace string) *SwarmManager {
	return &SwarmManager{
		runs:                make(map[string]*SubagentRunRecord),
		childIndex:          make(map[string][]string),
		provider:            provider,
		defaultModel:        defaultModel,
		workspace:           workspace,
		tools:               NewToolRegistry(),
		maxIterations:       10,
		nextID:              1,
		agentConfigs:        make(map[string]*config.AgentConfig),
		defaultMaxDepth:     3,
		defaultMaxChildren:  5,
		defaultAutoAnnounce: true,
	}
}

// SetAgentConfigs provides agent configurations for identity/role lookup.
func (sm *SwarmManager) SetAgentConfigs(configs map[string]*config.AgentConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.agentConfigs = configs
}

// SetDefaults configures default spawn limits from SubagentsConfig.
func (sm *SwarmManager) SetDefaults(sub *config.SubagentsConfig) {
	if sub == nil {
		return
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sub.MaxSpawnDepth > 0 {
		sm.defaultMaxDepth = sub.MaxSpawnDepth
	}
	if sub.MaxChildrenPer > 0 {
		sm.defaultMaxChildren = sub.MaxChildrenPer
	}
	if sub.AutoAnnounce != nil {
		sm.defaultAutoAnnounce = *sub.AutoAnnounce
	}
}

// SetLLMOptions sets max tokens, temperature, and force_stream for sub-agent LLM calls.
func (sm *SwarmManager) SetLLMOptions(maxTokens int, temperature *float64, forceStream bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxTokens = maxTokens
	sm.hasMaxTokens = true
	sm.forceStream = forceStream
	if temperature != nil {
		sm.temperature = *temperature
		sm.hasTemperature = true
	}
}

// SetTools sets the tool registry for sub-agent execution.
func (sm *SwarmManager) SetTools(tools *ToolRegistry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools = tools
}

// Spawn creates and starts a new sub-agent run.
func (sm *SwarmManager) Spawn(ctx context.Context, params SpawnParams) (string, error) {
	sm.mu.Lock()

	// Validate depth
	if params.Depth >= sm.defaultMaxDepth {
		sm.mu.Unlock()
		return "", fmt.Errorf("max spawn depth %d exceeded (current depth: %d)", sm.defaultMaxDepth, params.Depth)
	}

	// Validate children count for parent
	if params.ParentRunID != "" {
		children := sm.childIndex[params.ParentRunID]
		if len(children) >= sm.defaultMaxChildren {
			sm.mu.Unlock()
			return "", fmt.Errorf(
				"max children per parent %d exceeded for run %s",
				sm.defaultMaxChildren,
				params.ParentRunID,
			)
		}
	}

	runID := fmt.Sprintf("swarm-%d", sm.nextID)
	sm.nextID++

	runCtx, cancel := context.WithCancel(ctx)

	record := &SubagentRunRecord{
		RunID:       runID,
		ParentRunID: params.ParentRunID,
		AgentID:     params.AgentID,
		Label:       params.Label,
		Task:        params.Task,
		Status:      "running",
		Depth:       params.Depth,
		CancelFunc:  cancel,
		CreatedAt:   time.Now(),
	}

	sm.runs[runID] = record
	if params.ParentRunID != "" {
		sm.childIndex[params.ParentRunID] = append(sm.childIndex[params.ParentRunID], runID)
	}

	// Snapshot config for the goroutine
	toolsSnapshot := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	hasMaxTokens := sm.hasMaxTokens
	hasTemperature := sm.hasTemperature
	forceStream := sm.forceStream
	autoAnnounce := sm.defaultAutoAnnounce

	// Build system prompt from agent config if available
	systemPrompt := sm.buildSystemPrompt(params.AgentID)

	sm.mu.Unlock()

	go sm.runSpawnedTask(
		runCtx,
		record,
		systemPrompt,
		toolsSnapshot,
		maxIter,
		maxTokens,
		temperature,
		hasMaxTokens,
		hasTemperature,
		forceStream,
		autoAnnounce,
		params,
	)

	label := params.Label
	if label == "" {
		label = params.AgentID
	}
	return fmt.Sprintf(
		"Spawned sub-agent '%s' (run: %s, depth: %d) for task: %s",
		label,
		runID,
		params.Depth,
		params.Task,
	), nil
}

func (sm *SwarmManager) buildSystemPrompt(agentID string) string {
	if agentID == "" {
		return "You are a sub-agent. Complete the given task independently and report the result.\nYou have access to tools - use them as needed. Provide a clear summary when done."
	}

	cfg, ok := sm.agentConfigs[agentID]
	if !ok || cfg.Identity == "" {
		return fmt.Sprintf(
			"You are sub-agent '%s'. Complete the given task independently and report the result.\nYou have access to tools - use them as needed. Provide a clear summary when done.",
			agentID,
		)
	}

	return cfg.Identity + "\n\nComplete the given task and report back with a clear summary."
}

func (sm *SwarmManager) runSpawnedTask(
	ctx context.Context,
	record *SubagentRunRecord,
	systemPrompt string,
	toolsSnapshot *ToolRegistry,
	maxIter, maxTokens int,
	temperature float64,
	hasMaxTokens, hasTemperature, forceStream, autoAnnounce bool,
	params SpawnParams,
) {
	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: params.Task},
	}

	var llmOptions map[string]any
	if hasMaxTokens || hasTemperature || forceStream {
		llmOptions = make(map[string]any)
		if hasMaxTokens {
			llmOptions["max_tokens"] = maxTokens
		}
		if hasTemperature {
			llmOptions["temperature"] = temperature
		}
		if forceStream {
			llmOptions["force_stream"] = true
		}
	}

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:      sm.provider,
		Model:         sm.defaultModel,
		Tools:         toolsSnapshot,
		MaxIterations: maxIter,
		LLMOptions:    llmOptions,
	}, messages, params.OriginChannel, params.OriginChatID)

	sm.mu.Lock()
	var result *ToolResult
	defer func() {
		sm.mu.Unlock()
		if params.Callback != nil && result != nil {
			params.Callback(ctx, result)
		}
	}()

	if err != nil {
		record.Status = "failed"
		record.Outcome = fmt.Sprintf("Error: %v", err)
		record.CompletedAt = time.Now()

		if ctx.Err() != nil {
			record.Status = "canceled"
			record.Outcome = "Task canceled"
		}

		result = &ToolResult{
			ForLLM:  record.Outcome,
			IsError: true,
			Err:     err,
		}
	} else {
		record.Status = "completed"
		record.Outcome = loopResult.Content
		record.CompletedAt = time.Now()

		label := record.Label
		if label == "" {
			label = record.AgentID
		}
		if label == "" {
			label = record.RunID
		}

		result = &ToolResult{
			ForLLM: fmt.Sprintf("Sub-agent '%s' completed (run: %s, iterations: %d): %s",
				label, record.RunID, loopResult.Iterations, loopResult.Content),
			ForUser: loopResult.Content,
		}
	}

	// Queue announcement for parent's next LLM turn
	if autoAnnounce && record.Status == "completed" {
		label := record.Label
		if label == "" {
			label = record.RunID
		}
		announcement := fmt.Sprintf("[Sub-agent Result] %s: %s", label, record.Outcome)
		sm.announceMu.Lock()
		sm.pendingAnnouncements = append(sm.pendingAnnouncements, announcement)
		sm.announceMu.Unlock()
	}

	logger.InfoCF("swarm", "Sub-agent completed",
		map[string]any{
			"run_id": record.RunID,
			"status": record.Status,
			"agent":  record.AgentID,
			"depth":  record.Depth,
		})
}

// Kill cancels a run and all its children recursively.
func (sm *SwarmManager) Kill(runID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	record, ok := sm.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}

	sm.killRecursive(record)
	return nil
}

func (sm *SwarmManager) killRecursive(record *SubagentRunRecord) {
	if record.CancelFunc != nil {
		record.CancelFunc()
	}
	record.Status = "canceled"
	record.CompletedAt = time.Now()

	for _, childID := range sm.childIndex[record.RunID] {
		if child, ok := sm.runs[childID]; ok {
			sm.killRecursive(child)
		}
	}
}

// Steer injects a message into a running sub-agent (placeholder for future).
func (sm *SwarmManager) Steer(runID, message string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	record, ok := sm.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}
	if record.Status != "running" {
		return fmt.Errorf("run %q is not running (status: %s)", runID, record.Status)
	}

	// Steering would require injecting messages into the running tool loop.
	// For now, log the intent. Full implementation requires a message channel on the run.
	logger.InfoCF("swarm", "Steer requested (not yet implemented for active runs)",
		map[string]any{"run_id": runID, "message": message})
	return nil
}

// ListRuns returns runs filtered by parent. Pass empty string for all runs.
func (sm *SwarmManager) ListRuns(parentRunID string) []*SubagentRunRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []*SubagentRunRecord
	for _, record := range sm.runs {
		if parentRunID == "" || record.ParentRunID == parentRunID {
			result = append(result, record)
		}
	}
	return result
}

// GetRun returns a single run by ID.
func (sm *SwarmManager) GetRun(runID string) (*SubagentRunRecord, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	r, ok := sm.runs[runID]
	return r, ok
}

// DrainAnnouncements atomically returns and clears pending announcements.
func (sm *SwarmManager) DrainAnnouncements() []string {
	sm.announceMu.Lock()
	defer sm.announceMu.Unlock()

	if len(sm.pendingAnnouncements) == 0 {
		return nil
	}

	result := sm.pendingAnnouncements
	sm.pendingAnnouncements = nil
	return result
}

// Close cancels all running sub-agents.
func (sm *SwarmManager) Close() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, record := range sm.runs {
		if record.Status == "running" && record.CancelFunc != nil {
			record.CancelFunc()
			record.Status = "canceled"
			record.CompletedAt = time.Now()
		}
	}
}
