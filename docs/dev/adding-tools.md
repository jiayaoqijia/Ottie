# Adding Tools to Ottie

## Quick reference

| What | Where |
|-|-|
| Tool interface | `pkg/tools/base.go` — `Tool` interface |
| Effect class opt-in | `pkg/tools/base.go` — `EffectClassifier` interface |
| Registry | `pkg/tools/registry.go` — `ToolRegistry.Register` |
| Registration in agent loop | `pkg/agent/loop.go` — `registerSharedTools` |
| ACS ledger wrapping | `pkg/acs/dispatch.go` — `Bundle.Dispatch` |

## Steps to add a new tool

1. Create `pkg/tools/my_tool.go` implementing `Tool`:
   ```go
   type MyTool struct{}
   func (t *MyTool) Name() string { return "my_tool" }
   func (t *MyTool) Description() string { return "Does something" }
   func (t *MyTool) Parameters() map[string]any { return map[string]any{...} }
   func (t *MyTool) Execute(ctx context.Context, args map[string]any) *ToolResult { ... }
   ```

2. If the tool has side effects, implement `EffectClassifier`:
   ```go
   func (t *MyTool) EffectClass() EffectClass { return EffectWritesLocal }
   ```
   Available classes: `EffectReadOnly` (default), `EffectWritesLocal`, `EffectWritesState`, `EffectWritesChain`, `EffectWritesWallet`.

3. Register in `pkg/agent/loop.go` inside `registerSharedTools`:
   ```go
   if cfg.Tools.IsToolEnabled("my_tool") {
       agent.Tools.Register(tools.NewMyTool(...))
   }
   ```

4. Add a config key in `pkg/config/config.go` (ToolsConfig struct) and `pkg/config/defaults.go`.

5. Write tests in `pkg/tools/my_tool_test.go`.

6. When ACS is enabled and the tool declares a non-read-only effect class, the agent loop automatically wraps every invocation in a Prepare/Commit/Abort cycle via `acs.Bundle.Dispatch`. No additional code needed from the tool implementer.

## Effect class guidelines

| Class | When to use | Ledger-wrapped? |
|-|-|-|
| `EffectReadOnly` | Queries, searches, reads | No |
| `EffectWritesLocal` | File writes, shell commands | Yes |
| `EffectWritesState` | Skill install, config changes, cron | Yes |
| `EffectWritesChain` | Messages to external channels, API posts | Yes |
| `EffectWritesWallet` | Transaction signing, fund transfers | Yes |

## Async tools

Tools implementing `AsyncExecutor` are not currently wrapped by the action ledger because their real outcome arrives later via callback. The `acs.Bundle.Dispatch` helper skips ledger wrapping for async tools and returns immediately.
