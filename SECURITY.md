# Security Policy

## Supported Versions

Please see [Releases](https://github.com/jiayaoqijia/ottie/releases). We
recommend using the most recently released version.

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |
| Older   | No        |

## Reporting a Vulnerability

**Please do not file a public issue for security vulnerabilities.**

To report a vulnerability, email **security@altresear.ch** with:

- Description of the vulnerability
- Steps to reproduce
- Affected package(s) and version
- Potential impact assessment

### Response Timeline

| Stage | Timeline |
|-------|----------|
| Acknowledgment | Within 48 hours |
| Initial assessment | Within 1 week |
| Fix development | Depends on severity |
| Public disclosure | After fix is released |

## Scope

### In Scope

- Agent loop and LLM interaction (`pkg/agent/`)
- Tool implementations and sandbox escapes (`pkg/tools/`)
- Channel handlers and input validation (`pkg/channels/`)
- Credential and API key handling (`pkg/config/`)
- Swarm manager and sub-agent isolation (`pkg/tools/swarm.go`)
- Session storage and memory (`pkg/session/`, `pkg/memory/`)
- Prompt injection and DLP guards (`workspace/skills/prompt-injection-guard/`, `workspace/skills/clawwall/`)

### Out of Scope

- Issues in vendored dependencies (`vendor/`) — report these to their upstream projects
- Issues requiring physical access to the machine
- Social engineering attacks
- Denial of service through expected resource usage

## Responsible Disclosure

We ask that you:

1. Give us reasonable time to fix the issue before public disclosure
2. Make a good faith effort to avoid data destruction and service disruption
3. Do not access or modify data belonging to others
4. Act in good faith to avoid degrading our services

We commit to:

1. Acknowledging your report promptly
2. Keeping you informed of our progress
3. Crediting you (if desired) when we publish the fix
4. Not pursuing legal action against good-faith security researchers
