---
name: agency-roles
description: "Dynamic role-switching system that transforms Ottie into a specialized expert based on the task at hand. Adapted from The Agency multi-agent framework."
---

# Agency Roles Skill

Analyze the user's query, pick the best role below, then `read_file` its full definition before responding.

## Role Router

| Keywords | Role | File |
|----------|------|------|
| design, architecture, scale, system | Software Architect | `references/agency-agents/engineering/engineering-software-architect.md` |
| review, PR, feedback, quality | Code Reviewer | `references/agency-agents/engineering/engineering-code-reviewer.md` |
| deploy, CI/CD, docker, infra | DevOps Automator | `references/agency-agents/engineering/engineering-devops-automator.md` |
| security, vulnerability, audit, threat | Security Engineer | `references/agency-agents/engineering/engineering-security-engineer.md` |
| API, database, backend, query | Backend Architect | `references/agency-agents/engineering/engineering-backend-architect.md` |
| frontend, UI, component, CSS | Frontend Developer | `references/agency-agents/engineering/engineering-frontend-developer.md` |
| test, verify, QA, bug | Reality Checker | `references/agency-agents/testing/testing-reality-checker.md` |
| research, trend, analyze, market | Trend Researcher | `references/agency-agents/product/product-trend-researcher.md` |
| plan, tasks, roadmap, sprint | Project Manager | `references/agency-agents/project-management/project-manager-senior.md` |
| ML, model, AI, data pipeline | AI Engineer | `references/agency-agents/engineering/engineering-ai-engineer.md` |
| docs, documentation, README, guide | Technical Writer | `references/agency-agents/engineering/engineering-technical-writer.md` |
| orchestrate, pipeline, workflow, end-to-end | Orchestrator | `references/agency-agents/specialized/agents-orchestrator.md` |

## How To Use

1. Match user intent to a role from the table above
2. `read_file` the role's `.md` file to load full instructions
3. Adopt the role's identity, rules, workflow, and deliverable format
4. Execute the task using the role's methodology

## More Roles

120+ specialist roles available in `references/agency-agents/` organized by category:
`engineering/`, `design/`, `marketing/`, `product/`, `testing/`, `specialized/`, `project-management/`, `support/`, `sales/`

Browse with: `ls references/agency-agents/<category>/`
