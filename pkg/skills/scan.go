package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ScanFinding describes a single security concern found during local content scanning.
type ScanFinding struct {
	File    string // relative path within skill directory
	Line    int    // 1-based line number (0 if not line-specific)
	Pattern string // short label for the matched pattern
	Snippet string // trimmed line content (max 120 chars)
}

// ScanResult holds the outcome of a local skill content scan.
type ScanResult struct {
	Findings []ScanFinding
}

// HasFindings returns true if any security concerns were found.
func (r *ScanResult) HasFindings() bool {
	return len(r.Findings) > 0
}

// Summary returns a human-readable summary of findings for the LLM.
func (r *ScanResult) Summary() string {
	if !r.HasFindings() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Security scan found %d concern(s):\n", len(r.Findings)))
	for i, f := range r.Findings {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.Findings)-10))
			break
		}
		if f.Line > 0 {
			sb.WriteString(fmt.Sprintf("  - [%s] %s:%d — %s\n", f.Pattern, f.File, f.Line, f.Snippet))
		} else {
			sb.WriteString(fmt.Sprintf("  - [%s] %s — %s\n", f.Pattern, f.File, f.Snippet))
		}
	}
	return sb.String()
}

// dangerousPatterns are regex patterns that indicate potentially dangerous content
// in skill files. Each pattern has a label used in findings.
var dangerousPatterns = []struct {
	label   string
	pattern *regexp.Regexp
}{
	// Shell injection / reverse shells
	{"reverse-shell", regexp.MustCompile(`(?i)(nc\s+-[elp]|ncat\s+-|mkfifo|/dev/tcp/|bash\s+-i\s+>&)`)},
	{"curl-pipe-sh", regexp.MustCompile(`(?i)curl\s+.*\|\s*(ba)?sh`)},
	{"wget-pipe-sh", regexp.MustCompile(`(?i)wget\s+.*\|\s*(ba)?sh`)},

	// Credential/env exfiltration
	{"env-exfil", regexp.MustCompile(`(?i)(curl|wget|nc|fetch)\s+.*(\$\{?\w*KEY|TOKEN|SECRET|PASS|CRED)`)},
	{"env-dump", regexp.MustCompile(`(?i)printenv\s*\||(env|set)\s*>\s*/`)},

	// Filesystem attacks
	{"rm-rf-root", regexp.MustCompile(`rm\s+-[rR]f\s+/[^a-z]`)},
	{"chmod-world", regexp.MustCompile(`chmod\s+(777|a\+[rwx]+)\s+/`)},

	// Crypto miners / known malware patterns
	{"crypto-miner", regexp.MustCompile(`(?i)(xmrig|cryptonight|stratum\+tcp|minerd|coinhive)`)},

	// Base64 encoded payloads (suspiciously long)
	{"encoded-payload", regexp.MustCompile(`(?i)(echo|printf)\s+.*base64\s+(-d|--decode)`)},

	// Eval of remote content
	{"eval-remote", regexp.MustCompile(`(?i)eval\s*\(\s*\$\((curl|wget|fetch)`)},

	// SSH key theft
	{"ssh-exfil", regexp.MustCompile(`(?i)(cat|cp|scp|curl).*\.ssh/(id_rsa|id_ed25519|authorized_keys)`)},

	// Disk operations
	{"disk-wipe", regexp.MustCompile(`(?i)dd\s+if=/dev/zero\s+of=/dev/`)},
}

// binaryExtensions are file extensions that should not appear in skill packages.
var binaryExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".bin": true, ".com": true, ".msi": true, ".deb": true,
	".rpm": true, ".apk": true, ".elf": true,
}

// ScanSkillContent performs a local security scan on an extracted skill directory.
// It checks for dangerous shell patterns, suspicious file types, and other red flags.
// Returns a ScanResult with any findings; an empty result means the skill looks clean.
func ScanSkillContent(skillDir string) *ScanResult {
	result := &ScanResult{}

	_ = filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(skillDir, path)

		// Check for suspicious binary file types.
		ext := strings.ToLower(filepath.Ext(path))
		if binaryExtensions[ext] {
			result.Findings = append(result.Findings, ScanFinding{
				File:    relPath,
				Pattern: "binary-file",
				Snippet: fmt.Sprintf("unexpected binary file type: %s", ext),
			})
			return nil
		}

		// Skip non-text files (images, fonts, etc.) by checking size and extension.
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		if info.Size() > 1*1024*1024 { // skip files > 1MB for scanning
			return nil
		}

		// Only scan text-like files.
		if !isTextLikeExt(ext) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable files
		}

		lines := strings.Split(string(data), "\n")
		for lineNum, line := range lines {
			for _, dp := range dangerousPatterns {
				if dp.pattern.MatchString(line) {
					snippet := strings.TrimSpace(line)
					if len(snippet) > 120 {
						snippet = snippet[:120] + "..."
					}
					result.Findings = append(result.Findings, ScanFinding{
						File:    relPath,
						Line:    lineNum + 1,
						Pattern: dp.label,
						Snippet: snippet,
					})
				}
			}
		}

		return nil
	})

	return result
}

func isTextLikeExt(ext string) bool {
	textExts := map[string]bool{
		".md": true, ".txt": true, ".sh": true, ".bash": true,
		".py": true, ".js": true, ".ts": true, ".go": true,
		".rb": true, ".pl": true, ".php": true, ".lua": true,
		".yml": true, ".yaml": true, ".json": true, ".toml": true,
		".xml": true, ".html": true, ".css": true, ".sql": true,
		".r": true, ".rs": true, ".c": true, ".cpp": true, ".h": true,
		".java": true, ".kt": true, ".swift": true, ".ps1": true,
		".bat": true, ".cmd": true, ".zsh": true, ".fish": true,
		"": true, // extensionless files (Makefile, Dockerfile, etc.)
	}
	return textExts[ext]
}
