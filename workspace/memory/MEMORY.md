# Long-term Memory

## User Information

- GitHub: jiayaoqijia (1453879+jiayaoqijia@users.noreply.github.com)
- Repo: https://github.com/jiayaoqijia/ottie
- No AI attribution in commits (CLAUDE.md rule)

## Preferences

- Keep things simple, avoid bloat
- Prefers action over discussion
- Likes research-backed decisions

## AI Employee Agent Research (2026-03-13)

### Key Frameworks

| Framework | Pattern | Insight |
|-----------|---------|---------|
| BabyAGI | Execute → Create tasks → Prioritize → Loop | Never idle |
| MetaGPT | SOP roles + pub-sub | Agents only see relevant info |
| CrewAI | Role + goal + backstory | Explicit roles > generic prompts |
| Devin | Plan → Implement → Test → Review checkpoint | 80% savings, not full automation |
| AI_Employee | Inbox → Plans → Approval → Done | Eisenhower Matrix; weekly briefings |
| Mission Control | Daemon + JSON queue + retry limits | Escalate after 3 failures |

### Devin AI — NOT Open Source

- Proprietary by Cognition Labs ($20+/mo)
- "Open Source Initiative" = free credits for OSS, not open-sourcing Devin
- Leaked prompt: https://github.com/x1xhlol/system-prompts-and-models-of-ai-tools
- Also: https://github.com/EliFuzz/awesome-system-prompts
- OSS alternatives: OpenDevin, Devika

### ClawHub Skills of Interest

- **capability-evolver** (35K downloads) — self-evolution engine; security concern (Feishu exfiltration reported)
- **agent-task-tracker** — automatic lifecycle state tracking
- **AgentDo** — agent-to-agent task marketplace

### Good-Intern Skill Sources

Synthesized from: BabyAGI (task loop), Devin (checkpoints), AI_Employee (Eisenhower triage), Mission Control (escalation), Dorabot (propose-before-act), MetaGPT (structured handoffs)

### Reference Links

- CrewAI: https://github.com/crewAIInc/crewAI
- MetaGPT: https://github.com/FoundationAgents/MetaGPT
- AI_Employee: https://github.com/HumbalAli/AI_Employee
- ClawWork: https://github.com/HKUDS/ClawWork
- Anthropic context engineering: https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- Augment prompting: https://www.augmentcode.com/blog/how-to-build-your-agent-11-prompting-techniques-for-better-ai-agents

## Configuration

- Forked from Ottie (github.com/jiayaoqijia/ottie)
- Self-evolving skills enabled
- Security scanning on skill installs
