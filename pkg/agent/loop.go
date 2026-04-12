// Ottie - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Ottie contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jiayaoqijia/ottie/pkg/acs"
	"github.com/jiayaoqijia/ottie/pkg/bus"
	"github.com/jiayaoqijia/ottie/pkg/channels"
	"github.com/jiayaoqijia/ottie/pkg/commands"
	"github.com/jiayaoqijia/ottie/pkg/config"
	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
	"github.com/jiayaoqijia/ottie/pkg/constants"
	"github.com/jiayaoqijia/ottie/pkg/infra"
	"github.com/jiayaoqijia/ottie/pkg/logger"
	"github.com/jiayaoqijia/ottie/pkg/media"
	"github.com/jiayaoqijia/ottie/pkg/providers"
	"github.com/jiayaoqijia/ottie/pkg/routing"
	"github.com/jiayaoqijia/ottie/pkg/skills"
	"github.com/jiayaoqijia/ottie/pkg/state"
	"github.com/jiayaoqijia/ottie/pkg/swarm/board"
	"github.com/jiayaoqijia/ottie/pkg/tools"
	"github.com/jiayaoqijia/ottie/pkg/utils"
	"github.com/jiayaoqijia/ottie/pkg/voice"
)

type AgentLoop struct {
	bus            *bus.MessageBus
	cfg            *config.Config
	registry       *AgentRegistry
	state          *state.Manager
	running        atomic.Bool
	summarizing    sync.Map
	fallback       *providers.FallbackChain
	channelManager *channels.Manager
	mediaStore     media.MediaStore
	transcriber    voice.Transcriber
	cmdRegistry    *commands.Registry
	mcp            mcpRuntime
	mu             sync.RWMutex
	// Track active requests for safe provider cleanup
	activeRequests sync.WaitGroup
	// Message deduplication cache
	dedupe *infra.DedupeCache
	// Inbound message debouncer (nil if disabled)
	debouncer *infra.Debouncer
	// Shared project board for multi-bot coordination (nil when swarm disabled)
	projectBoard board.ProjectBoard
	// R11 ACS bundle — optional replay coordinator (pkg/principal
	// + pkg/actionlog + pkg/execmanifest). When cfg.ACS.Enabled is
	// false this is nil and every ACS hook in the loop is a no-op,
	// preserving bit-for-bit identical pre-R11 behavior.
	acs *acs.Bundle
	// acsTurnCounters maps session_key -> *atomic.Int64 for the
	// monotonic per-session turn counter used by the execmanifest
	// UNIQUE(session_id, turn) constraint. The counter is lazy-
	// initialized from MaxTurn() on first use per session so it
	// survives process restart AND history-summarization without
	// colliding with older rows. Codex R11 caught that the
	// previous `len(history)` proxy resets after summarization
	// and produces duplicate (session_id, turn) pairs that
	// silently disable ACS capture for every subsequent turn.
	acsTurnCounters sync.Map
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string   // Session identifier for history/context
	Channel         string   // Target channel for tool execution
	ChatID          string   // Target chat ID for tool execution
	UserMessage     string   // User message content (may include prefix)
	Media           []string // media:// refs from inbound message
	DefaultResponse string   // Response when LLM returns empty
	EnableSummary   bool     // Whether to trigger summarization
	SendResponse    bool     // Whether to send response via bus
	NoHistory       bool     // If true, don't load session history (for heartbeat)
}

const (
	defaultResponse           = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."
	sessionKeyAgentPrefix     = "agent:"
	metadataKeyAccountID      = "account_id"
	metadataKeyGuildID        = "guild_id"
	metadataKeyTeamID         = "team_id"
	metadataKeyParentPeerKind = "parent_peer_kind"
	metadataKeyParentPeerID   = "parent_peer_id"
)

func NewAgentLoop(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	provider providers.LLMProvider,
) *AgentLoop {
	registry := NewAgentRegistry(cfg, provider)

	// Create shared project board for multi-bot coordination (nil when swarm disabled)
	var projectBoard board.ProjectBoard
	if cfg.Swarm.Enabled {
		projectBoard = board.NewMemoryBoard()
	}

	// Register shared tools to all agents
	registerSharedTools(cfg, msgBus, registry, provider, projectBoard)

	// Set up shared fallback chain
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// Create state manager using default agent's workspace for channel recording
	defaultAgent := registry.GetDefaultAgent()
	var stateManager *state.Manager
	if defaultAgent != nil {
		stateManager = state.NewManager(defaultAgent.Workspace)
	}

	al := &AgentLoop{
		bus:          msgBus,
		cfg:          cfg,
		registry:     registry,
		state:        stateManager,
		summarizing:  sync.Map{},
		fallback:     fallbackChain,
		cmdRegistry:  commands.NewRegistry(commands.BuiltinDefinitions()),
		dedupe:       infra.NewDedupeCache(0, 0), // defaults: 20min TTL, 5000 max
		projectBoard: projectBoard,
	}

	// Initialize debouncer if configured
	if cfg.Session.DebounceMs > 0 {
		delay := time.Duration(cfg.Session.DebounceMs) * time.Millisecond
		al.debouncer = infra.NewDebouncer(delay, func(msg bus.InboundMessage) {
			al.processAndRespond(context.Background(), msg)
		})
	}

	// R11 ACS bundle: opened only when the config flag is on. The
	// ACS-off path leaves al.acs nil so every hook in runAgentLoop
	// / runLLMIteration is a cheap nil-check that matches the
	// pre-R11 behavior exactly.
	if cfg.ACS.Enabled {
		dbDir := cfg.ACS.DBDir
		if dbDir == "" && defaultAgent != nil {
			dbDir = filepath.Join(defaultAgent.Workspace, "acs")
		}
		if dbDir != "" {
			if bundle, err := acs.Open(acs.Config{
				DBDir:           dbDir,
				WriteQueueDepth: cfg.ACS.WriteQueueDepth,
			}); err != nil {
				logger.WarnCF("agent", "ACS bundle failed to open; continuing with ACS disabled",
					map[string]any{"error": err.Error(), "db_dir": dbDir})
			} else {
				al.acs = bundle
				logger.InfoCF("agent", "ACS bundle opened",
					map[string]any{"db_dir": dbDir})
			}
		}
	}

	return al
}

// registerSharedTools registers tools that are shared across all agents (web, message, spawn).
func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	provider providers.LLMProvider,
	projectBoard board.ProjectBoard,
) {
	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		if cfg.Tools.IsToolEnabled("web") {
			searchTool, err := tools.NewWebSearchTool(tools.WebSearchToolOptions{
				BraveAPIKeys:         config.MergeAPIKeys(cfg.Tools.Web.Brave.APIKey, cfg.Tools.Web.Brave.APIKeys),
				BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
				BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
				TavilyAPIKeys:        config.MergeAPIKeys(cfg.Tools.Web.Tavily.APIKey, cfg.Tools.Web.Tavily.APIKeys),
				TavilyBaseURL:        cfg.Tools.Web.Tavily.BaseURL,
				TavilyMaxResults:     cfg.Tools.Web.Tavily.MaxResults,
				TavilyEnabled:        cfg.Tools.Web.Tavily.Enabled,
				DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
				DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
				PerplexityAPIKeys: config.MergeAPIKeys(
					cfg.Tools.Web.Perplexity.APIKey,
					cfg.Tools.Web.Perplexity.APIKeys,
				),
				PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
				PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
				SearXNGBaseURL:       cfg.Tools.Web.SearXNG.BaseURL,
				SearXNGMaxResults:    cfg.Tools.Web.SearXNG.MaxResults,
				SearXNGEnabled:       cfg.Tools.Web.SearXNG.Enabled,
				GLMSearchAPIKey:      cfg.Tools.Web.GLMSearch.APIKey,
				GLMSearchBaseURL:     cfg.Tools.Web.GLMSearch.BaseURL,
				GLMSearchEngine:      cfg.Tools.Web.GLMSearch.SearchEngine,
				GLMSearchMaxResults:  cfg.Tools.Web.GLMSearch.MaxResults,
				GLMSearchEnabled:     cfg.Tools.Web.GLMSearch.Enabled,
				Proxy:                cfg.Tools.Web.Proxy,
			})
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web search tool", map[string]any{"error": err.Error()})
			} else if searchTool != nil {
				agent.Tools.Register(searchTool)
			}
		}
		if cfg.Tools.IsToolEnabled("web_fetch") {
			fetchTool, err := tools.NewWebFetchToolWithProxy(50000, cfg.Tools.Web.Proxy, cfg.Tools.Web.FetchLimitBytes)
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web fetch tool", map[string]any{"error": err.Error()})
			} else {
				agent.Tools.Register(fetchTool)
			}
		}

		// Message tool
		if cfg.Tools.IsToolEnabled("message") {
			messageTool := tools.NewMessageTool()
			messageTool.SetSendCallback(func(channel, chatID, content string) error {
				pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer pubCancel()
				return msgBus.PublishOutbound(pubCtx, bus.OutboundMessage{
					Channel: channel,
					ChatID:  chatID,
					Content: content,
				})
			})
			agent.Tools.Register(messageTool)
		}

		// Send file tool (outbound media via MediaStore — store injected later by SetMediaStore)
		if cfg.Tools.IsToolEnabled("send_file") {
			sendFileTool := tools.NewSendFileTool(
				agent.Workspace,
				cfg.Agents.Defaults.RestrictToWorkspace,
				cfg.Agents.Defaults.GetMaxMediaSize(),
				nil,
			)
			agent.Tools.Register(sendFileTool)
		}

		// Render HTML tool (HTML-to-PNG via headless Chromium — store injected later by SetMediaStore)
		// Only register if Chromium/Chrome is actually available; exposing the tool
		// without a browser causes models (especially local ones like Ollama) to
		// attempt huge HTML tool calls that fail with parse errors.
		if cfg.Tools.IsToolEnabled("render_html") && tools.HasBrowser() {
			renderTool := tools.NewRenderHTMLTool(
				agent.Workspace,
				tools.RenderHTMLConfig{
					ViewportWidth:  cfg.Tools.RenderHTML.ViewportWidth,
					ViewportHeight: cfg.Tools.RenderHTML.ViewportHeight,
					Timeout:        time.Duration(cfg.Tools.RenderHTML.TimeoutSeconds) * time.Second,
				},
			)
			agent.Tools.Register(renderTool)
		}

		// Skill discovery and installation tools
		skills_enabled := cfg.Tools.IsToolEnabled("skills")
		find_skills_enable := cfg.Tools.IsToolEnabled("find_skills")
		install_skills_enable := cfg.Tools.IsToolEnabled("install_skill")
		if skills_enabled && (find_skills_enable || install_skills_enable) {
			registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
				MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
				ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
			})

			// Add local index registry for instant offline skill search.
			if cfg.Tools.Skills.Registries.LocalIndex.Enabled {
				clawhubReg := registryMgr.GetRegistry("clawhub")
				localReg := skills.NewLocalIndexRegistry(skills.LocalIndexConfig{
					Enabled:   true,
					IndexPath: cfg.Tools.Skills.Registries.LocalIndex.IndexPath,
					Fallback:  clawhubReg,
				})
				if err := localReg.Load(); err != nil {
					logger.WarnCF("agent", "failed to load local skill index", map[string]any{"error": err})
				}
				registryMgr.AddRegistry(localReg)

				// Sync local index from ClawHub in background on startup.
				if clawhubReg != nil && cfg.Tools.Skills.Registries.LocalIndex.SyncOnStartup {
					go func() {
						syncCtx, syncCancel := context.WithTimeout(context.Background(), 2*time.Minute)
						defer syncCancel()
						count, err := localReg.Sync(syncCtx, clawhubReg)
						if err != nil {
							logger.WarnCF("agent", "background skill index sync failed", map[string]any{"error": err})
						} else {
							logger.InfoCF("agent", "skill index synced on startup", map[string]any{"count": count})
						}
					}()
				}
			}

			if find_skills_enable {
				searchCache := skills.NewSearchCache(
					cfg.Tools.Skills.SearchCache.MaxSize,
					time.Duration(cfg.Tools.Skills.SearchCache.TTLSeconds)*time.Second,
				)
				agent.Tools.Register(tools.NewFindSkillsTool(registryMgr, searchCache))
			}

			if install_skills_enable {
				agent.Tools.Register(tools.NewInstallSkillTool(registryMgr, agent.Workspace))
			}
		}

		// delegate tool: unified surface for both Mode A (SubagentManager) and
		// Mode B (SwarmManager). Orchestrator agents with subagents configured
		// get the swarm-backed implementation; everyone else gets the local
		// in-process SubagentManager. The tool is always called "delegate" so
		// the model surface is identical regardless of backend.
		if cfg.Tools.IsToolEnabled("delegate") || cfg.Tools.IsToolEnabled("spawn") {
			if agent.Subagents != nil {
				// Orchestrator path: SwarmManager-backed delegate registered
				// by registerSwarmTools below.
			} else if cfg.Tools.IsToolEnabled("subagent") {
				// Local in-process delegate path.
				subagentManager := tools.NewSubagentManager(provider, agent.Model, agent.Workspace)
				subagentManager.SetLLMOptions(agent.MaxTokens, agent.Temperature)
				spawnTool := tools.NewSpawnTool(subagentManager)
				currentAgentID := agentID
				spawnTool.SetAllowlistChecker(func(targetAgentID string) bool {
					return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
				})
				agent.Tools.Register(spawnTool)
			} else {
				logger.WarnCF("agent", "delegate tool requires subagent to be enabled", nil)
			}
		}

		// Swarm path: registerSwarmTools registers the SwarmManager-backed
		// delegate for orchestrator agents with subagents configured.
		registerSwarmTools(cfg, agent, agentID, provider, registry)

		// ProjectBoard tool (Mode B): register when swarm is enabled.
		if cfg.Swarm.Enabled && projectBoard != nil {
			boardTool := tools.NewProjectBoardTool(projectBoard, cfg.Swarm.InstanceID)
			agent.Tools.Register(boardTool)
			logger.InfoCF("agent", "Registered project_board tool (swarm mode)",
				map[string]any{"agent_id": agentID, "instance_id": cfg.Swarm.InstanceID})
		}

		// Re-apply per-agent tool filters after all shared tools are registered.
		// This ensures tools_allow/tools_deny in agent config also apply to
		// shared tools (web, spawn, swarm, project_board).
		for i := range cfg.Agents.List {
			ac := &cfg.Agents.List[i]
			if routing.NormalizeAgentID(ac.ID) == agentID {
				if len(ac.ToolsAllow) > 0 || len(ac.ToolsDeny) > 0 {
					agent.Tools.ApplyFilter(ac.ToolsAllow, ac.ToolsDeny)
				}
				break
			}
		}
	}
}

// registerSwarmTools registers swarm tools for agents with orchestrator role or subagents configured.
func registerSwarmTools(
	cfg *config.Config,
	agent *AgentInstance,
	agentID string,
	provider providers.LLMProvider,
	registry *AgentRegistry,
) {
	if agent.Subagents == nil {
		return
	}

	// Only agents with subagents configured (orchestrators) get swarm tools
	swarmMgr := tools.NewSwarmManager(provider, agent.Model, agent.Workspace)
	swarmMgr.SetLLMOptions(agent.MaxTokens, agent.Temperature, agent.ForceStream)
	swarmMgr.SetTools(agent.Tools)
	swarmMgr.SetDefaults(agent.Subagents)

	// Build agent configs map for identity lookup
	agentConfigs := make(map[string]*config.AgentConfig)
	for i := range cfg.Agents.List {
		ac := &cfg.Agents.List[i]
		agentConfigs[ac.ID] = ac
	}
	swarmMgr.SetAgentConfigs(agentConfigs)

	agent.SwarmManager = swarmMgr

	// Register swarm-backed delegate tool (same Name() as local delegate).
	delegateTool := tools.NewSessionsSpawnTool(swarmMgr)
	currentAgentID := agentID
	delegateTool.SetAllowlistChecker(func(targetAgentID string) bool {
		return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
	})
	agent.Tools.Register(delegateTool)

	// Register sessions_control tool (swarm-only; has no local equivalent).
	controlTool := tools.NewSessionsControlTool(swarmMgr)
	agent.Tools.Register(controlTool)

	logger.InfoCF("agent", "Registered swarm tools (delegate, sessions_control)",
		map[string]any{"agent_id": agentID})
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	if err := al.ensureMCPInitialized(ctx); err != nil {
		return err
	}

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			// Deduplicate: skip messages already seen within TTL
			if msg.MessageID != "" {
				dedupeKey := msg.Channel + "|" + msg.SenderID + "|" + msg.ChatID + "|" + msg.MessageID
				if al.dedupe.Check(dedupeKey) {
					logger.DebugCF("agent", "Duplicate message skipped", map[string]any{
						"channel":    msg.Channel,
						"message_id": msg.MessageID,
					})
					continue
				}
			}

			// Debounce: if enabled, group rapid messages and process only the latest
			if al.debouncer != nil {
				debounceKey := msg.Channel + ":" + msg.ChatID
				al.debouncer.Submit(debounceKey, msg)
				continue
			}

			al.processAndRespond(ctx, msg)
		}
	}

	return nil
}

// processAndRespond processes a single inbound message and publishes the response.
// Extracted from Run() so the debouncer callback can also invoke it.
func (al *AgentLoop) processAndRespond(ctx context.Context, msg bus.InboundMessage) {
	response, err := al.processMessage(ctx, msg)
	if err != nil {
		response = providers.UserFacingError(err)
	}

	if response != "" {
		// Check if the message tool already sent a response during this round.
		// If so, skip publishing to avoid duplicate messages to the user.
		// Use default agent's tools to check (message tool is shared).
		alreadySent := false
		defaultAgent := al.GetRegistry().GetDefaultAgent()
		if defaultAgent != nil {
			if tool, ok := defaultAgent.Tools.Get("message"); ok {
				if mt, ok := tool.(*tools.MessageTool); ok {
					alreadySent = mt.HasSentInRound()
				}
			}
		}

		if !alreadySent {
			al.bus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: response,
			})
			logger.InfoCF("agent", "Published outbound response",
				map[string]any{
					"channel":     msg.Channel,
					"chat_id":     msg.ChatID,
					"content_len": len(response),
				})
		} else {
			logger.DebugCF(
				"agent",
				"Skipped outbound (message tool already sent)",
				map[string]any{"channel": msg.Channel},
			)
		}
	}
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
}

// Close releases resources held by agent session stores. Call after Stop.
func (al *AgentLoop) Close() {
	if al.debouncer != nil {
		al.debouncer.Stop()
	}

	// Close the ACS bundle (if opened). The bundle's Close is
	// idempotent and drains in-flight ledger/manifest writes
	// before returning. Errors are logged, not returned, because
	// Close has no error-return signature and half-closed
	// subsystems are better than never-closed ones.
	if al.acs != nil {
		if err := al.acs.Close(); err != nil {
			logger.WarnCF("agent", "ACS bundle close error",
				map[string]any{"error": err.Error()})
		}
		al.acs = nil
	}

	mcpManager := al.mcp.takeManager()

	if mcpManager != nil {
		if err := mcpManager.Close(); err != nil {
			logger.ErrorCF("agent", "Failed to close MCP manager",
				map[string]any{
					"error": err.Error(),
				})
		}
	}

	if al.projectBoard != nil {
		if err := al.projectBoard.Close(); err != nil {
			logger.ErrorCF("agent", "Failed to close project board",
				map[string]any{"error": err.Error()})
		}
	}

	al.GetRegistry().Close()
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	registry := al.GetRegistry()
	for _, agentID := range registry.ListAgentIDs() {
		if agent, ok := registry.GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// ReloadProviderAndConfig atomically swaps the provider and config with proper synchronization.
// It uses a context to allow timeout control from the caller.
// Returns an error if the reload fails or context is canceled.
func (al *AgentLoop) ReloadProviderAndConfig(
	ctx context.Context,
	provider providers.LLMProvider,
	cfg *config.Config,
) error {
	// Validate inputs
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Create new registry with updated config and provider
	// Wrap in defer/recover to handle any panics gracefully
	var registry *AgentRegistry
	var panicErr error
	done := make(chan struct{}, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic during registry creation: %v", r)
				logger.ErrorCF("agent", "Panic during registry creation",
					map[string]any{"panic": r})
			}
			close(done)
		}()

		registry = NewAgentRegistry(cfg, provider)
	}()

	// Wait for completion or context cancellation
	select {
	case <-done:
		if registry == nil {
			if panicErr != nil {
				return fmt.Errorf("registry creation failed: %w", panicErr)
			}
			return fmt.Errorf("registry creation failed (nil result)")
		}
	case <-ctx.Done():
		return fmt.Errorf("context canceled during registry creation: %w", ctx.Err())
	}

	// Check context again before proceeding
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled after registry creation: %w", err)
	}

	// Ensure shared tools are re-registered on the new registry
	var reloadBoard board.ProjectBoard
	if cfg.Swarm.Enabled {
		reloadBoard = board.NewMemoryBoard()
	}
	registerSharedTools(cfg, al.bus, registry, provider, reloadBoard)

	// Atomically swap the config and registry under write lock
	// This ensures readers see a consistent pair
	al.mu.Lock()
	oldRegistry := al.registry
	oldBoard := al.projectBoard

	// Store new values
	al.cfg = cfg
	al.registry = registry
	al.projectBoard = reloadBoard

	// Also update fallback chain with new config
	al.fallback = providers.NewFallbackChain(providers.NewCooldownTracker())

	al.mu.Unlock()

	// Close old project board after releasing the lock
	if oldBoard != nil {
		_ = oldBoard.Close()
	}

	// Close old provider after releasing the lock
	// This prevents blocking readers while closing
	if oldProvider, ok := extractProvider(oldRegistry); ok {
		if stateful, ok := oldProvider.(providers.StatefulProvider); ok {
			// Give in-flight requests a moment to complete
			// Use a reasonable timeout that balances cleanup vs resource usage
			select {
			case <-time.After(100 * time.Millisecond):
				stateful.Close()
			case <-ctx.Done():
				// Context canceled, close immediately but log warning
				logger.WarnCF("agent", "Context canceled during provider cleanup, forcing close",
					map[string]any{"error": ctx.Err()})
				stateful.Close()
			}
		}
	}

	// Close old registry's agent resources (session stores, swarm managers)
	if oldRegistry != nil {
		oldRegistry.Close()
	}

	logger.InfoCF("agent", "Provider and config reloaded successfully",
		map[string]any{
			"model": cfg.Agents.Defaults.GetModelName(),
		})

	return nil
}

// GetRegistry returns the current registry (thread-safe)
func (al *AgentLoop) GetRegistry() *AgentRegistry {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.registry
}

// GetConfig returns the current config (thread-safe)
func (al *AgentLoop) GetConfig() *config.Config {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.cfg
}

// SetMediaStore injects a MediaStore for media lifecycle management.
func (al *AgentLoop) SetMediaStore(s media.MediaStore) {
	al.mediaStore = s

	// Propagate store to send_file and render_html tools in all agents.
	registry := al.GetRegistry()
	registry.ForEachTool("send_file", func(t tools.Tool) {
		if sf, ok := t.(*tools.SendFileTool); ok {
			sf.SetMediaStore(s)
		}
	})
	registry.ForEachTool("render_html", func(t tools.Tool) {
		if rt, ok := t.(*tools.RenderHTMLTool); ok {
			rt.SetMediaStore(s)
		}
	})
}

// SetTranscriber injects a voice transcriber for agent-level audio transcription.
func (al *AgentLoop) SetTranscriber(t voice.Transcriber) {
	al.transcriber = t
}

var audioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

// transcribeAudioInMessage resolves audio media refs, transcribes them, and
// replaces audio annotations in msg.Content with the transcribed text.
// Returns the (possibly modified) message and true if audio was transcribed.
func (al *AgentLoop) transcribeAudioInMessage(ctx context.Context, msg bus.InboundMessage) (bus.InboundMessage, bool) {
	if al.transcriber == nil || al.mediaStore == nil || len(msg.Media) == 0 {
		return msg, false
	}

	// Transcribe each audio media ref in order.
	var transcriptions []string
	for _, ref := range msg.Media {
		path, meta, err := al.mediaStore.ResolveWithMeta(ref)
		if err != nil {
			logger.WarnCF("voice", "Failed to resolve media ref", map[string]any{"ref": ref, "error": err})
			continue
		}
		if !utils.IsAudioFile(meta.Filename, meta.ContentType) {
			continue
		}
		result, err := al.transcriber.Transcribe(ctx, path)
		if err != nil {
			logger.WarnCF("voice", "Transcription failed", map[string]any{"ref": ref, "error": err})
			transcriptions = append(transcriptions, "")
			continue
		}
		transcriptions = append(transcriptions, result.Text)
	}

	if len(transcriptions) == 0 {
		return msg, false
	}

	al.sendTranscriptionFeedback(ctx, msg.Channel, msg.ChatID, msg.MessageID, transcriptions)

	// Replace audio annotations sequentially with transcriptions.
	idx := 0
	newContent := audioAnnotationRe.ReplaceAllStringFunc(msg.Content, func(match string) string {
		if idx >= len(transcriptions) {
			return match
		}
		text := transcriptions[idx]
		idx++
		return "[voice: " + text + "]"
	})

	// Append any remaining transcriptions not matched by an annotation.
	for ; idx < len(transcriptions); idx++ {
		newContent += "\n[voice: " + transcriptions[idx] + "]"
	}

	msg.Content = newContent
	return msg, true
}

// sendTranscriptionFeedback sends feedback to the user with the result of
// audio transcription if the option is enabled. It uses Manager.SendMessage
// which executes synchronously (rate limiting, splitting, retry) so that
// ordering with the subsequent placeholder is guaranteed.
func (al *AgentLoop) sendTranscriptionFeedback(
	ctx context.Context,
	channel, chatID, messageID string,
	validTexts []string,
) {
	if !al.cfg.Voice.EchoTranscription {
		return
	}
	if al.channelManager == nil {
		return
	}

	var nonEmpty []string
	for _, t := range validTexts {
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}

	var feedbackMsg string
	if len(nonEmpty) > 0 {
		feedbackMsg = "Transcript: " + strings.Join(nonEmpty, "\n")
	} else {
		feedbackMsg = "No voice detected in the audio"
	}

	err := al.channelManager.SendMessage(ctx, bus.OutboundMessage{
		Channel:          channel,
		ChatID:           chatID,
		Content:          feedbackMsg,
		ReplyToMessageID: messageID,
	})
	if err != nil {
		logger.WarnCF("voice", "Failed to send transcription feedback", map[string]any{"error": err.Error()})
	}
}

// inferMediaType determines the media type ("image", "audio", "video", "file")
// from a filename and MIME content type.
func inferMediaType(filename, contentType string) string {
	ct := strings.ToLower(contentType)
	fn := strings.ToLower(filename)

	if strings.HasPrefix(ct, "image/") {
		return "image"
	}
	if strings.HasPrefix(ct, "audio/") || ct == "application/ogg" {
		return "audio"
	}
	if strings.HasPrefix(ct, "video/") {
		return "video"
	}

	// Fallback: infer from extension
	ext := filepath.Ext(fn)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma", ".opus":
		return "audio"
	case ".mp4", ".avi", ".mov", ".webm", ".mkv":
		return "video"
	}

	return "file"
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey string,
) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "user",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(
	ctx context.Context,
	content, channel, chatID string,
) (string, error) {
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for heartbeat")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	var hadAudio bool
	msg, hadAudio = al.transcribeAudioInMessage(ctx, msg)

	// For audio messages the placeholder was deferred by the channel.
	// Now that transcription (and optional feedback) is done, send it.
	if hadAudio && al.channelManager != nil {
		al.channelManager.SendPlaceholder(ctx, msg.Channel, msg.ChatID)
	}

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	route, agent, routeErr := al.resolveMessageRoute(msg)
	if routeErr != nil {
		return "", routeErr
	}

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if resetter, ok := tool.(interface{ ResetSentInRound() }); ok {
			resetter.ResetSentInRound()
		}
	}

	// Resolve session key from route, while preserving explicit agent-scoped keys.
	scopeKey := resolveScopeKey(route, msg.SessionKey)
	sessionKey := scopeKey

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":      agent.ID,
			"scope_key":     scopeKey,
			"session_key":   sessionKey,
			"matched_by":    route.MatchedBy,
			"route_agent":   route.AgentID,
			"route_channel": route.Channel,
		})

	opts := processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		Media:           msg.Media,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	}

	// context-dependent commands check their own Runtime fields and report
	// "unavailable" when the required capability is nil.
	if response, handled := al.handleCommand(ctx, msg, agent, &opts); handled {
		return response, nil
	}

	return al.runAgentLoop(ctx, agent, opts)
}

func (al *AgentLoop) resolveMessageRoute(msg bus.InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	registry := al.GetRegistry()
	route := registry.ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  inboundMetadata(msg, metadataKeyAccountID),
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    inboundMetadata(msg, metadataKeyGuildID),
		TeamID:     inboundMetadata(msg, metadataKeyTeamID),
	})

	agent, ok := registry.GetAgent(route.AgentID)
	if !ok {
		agent = registry.GetDefaultAgent()
	}
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	return route, agent, nil
}

func resolveScopeKey(route routing.ResolvedRoute, msgSessionKey string) string {
	if msgSessionKey != "" && strings.HasPrefix(msgSessionKey, sessionKeyAgentPrefix) {
		return msgSessionKey
	}
	return route.SessionKey
}

func (al *AgentLoop) processSystemMessage(
	ctx context.Context,
	msg bus.InboundMessage,
) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf(
			"processSystemMessage called with non-system message channel: %s",
			msg.Channel,
		)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop is the core message processing logic.
func (al *AgentLoop) runAgentLoop(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
) (string, error) {
	// 0. Record last channel for heartbeat notifications (skip internal channels and cli)
	if opts.Channel != "" && opts.ChatID != "" {
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF(
					"agent",
					"Failed to record last channel",
					map[string]any{"error": err.Error()},
				)
			}
		}
	}

	// 1. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)

	// Resolve media:// refs to base64 data URLs (streaming)
	cfg := al.GetConfig()
	maxMediaSize := cfg.Agents.Defaults.GetMaxMediaSize()
	messages = resolveMediaRefs(messages, al.mediaStore, maxMediaSize)

	// 2. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 2.5 Select the effective model tier for this turn. This
	// runs ONCE per turn so tool-follow-up iterations do not
	// switch models mid-way through. The result is passed into
	// both beginACSTurn (so the manifest records the real model)
	// and runLLMIteration (so the LLM call path uses the same
	// tier it already does today).
	activeCandidates, activeModel := al.selectCandidates(agent, opts.UserMessage, messages)

	// 2.6 ACS: begin a turn manifest. The returned traceID is
	// threaded into runLLMIteration so each LLM call can be
	// recorded against the right row. When ACS is disabled the
	// helper is not called at all (the ACS-off path has zero
	// extra cost beyond the nil check) and acsTraceID stays "".
	var acsTraceID string
	if al.acs != nil {
		acsTraceID = al.beginACSTurn(ctx, agent, opts, messages, activeModel)
	}

	// 3. Run LLM iteration loop
	finalContent, iteration, mediaSent, err := al.runLLMIteration(ctx, agent, messages, opts, acsTraceID, activeCandidates, activeModel)
	if err != nil {
		return "", err
	}

	// 4. Handle empty response — skip default if media was already sent to user
	if finalContent == "" && !mediaSent {
		finalContent = opts.DefaultResponse
	}

	// 5. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 6. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 7. Optional: send response via bus (skip empty text when media was already sent)
	if opts.SendResponse && !(finalContent == "" && mediaSent) {
		al.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// 8. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]any{
			"agent_id":     agent.ID,
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

func (al *AgentLoop) targetReasoningChannelID(channelName string) (chatID string) {
	if al.channelManager == nil {
		return ""
	}
	if ch, ok := al.channelManager.GetChannel(channelName); ok {
		return ch.ReasoningChannelID()
	}
	return ""
}

func (al *AgentLoop) handleReasoning(
	ctx context.Context,
	reasoningContent, channelName, channelID string,
) {
	if reasoningContent == "" || channelName == "" || channelID == "" {
		return
	}

	// Check context cancellation before attempting to publish,
	// since PublishOutbound's select may race between send and ctx.Done().
	if ctx.Err() != nil {
		return
	}

	// Use a short timeout so the goroutine does not block indefinitely when
	// the outbound bus is full.  Reasoning output is best-effort; dropping it
	// is acceptable to avoid goroutine accumulation.
	pubCtx, pubCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pubCancel()

	if err := al.bus.PublishOutbound(pubCtx, bus.OutboundMessage{
		Channel: channelName,
		ChatID:  channelID,
		Content: reasoningContent,
	}); err != nil {
		// Treat context.DeadlineExceeded / context.Canceled as expected
		// (bus full under load, or parent canceled).  Check the error
		// itself rather than ctx.Err(), because pubCtx may time out
		// (5 s) while the parent ctx is still active.
		// Also treat ErrBusClosed as expected — it occurs during normal
		// shutdown when the bus is closed before all goroutines finish.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			errors.Is(err, bus.ErrBusClosed) {
			logger.DebugCF("agent", "Reasoning publish skipped (timeout/cancel)", map[string]any{
				"channel": channelName,
				"error":   err.Error(),
			})
		} else {
			logger.WarnCF("agent", "Failed to publish reasoning (best-effort)", map[string]any{
				"channel": channelName,
				"error":   err.Error(),
			})
		}
	}
}

// runLLMIteration executes the LLM call loop with tool handling.
// acsTraceID is the R11 execmanifest trace_id for this turn; when
// empty, every ACS hook is a no-op. activeCandidates/activeModel
// are pre-computed by runAgentLoop via selectCandidates so the
// R11 manifest row can record the same model the turn uses.
func (al *AgentLoop) runLLMIteration(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
	acsTraceID string,
	activeCandidates []providers.FallbackCandidate,
	activeModel string,
) (string, int, bool, error) {
	iteration := 0
	var finalContent string
	mediaSent := false
	// acsCallSeq is the monotonic per-turn LLM-call counter.
	// Every actual provider.Chat invocation — including retries
	// in the outer loop and fallback-chain attempts inside
	// callLLM — increments it via acsChat. The counter is
	// closed over by callLLM so retries share one sequence.
	acsCallSeq := 0
	_ = acsCallSeq // silences "declared but not used" when acs is nil

	// Drain any pending sub-agent announcements before starting the LLM loop.
	// Announcements are injected as user messages so the LLM sees sub-agent
	// results. This is done once before the loop, not inside it, to avoid
	// breaking the tool-call/tool-result message sequence mid-iteration.
	if agent.SwarmManager != nil {
		if announcements := agent.SwarmManager.DrainAnnouncements(); len(announcements) > 0 {
			for _, ann := range announcements {
				messages = append(messages, providers.Message{
					Role:    "user",
					Content: ann,
				})
			}
			logger.InfoCF("agent", "Injected sub-agent announcements",
				map[string]any{
					"agent_id": agent.ID,
					"count":    len(announcements),
				})
		}
	}

	for iteration < agent.MaxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		// Build tool definitions
		providerToolDefs := agent.Tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]any{
				"agent_id":          agent.ID,
				"iteration":         iteration,
				"model":             activeModel,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        agent.MaxTokens,
				"temperature":       agent.Temperature,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]any{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		// Call LLM with fallback chain if multiple candidates are configured.
		var response *providers.LLMResponse
		var err error

		llmOpts := map[string]any{
			"max_tokens":       agent.MaxTokens,
			"prompt_cache_key": agent.ID,
		}
		if agent.Temperature != nil {
			llmOpts["temperature"] = *agent.Temperature
		}
		if agent.ForceStream {
			llmOpts["force_stream"] = true
		}
		// parseThinkingLevel guarantees ThinkingOff for empty/unknown values,
		// so checking != ThinkingOff is sufficient.
		if agent.ThinkingLevel != ThinkingOff {
			if tc, ok := agent.Provider.(providers.ThinkingCapable); ok && tc.SupportsThinking() {
				llmOpts["thinking_level"] = string(agent.ThinkingLevel)
			} else {
				logger.WarnCF("agent", "thinking_level is set but current provider does not support it, ignoring",
					map[string]any{"agent_id": agent.ID, "thinking_level": string(agent.ThinkingLevel)})
			}
		}

		// ACS LLM-call instrumentation wrapper. Fires ONCE per
		// actual provider.Chat invocation, including retries and
		// fallback-chain attempts, so the execmanifest trace
		// faithfully records every LLM call with the REAL model
		// that was tried (not the configured primary). The per-
		// turn callSeq is passed by pointer so retries inside
		// the outer loop keep incrementing the same counter.
		acsChat := func(ctx context.Context, modelUsed string, err error) {
			if al.acs == nil || acsTraceID == "" {
				return
			}
			seq := acsCallSeq
			acsCallSeq++
			status := "ok"
			if err != nil {
				status = "error"
			}
			rcErr := al.acs.RecordLLMCall(ctx, execmanifest.ProviderCall{
				TraceID:   acsTraceID,
				CallSeq:   seq,
				RequestID: fmt.Sprintf("%s-call-%d-%s", acsTraceID, seq, status),
				ModelID:   modelUsed,
			})
			if rcErr != nil {
				logger.WarnCF("agent", "ACS RecordLLMCall failed (continuing)",
					map[string]any{
						"error":    rcErr.Error(),
						"trace_id": acsTraceID,
						"call_seq": seq,
					})
			}
		}

		callLLM := func() (*providers.LLMResponse, error) {
			al.activeRequests.Add(1)
			defer al.activeRequests.Done()

			if len(activeCandidates) > 1 && al.fallback != nil {
				fbResult, fbErr := al.fallback.Execute(
					ctx,
					activeCandidates,
					func(ctx context.Context, provider, model string) (*providers.LLMResponse, error) {
						resp, runErr := agent.Provider.Chat(ctx, messages, providerToolDefs, model, llmOpts)
						// Record this specific attempt, success or failure,
						// with the ACTUAL model the fallback chose.
						acsChat(ctx, model, runErr)
						return resp, runErr
					},
				)
				if fbErr != nil {
					return nil, fbErr
				}
				if fbResult.Provider != "" && len(fbResult.Attempts) > 0 {
					logger.InfoCF(
						"agent",
						fmt.Sprintf("Fallback: succeeded with %s/%s after %d attempts",
							fbResult.Provider, fbResult.Model, len(fbResult.Attempts)+1),
						map[string]any{"agent_id": agent.ID, "iteration": iteration},
					)
				}
				return fbResult.Response, nil
			}
			resp, err := agent.Provider.Chat(ctx, messages, providerToolDefs, activeModel, llmOpts)
			acsChat(ctx, activeModel, err)
			return resp, err
		}

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = callLLM()
			if err == nil {
				break
			}

			errMsg := strings.ToLower(err.Error())

			// Check if this is a network/HTTP timeout — not a context window error.
			isTimeoutError := errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(errMsg, "deadline exceeded") ||
				strings.Contains(errMsg, "client.timeout") ||
				strings.Contains(errMsg, "timed out") ||
				strings.Contains(errMsg, "timeout exceeded")

			// Detect real context window / token limit errors, excluding network timeouts.
			isContextError := !isTimeoutError && (strings.Contains(errMsg, "context_length_exceeded") ||
				strings.Contains(errMsg, "context window") ||
				strings.Contains(errMsg, "maximum context length") ||
				strings.Contains(errMsg, "token limit") ||
				strings.Contains(errMsg, "too many tokens") ||
				strings.Contains(errMsg, "max_tokens") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "prompt is too long") ||
				strings.Contains(errMsg, "request too large"))

			if isTimeoutError && retry < maxRetries {
				backoff := time.Duration(retry+1) * 5 * time.Second
				logger.WarnCF("agent", "Timeout error, retrying after backoff", map[string]any{
					"error":   err.Error(),
					"retry":   retry,
					"backoff": backoff.String(),
				})
				time.Sleep(backoff)
				continue
			}

			if isContextError && retry < maxRetries {
				logger.WarnCF(
					"agent",
					"Context window error detected, attempting compression",
					map[string]any{
						"error": err.Error(),
						"retry": retry,
					},
				)

				if retry == 0 && !constants.IsInternalChannel(opts.Channel) {
					al.bus.PublishOutbound(ctx, bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: "Context window exceeded. Compressing history and retrying...",
					})
				}

				al.forceCompression(agent, opts.SessionKey)
				newHistory := agent.Sessions.GetHistory(opts.SessionKey)
				newSummary := agent.Sessions.GetSummary(opts.SessionKey)
				messages = agent.ContextBuilder.BuildMessages(
					newHistory, newSummary, "",
					nil, opts.Channel, opts.ChatID,
				)
				continue
			}
			break
		}

		if err != nil {
			logger.ErrorCF("agent", "LLM call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"model":     activeModel,
					"error":     err.Error(),
				})
			return "", iteration, mediaSent, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		// ACS LLM-call recording is handled INSIDE callLLM /
		// acsChat so retries and fallback-chain attempts each
		// get their own row with the real model. No recording
		// needed here at the iteration level.

		go al.handleReasoning(
			ctx,
			response.Reasoning,
			opts.Channel,
			al.targetReasoningChannelID(opts.Channel),
		)

		logger.DebugCF("agent", "LLM response",
			map[string]any{
				"agent_id":       agent.ID,
				"iteration":      iteration,
				"content_chars":  len(response.Content),
				"tool_calls":     len(response.ToolCalls),
				"reasoning":      response.Reasoning,
				"target_channel": al.targetReasoningChannelID(opts.Channel),
				"channel":        opts.Channel,
			})
		// Check if no tool calls - then check reasoning content if any
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			if finalContent == "" && response.ReasoningContent != "" {
				finalContent = response.ReasoningContent
			}
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]any{
					"agent_id":      agent.ID,
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Log tool calls
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]any{
				"agent_id":  agent.ID,
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:             "assistant",
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// Copy ExtraContent to ensure thought_signature is persisted for Gemini 3
			extraContent := tc.ExtraContent
			thoughtSignature := ""
			if tc.Function != nil {
				thoughtSignature = tc.Function.ThoughtSignature
			}

			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:             tc.Name,
					Arguments:        string(argumentsJSON),
					ThoughtSignature: thoughtSignature,
				},
				ExtraContent:     extraContent,
				ThoughtSignature: thoughtSignature,
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls in parallel
		type indexedAgentResult struct {
			result *tools.ToolResult
			tc     providers.ToolCall
		}

		agentResults := make([]indexedAgentResult, len(normalizedToolCalls))
		var wg sync.WaitGroup

		for i, tc := range normalizedToolCalls {
			agentResults[i].tc = tc

			wg.Add(1)
			go func(idx int, tc providers.ToolCall) {
				defer wg.Done()

				argsJSON, _ := json.Marshal(tc.Arguments)
				argsPreview := utils.Truncate(string(argsJSON), 200)
				logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
					map[string]any{
						"agent_id":  agent.ID,
						"tool":      tc.Name,
						"iteration": iteration,
					})

				// Create async callback for tools that implement AsyncExecutor.
				// When the background work completes, this publishes the result
				// as an inbound system message so processSystemMessage routes it
				// back to the user via the normal agent loop.
				asyncCallback := func(_ context.Context, result *tools.ToolResult) {
					// Send ForUser content directly to the user (immediate feedback),
					// mirroring the synchronous tool execution path.
					if !result.Silent && result.ForUser != "" {
						outCtx, outCancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer outCancel()
						_ = al.bus.PublishOutbound(outCtx, bus.OutboundMessage{
							Channel: opts.Channel,
							ChatID:  opts.ChatID,
							Content: result.ForUser,
						})
					}

					// Determine content for the agent loop (ForLLM or error).
					content := result.ForLLM
					if content == "" && result.Err != nil {
						content = result.Err.Error()
					}
					if content == "" {
						return
					}

					logger.InfoCF("agent", "Async tool completed, publishing result",
						map[string]any{
							"tool":        tc.Name,
							"content_len": len(content),
							"channel":     opts.Channel,
						})

					pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer pubCancel()
					_ = al.bus.PublishInbound(pubCtx, bus.InboundMessage{
						Channel:  "system",
						SenderID: fmt.Sprintf("async:%s", tc.Name),
						ChatID:   fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID),
						Content:  content,
					})
				}

				// Run tool dispatch through the R12 ACS ledger
				// wrapper. When ACS is off or the tool does not
				// declare a side-effecting class, Dispatch falls
				// through to the registry's ExecuteWithContext
				// directly and produces zero ledger rows.
				runTool := func(ctx context.Context) *tools.ToolResult {
					return agent.Tools.ExecuteWithContext(
						ctx,
						tc.Name,
						tc.Arguments,
						opts.Channel,
						opts.ChatID,
						asyncCallback,
					)
				}
				var toolResult *tools.ToolResult
				if al.acs != nil && acsTraceID != "" {
					// Look up the tool to classify its effect; if
					// the tool isn't registered under that name,
					// fall through to the direct path and let the
					// registry's own "unknown tool" error surface.
					var toolObj tools.Tool
					if t, ok := agent.Tools.Get(tc.Name); ok {
						toolObj = t
					}
					if toolObj != nil {
						dr := al.acs.Dispatch(ctx, acs.DispatchRequest{
							TraceID:   acsTraceID,
							Tool:      toolObj,
							Args:      tc.Arguments,
							Principal: acsPrincipalLabel(agent.ID, opts.Channel, opts.ChatID),
						}, runTool)
						toolResult = dr.Result
						// Codex R12: surface ledger finalization
						// failures to the operator log. A silent
						// commit/abort miss would hide ledger-
						// reality drift.
						if dr.FinalizeErr != nil {
							logger.WarnCF("agent", "ACS tool-dispatch finalize failed",
								map[string]any{
									"tool":      tc.Name,
									"intent_id": dr.IntentID,
									"trace_id":  acsTraceID,
									"error":     dr.FinalizeErr.Error(),
								})
						}
					} else {
						toolResult = runTool(ctx)
					}
				} else {
					toolResult = runTool(ctx)
				}
				agentResults[idx].result = toolResult
			}(i, tc)
		}
		wg.Wait()

		// Process results in original order (send to user, save to session)
		for _, r := range agentResults {
			// Send ForUser content to user immediately if not Silent
			if !r.result.Silent && r.result.ForUser != "" && opts.SendResponse {
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Content: r.result.ForUser,
				})
				logger.DebugCF("agent", "Sent tool result to user",
					map[string]any{
						"tool":        r.tc.Name,
						"content_len": len(r.result.ForUser),
					})
			}

			// If tool returned media refs, publish them as outbound media
			if len(r.result.Media) > 0 {
				parts := make([]bus.MediaPart, 0, len(r.result.Media))
				for _, ref := range r.result.Media {
					part := bus.MediaPart{Ref: ref}
					if al.mediaStore != nil {
						if _, meta, err := al.mediaStore.ResolveWithMeta(ref); err == nil {
							part.Filename = meta.Filename
							part.ContentType = meta.ContentType
							part.Type = inferMediaType(meta.Filename, meta.ContentType)
						}
					}
					parts = append(parts, part)
				}
				al.bus.PublishOutboundMedia(ctx, bus.OutboundMediaMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Parts:   parts,
				})
				mediaSent = true
			}

			// Determine content for LLM based on tool result
			contentForLLM := r.result.ForLLM
			if contentForLLM == "" && r.result.Err != nil {
				contentForLLM = r.result.Err.Error()
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: r.tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			agent.Sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}

		// Tick down TTL of discovered tools after processing tool results.
		// Only reached when tool calls were made (the loop continues);
		// the break on no-tool-call responses skips this.
		// NOTE: This is safe because processMessage is sequential per agent.
		// If per-agent concurrency is added, TTL consistency between
		// ToProviderDefs and Get must be re-evaluated.
		agent.Tools.TickTTL()
		logger.DebugCF("agent", "TTL tick after tool execution", map[string]any{
			"agent_id": agent.ID, "iteration": iteration,
		})
	}

	return finalContent, iteration, mediaSent, nil
}

// selectCandidates returns the model candidates and resolved model name to use
// for a conversation turn. When model routing is configured and the incoming
// message scores below the complexity threshold, it returns the light model
// candidates instead of the primary ones.
//
// The returned (candidates, model) pair is used for all LLM calls within one
// turn — tool follow-up iterations use the same tier as the initial call so
// that a multi-step tool chain doesn't switch models mid-way.
func (al *AgentLoop) selectCandidates(
	agent *AgentInstance,
	userMsg string,
	history []providers.Message,
) (candidates []providers.FallbackCandidate, model string) {
	if agent.Router == nil || len(agent.LightCandidates) == 0 {
		return agent.Candidates, agent.Model
	}

	_, usedLight, score := agent.Router.SelectModel(userMsg, history, agent.Model)
	if !usedLight {
		logger.DebugCF("agent", "Model routing: primary model selected",
			map[string]any{
				"agent_id":  agent.ID,
				"score":     score,
				"threshold": agent.Router.Threshold(),
			})
		return agent.Candidates, agent.Model
	}

	logger.InfoCF("agent", "Model routing: light model selected",
		map[string]any{
			"agent_id":    agent.ID,
			"light_model": agent.Router.LightModel(),
			"score":       score,
			"threshold":   agent.Router.Threshold(),
		})
	return agent.LightCandidates, agent.Router.LightModel()
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(agent *AgentInstance, sessionKey, channel, chatID string) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * agent.SummarizeTokenPercent / 100

	if len(newHistory) > agent.SummarizeMessageThreshold || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := al.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				al.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(agent *AgentInstance, sessionKey string) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	conversation := history[1 : len(history)-1]
	if len(conversation) == 0 {
		return
	}

	// Find the mid-point of the conversation, then adjust it so we
	// don't split a tool-call/tool-result pair. A split would leave
	// the kept half with a dangling tool_call_id reference (tool
	// result without its matching assistant tool-call message),
	// which breaks the OpenAI/Anthropic message format.
	//
	// Two cases to guard against:
	// (a) mid lands ON a tool-result → the tool-call before it is dropped
	// (b) mid lands right AFTER a tool-call → the tool-results after it are kept orphaned
	mid := len(conversation) / 2

	// Case (a): walk forward past any tool-result messages so we
	// don't keep a result without its call.
	for mid < len(conversation) && conversation[mid].Role == "tool" {
		mid++
	}
	// Case (b): if the last dropped message (mid-1) is an assistant
	// with tool calls, include it in the kept set by stepping back.
	if mid > 0 && mid < len(conversation) &&
		conversation[mid-1].Role == "assistant" && len(conversation[mid-1].ToolCalls) > 0 {
		mid--
	}
	// Safety: don't drop everything — keep at least 2 messages
	if mid >= len(conversation)-1 {
		// Find the first safe split point (not inside a tool pair)
		mid = len(conversation) / 2
		// Walk backward to find a clean boundary
		for mid > 1 && (conversation[mid].Role == "tool" ||
			(conversation[mid-1].Role == "assistant" && len(conversation[mid-1].ToolCalls) > 0)) {
			mid--
		}
	}

	// New history structure:
	// 1. System Prompt (with compression note appended)
	// 2. Second half of conversation
	// 3. Last message

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0, 1+len(keptConversation)+1)

	// Append compression note to the original system prompt instead of adding a new system message
	// This avoids having two consecutive system messages which some APIs (like Zhipu) reject
	compressionNote := fmt.Sprintf(
		"\n\n[System Note: Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	enhancedSystemPrompt := history[0]
	enhancedSystemPrompt.Content = enhancedSystemPrompt.Content + compressionNote
	newHistory = append(newHistory, enhancedSystemPrompt)

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, history[len(history)-1]) // Last message

	// Update session
	agent.Sessions.SetHistory(sessionKey, newHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	registry := al.GetRegistry()
	agent := registry.GetDefaultAgent()
	if agent == nil {
		return info
	}

	// Tools info
	toolsList := agent.Tools.List()
	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
	}

	// Skills info
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// Agents info
	info["agents"] = map[string]any{
		"count": len(registry.ListAgentIDs()),
		"ids":   registry.ListAgentIDs(),
	}

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, msg := range messages {
		fmt.Fprintf(&sb, "  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			sb.WriteString("  ToolCalls:\n")
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					fmt.Fprintf(
						&sb,
						"      Arguments: %s\n",
						utils.Truncate(tc.Function.Arguments, 200),
					)
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			fmt.Fprintf(&sb, "  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&sb, "  ToolCallID: %s\n", msg.ToolCallID)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(toolDefs []providers.ToolDefinition) string {
	if len(toolDefs) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, tool := range toolDefs {
		fmt.Fprintf(&sb, "  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		fmt.Fprintf(&sb, "      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			fmt.Fprintf(
				&sb,
				"      Parameters: %s\n",
				utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200),
			)
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	const (
		maxSummarizationMessages = 10
		llmMaxRetries            = 3
		llmTemperature           = 0.3
		fallbackMaxContentLength = 200
	)

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > maxSummarizationMessages {
		mid := len(validMessages) / 2

		mid = al.findNearestUserMessage(validMessages, mid)

		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, agent, part1, "")
		s2, _ := al.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)

		resp, err := al.retryLLMCall(ctx, agent, mergePrompt, llmMaxRetries)
		if err == nil && resp.Content != "" {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)
		agent.Sessions.TruncateHistory(sessionKey, 4)
		agent.Sessions.Save(sessionKey)
	}
}

// findNearestUserMessage finds the nearest user message to the given index.
// It searches backward first, then forward if no user message is found.
func (al *AgentLoop) findNearestUserMessage(messages []providers.Message, mid int) int {
	originalMid := mid

	for mid > 0 && messages[mid].Role != "user" {
		mid--
	}

	if messages[mid].Role == "user" {
		return mid
	}

	mid = originalMid
	for mid < len(messages) && messages[mid].Role != "user" {
		mid++
	}

	if mid < len(messages) {
		return mid
	}

	return originalMid
}

// retryLLMCall calls the LLM with retry logic.
func (al *AgentLoop) retryLLMCall(
	ctx context.Context,
	agent *AgentInstance,
	prompt string,
	maxRetries int,
) (*providers.LLMResponse, error) {
	const (
		llmTemperature = 0.3
	)

	var resp *providers.LLMResponse
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		al.activeRequests.Add(1)
		resp, err = func() (*providers.LLMResponse, error) {
			defer al.activeRequests.Done()
			return agent.Provider.Chat(
				ctx,
				[]providers.Message{{Role: "user", Content: prompt}},
				nil,
				agent.Model,
				map[string]any{
					"max_tokens":       agent.MaxTokens,
					"temperature":      llmTemperature,
					"prompt_cache_key": agent.ID,
				},
			)
		}()

		if err == nil && resp != nil && resp.Content != "" {
			return resp, nil
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	return resp, err
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	const (
		llmMaxRetries             = 3
		llmTemperature            = 0.3
		fallbackMinContentLength  = 200
		fallbackMaxContentPercent = 10
	)

	var sb strings.Builder
	sb.WriteString(
		"Provide a concise summary of this conversation segment, preserving core context and key points.\n",
	)
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := al.retryLLMCall(ctx, agent, prompt, llmMaxRetries)
	if err == nil && response.Content != "" {
		return strings.TrimSpace(response.Content), nil
	}

	var fallback strings.Builder
	fallback.WriteString("Conversation summary: ")
	for i, m := range batch {
		if i > 0 {
			fallback.WriteString(" | ")
		}
		content := strings.TrimSpace(m.Content)
		runes := []rune(content)
		if len(runes) == 0 {
			fallback.WriteString(fmt.Sprintf("%s: ", m.Role))
			continue
		}

		keepLength := len(runes) * fallbackMaxContentPercent / 100
		if keepLength < fallbackMinContentLength {
			keepLength = fallbackMinContentLength
		}

		if keepLength > len(runes) {
			keepLength = len(runes)
		}

		content = string(runes[:keepLength])
		if keepLength < len(runes) {
			content += "..."
		}
		fallback.WriteString(fmt.Sprintf("%s: %s", m.Role, content))
	}
	return fallback.String(), nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	agent *AgentInstance,
	opts *processOptions,
) (string, bool) {
	if !commands.HasCommandPrefix(msg.Content) {
		return "", false
	}

	if al.cmdRegistry == nil {
		return "", false
	}

	rt := al.buildCommandsRuntime(agent, opts)
	executor := commands.NewExecutor(al.cmdRegistry, rt)

	var commandReply string
	result := executor.Execute(ctx, commands.Request{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		SenderID: msg.SenderID,
		Text:     msg.Content,
		Reply: func(text string) error {
			commandReply = text
			return nil
		},
	})

	switch result.Outcome {
	case commands.OutcomeHandled:
		if result.Err != nil {
			return mapCommandError(result), true
		}
		if commandReply != "" {
			return commandReply, true
		}
		return "", true
	default: // OutcomePassthrough — let the message fall through to LLM
		return "", false
	}
}

func (al *AgentLoop) buildCommandsRuntime(agent *AgentInstance, opts *processOptions) *commands.Runtime {
	registry := al.GetRegistry()
	cfg := al.GetConfig()
	rt := &commands.Runtime{
		Config:          cfg,
		ListAgentIDs:    registry.ListAgentIDs,
		ListDefinitions: al.cmdRegistry.Definitions,
		GetEnabledChannels: func() []string {
			if al.channelManager == nil {
				return nil
			}
			return al.channelManager.GetEnabledChannels()
		},
		SwitchChannel: func(value string) error {
			if al.channelManager == nil {
				return fmt.Errorf("channel manager not initialized")
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Errorf("channel '%s' not found or not enabled", value)
			}
			return nil
		},
	}
	if agent != nil {
		rt.GetModelInfo = func() (string, string) {
			return agent.Model, cfg.Agents.Defaults.Provider
		}
		rt.SwitchModel = func(value string) (string, error) {
			oldModel := agent.Model
			agent.Model = value
			return oldModel, nil
		}

		rt.ClearHistory = func() error {
			if opts == nil {
				return fmt.Errorf("process options not available")
			}
			if agent.Sessions == nil {
				return fmt.Errorf("sessions not initialized for agent")
			}

			agent.Sessions.SetHistory(opts.SessionKey, make([]providers.Message, 0))
			agent.Sessions.SetSummary(opts.SessionKey, "")
			agent.Sessions.Save(opts.SessionKey)
			return nil
		}
	}
	return rt
}

func mapCommandError(result commands.ExecuteResult) string {
	if result.Command == "" {
		return fmt.Sprintf("Failed to execute command: %v", result.Err)
	}
	return fmt.Sprintf("Failed to execute /%s: %v", result.Command, result.Err)
}

// extractPeer extracts the routing peer from the inbound message's structured Peer field.
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	if msg.Peer.Kind == "" {
		return nil
	}
	peerID := msg.Peer.ID
	if peerID == "" {
		if msg.Peer.Kind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: msg.Peer.Kind, ID: peerID}
}

func inboundMetadata(msg bus.InboundMessage, key string) string {
	if msg.Metadata == nil {
		return ""
	}
	return msg.Metadata[key]
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := inboundMetadata(msg, metadataKeyParentPeerKind)
	parentID := inboundMetadata(msg, metadataKeyParentPeerID)
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}

// Helper to extract provider from registry for cleanup
func extractProvider(registry *AgentRegistry) (providers.LLMProvider, bool) {
	if registry == nil {
		return nil, false
	}
	// Get any agent to access the provider
	defaultAgent := registry.GetDefaultAgent()
	if defaultAgent == nil {
		return nil, false
	}
	return defaultAgent.Provider, true
}

// beginACSTurn records a new execmanifest row at the top of a
// turn. When the ACS bundle is disabled (al.acs == nil) this is a
// cheap no-op that returns an empty traceID; every downstream ACS
// hook then also skips because an empty traceID signals "ACS off".
//
// The manifest captures the prompt hash, a stand-in tool schema
// hash, a monotonic per-session turn number (initialized from
// execmanifest.MaxTurn on first use per session), and the model
// the turn will use. Full tool schema hashing from the registry
// is a follow-up — the current slice uses the message content
// hash as a stand-in so the row is at least attributable.
func (al *AgentLoop) beginACSTurn(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
	messages []providers.Message,
	activeModel string,
) string {
	if al.acs == nil {
		return ""
	}
	// Allocate a monotonic per-session turn number. This is
	// strictly increasing for the life of the agent process AND
	// lazily seeded from MaxTurn() so it survives process
	// restarts — a crash and restart resumes counting from
	// MAX(turn)+1 of what was on disk, not from 0. Codex R11
	// caught that the previous len(history) proxy resets after
	// history summarization and produces duplicate
	// (session_id, turn) pairs on the UNIQUE constraint.
	turnNum := al.allocateACSTurnNumber(ctx, opts.SessionKey)

	promptHash := hashMessagesForACS(messages)
	// Record the active model that the turn will actually use
	// (after fallback-chain routing), not agent.Model. This
	// matches what the provider call rows will say.
	modelID := activeModel
	if modelID == "" {
		modelID = agent.Model
	}
	manifest := execmanifest.Manifest{
		SessionID:      opts.SessionKey,
		Turn:           turnNum,
		PromptHash:     promptHash,
		ToolSchemaHash: promptHash, // stand-in until a real schema hash is threaded in
		SkillHashes:    nil,        // canonicalized to []
		McpVersions:    nil,        // canonicalized to {}
		ModelID:        modelID,
		PromptEpoch:    0, // stand-in until the R4 §14 prompt epoch API is wired
	}
	traceID, err := al.acs.BeginTurn(ctx, manifest)
	if err != nil {
		logger.WarnCF("agent", "ACS BeginTurn failed (continuing)",
			map[string]any{
				"error":       err.Error(),
				"session_key": opts.SessionKey,
				"turn":        turnNum,
			})
		return ""
	}
	return traceID
}

// acsPrincipalLabel produces a stand-in principal label string
// for a tool dispatch until pkg/principal is threaded into the
// runtime tool registry. Matches pkg/principal.Label() format
// so downstream audits can parse the column with one regex.
// Codex R12 caught that the previous version hardcoded
// `agent=main`, which would mis-attribute every side-effecting
// tool in a multi-agent deployment — fixed by taking agent.ID
// as a parameter. Once the full PrincipalContext plumbing
// lands, this helper should go away and the typed principal
// should flow through instead.
func acsPrincipalLabel(agentID, channel, chatID string) string {
	if agentID == "" {
		agentID = "unknown"
	}
	if channel == "" {
		channel = "unknown"
	}
	if chatID == "" {
		chatID = "unknown"
	}
	return "agent=" + agentID + ";user=unknown;account=unknown;channel=" + channel + ":" + chatID
}

// allocateACSTurnNumber returns a monotonic per-session turn
// number. On first call for a session, it queries
// execmanifest.MaxTurn to seed the counter from what's already on
// disk; subsequent calls return atomic increments. The sync.Map
// lookup + CAS pattern ensures concurrent turns on the same
// session each get a distinct number without a global mutex.
func (al *AgentLoop) allocateACSTurnNumber(ctx context.Context, sessionKey string) int {
	if al.acs == nil {
		return 0
	}
	counterAny, ok := al.acsTurnCounters.Load(sessionKey)
	if !ok {
		// Seed from disk. MaxTurn returns 0 on a fresh session
		// so the first allocated number becomes 1.
		maxTurn, err := al.acs.MaxTurn(ctx, sessionKey)
		if err != nil {
			// Unable to seed — log and fall back to starting
			// from 0. The worst case is a collision on
			// resume, which returns ErrTraceAlreadyRecorded and
			// disables ACS for that turn. Better than failing
			// the turn outright.
			logger.WarnCF("agent", "ACS MaxTurn lookup failed, seeding counter from 0",
				map[string]any{"error": err.Error(), "session_key": sessionKey})
			maxTurn = 0
		}
		fresh := &atomic.Int64{}
		fresh.Store(int64(maxTurn))
		actual, loaded := al.acsTurnCounters.LoadOrStore(sessionKey, fresh)
		if loaded {
			counterAny = actual
		} else {
			counterAny = fresh
		}
	}
	counter := counterAny.(*atomic.Int64)
	return int(counter.Add(1))
}

// ACS LLM-call recording has moved into an inline closure inside
// runLLMIteration (`acsChat`) so every provider.Chat invocation —
// including retries in the outer loop and fallback-chain attempts
// via providers.FallbackChain — gets its own execmanifest
// provider-call row with the ACTUAL model that was tried. See
// the acsChat closure at the top of the `for iteration < ...`
// loop in runLLMIteration. The top-level helper that used to
// live here was removed in the R11 fix round.

// hashMessagesForACS produces a deterministic stand-in for the
// R4 §14 prompt hash. It concatenates the message roles and first
// 256 bytes of each content field and runs FNV-1a over the result.
// The goal is attribution, not cryptographic integrity — the
// dedicated prompt-epoch API from R4 §14 will replace this.
func hashMessagesForACS(messages []providers.Message) string {
	// Lightweight hash — avoids pulling crypto/sha256 into the
	// hot path for what is an observability field.
	var h uint64 = 1469598103934665603 // FNV-1a offset basis
	for _, m := range messages {
		for _, c := range m.Role {
			h ^= uint64(c)
			h *= 1099511628211
		}
		content := m.Content
		if len(content) > 256 {
			content = content[:256]
		}
		for _, c := range content {
			h ^= uint64(c)
			h *= 1099511628211
		}
	}
	return fmt.Sprintf("fnv1a-%016x", h)
}

