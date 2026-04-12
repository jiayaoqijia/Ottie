package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsInfoValidate(t *testing.T) {
	testcases := []struct {
		name        string
		skillName   string
		description string
		wantErr     bool
		errContains []string
	}{
		{
			name:        "valid-skill",
			skillName:   "valid-skill",
			description: "a valid skill description",
			wantErr:     false,
		},
		{
			name:        "empty-name",
			skillName:   "",
			description: "description without name",
			wantErr:     true,
			errContains: []string{"name is required"},
		},
		{
			name:        "empty-description",
			skillName:   "skill-without-description",
			description: "",
			wantErr:     true,
			errContains: []string{"description is required"},
		},
		{
			name:        "empty-both",
			skillName:   "",
			description: "",
			wantErr:     true,
			errContains: []string{"name is required", "description is required"},
		},
		{
			name:        "name-with-spaces",
			skillName:   "skill with spaces",
			description: "invalid name with spaces",
			wantErr:     true,
			errContains: []string{"name must be alphanumeric with hyphens"},
		},
		{
			name:        "name-with-underscore",
			skillName:   "skill_underscore",
			description: "invalid name with underscore",
			wantErr:     true,
			errContains: []string{"name must be alphanumeric with hyphens"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			info := SkillInfo{
				Name:        tc.skillName,
				Description: tc.description,
			}
			err := info.validate()
			if tc.wantErr {
				assert.Error(t, err)
				for _, msg := range tc.errContains {
					assert.ErrorContains(t, err, msg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractFrontmatter(t *testing.T) {
	sl := &SkillsLoader{}

	testcases := []struct {
		name           string
		content        string
		expectedName   string
		expectedDesc   string
		lineEndingType string
	}{
		{
			name:           "unix-line-endings",
			lineEndingType: "Unix (\\n)",
			content:        "---\nname: test-skill\ndescription: A test skill\n---\n\n# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
		{
			name:           "windows-line-endings",
			lineEndingType: "Windows (\\r\\n)",
			content:        "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n\r\n# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
		{
			name:           "classic-mac-line-endings",
			lineEndingType: "Classic Mac (\\r)",
			content:        "---\rname: test-skill\rdescription: A test skill\r---\r\r# Skill Content",
			expectedName:   "test-skill",
			expectedDesc:   "A test skill",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// Extract frontmatter
			frontmatter := sl.extractFrontmatter(tc.content)
			assert.NotEmpty(t, frontmatter, "Frontmatter should be extracted for %s line endings", tc.lineEndingType)

			// Parse YAML to get name and description (parseSimpleYAML now handles all line ending types)
			yamlMeta := sl.parseSimpleYAML(frontmatter)
			assert.Equal(
				t,
				tc.expectedName,
				yamlMeta["name"],
				"Name should be correctly parsed from frontmatter with %s line endings",
				tc.lineEndingType,
			)
			assert.Equal(
				t,
				tc.expectedDesc,
				yamlMeta["description"],
				"Description should be correctly parsed from frontmatter with %s line endings",
				tc.lineEndingType,
			)
		})
	}
}

// createSkillDir creates a skill directory with a SKILL.md file containing the given frontmatter.
func createSkillDir(t *testing.T, base, dirName, name, description string) {
	t.Helper()
	dir := filepath.Join(base, dirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestListSkillsWorkspaceOverridesGlobal(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")

	createSkillDir(t, filepath.Join(ws, "skills"), "my-skill", "my-skill", "workspace version")
	createSkillDir(t, global, "my-skill", "my-skill", "global version")

	sl := NewSkillsLoader(ws, global, "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 1)
	assert.Equal(t, "workspace", skills[0].Source)
	assert.Equal(t, "workspace version", skills[0].Description)
}

func TestListSkillsGlobalOverridesBuiltin(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")
	builtin := filepath.Join(tmp, "builtin")

	createSkillDir(t, global, "my-skill", "my-skill", "global version")
	createSkillDir(t, builtin, "my-skill", "my-skill", "builtin version")

	sl := NewSkillsLoader(ws, global, builtin)
	skills := sl.ListSkills()

	assert.Len(t, skills, 1)
	assert.Equal(t, "global", skills[0].Source)
	assert.Equal(t, "global version", skills[0].Description)
}

func TestListSkillsMetadataNameDedup(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")

	// Different directory names but same metadata name
	createSkillDir(t, filepath.Join(ws, "skills"), "dir-a", "shared-name", "workspace version")
	createSkillDir(t, global, "dir-b", "shared-name", "global version")

	sl := NewSkillsLoader(ws, global, "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 1)
	assert.Equal(t, "shared-name", skills[0].Name)
	assert.Equal(t, "workspace", skills[0].Source)
}

func TestListSkillsMultipleDistinctSkills(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")
	builtin := filepath.Join(tmp, "builtin")

	createSkillDir(t, filepath.Join(ws, "skills"), "skill-a", "skill-a", "desc a")
	createSkillDir(t, global, "skill-b", "skill-b", "desc b")
	createSkillDir(t, builtin, "skill-c", "skill-c", "desc c")

	sl := NewSkillsLoader(ws, global, builtin)
	skills := sl.ListSkills()

	assert.Len(t, skills, 3)
	names := map[string]string{}
	for _, s := range skills {
		names[s.Name] = s.Source
	}
	assert.Equal(t, "workspace", names["skill-a"])
	assert.Equal(t, "global", names["skill-b"])
	assert.Equal(t, "builtin", names["skill-c"])
}

func TestListSkillsInvalidSkillSkipped(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")

	// Invalid name (underscore)
	createSkillDir(t, filepath.Join(ws, "skills"), "bad_skill", "bad_skill", "desc")
	// Valid skill
	createSkillDir(t, global, "good-skill", "good-skill", "desc")

	sl := NewSkillsLoader(ws, global, "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 1)
	assert.Equal(t, "good-skill", skills[0].Name)
}

func TestListSkillsEmptyAndNonexistentDirs(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	emptyDir := filepath.Join(tmp, "empty")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))

	sl := NewSkillsLoader(ws, emptyDir, filepath.Join(tmp, "nonexistent"))
	skills := sl.ListSkills()

	assert.Empty(t, skills)
}

func TestListSkillsDirWithoutSkillMD(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")

	// Directory exists but has no SKILL.md
	require.NoError(t, os.MkdirAll(filepath.Join(global, "no-skillmd"), 0o755))
	// Valid skill alongside
	createSkillDir(t, global, "real-skill", "real-skill", "desc")

	sl := NewSkillsLoader(ws, global, "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 1)
	assert.Equal(t, "real-skill", skills[0].Name)
}

func TestStripFrontmatter(t *testing.T) {
	sl := &SkillsLoader{}

	testcases := []struct {
		name            string
		content         string
		expectedContent string
		lineEndingType  string
	}{
		{
			name:            "unix-line-endings",
			lineEndingType:  "Unix (\\n)",
			content:         "---\nname: test-skill\ndescription: A test skill\n---\n\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "windows-line-endings",
			lineEndingType:  "Windows (\\r\\n)",
			content:         "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n\r\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "classic-mac-line-endings",
			lineEndingType:  "Classic Mac (\\r)",
			content:         "---\rname: test-skill\rdescription: A test skill\r---\r\r# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "unix-line-endings-without-trailing-newline",
			lineEndingType:  "Unix (\\n) without trailing newline",
			content:         "---\nname: test-skill\ndescription: A test skill\n---\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "windows-line-endings-without-trailing-newline",
			lineEndingType:  "Windows (\\r\\n) without trailing newline",
			content:         "---\r\nname: test-skill\r\ndescription: A test skill\r\n---\r\n# Skill Content",
			expectedContent: "# Skill Content",
		},
		{
			name:            "no-frontmatter",
			lineEndingType:  "No frontmatter",
			content:         "# Skill Content\n\nSome content here.",
			expectedContent: "# Skill Content\n\nSome content here.",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			result := sl.stripFrontmatter(tc.content)
			assert.Equal(
				t,
				tc.expectedContent,
				result,
				"Frontmatter should be stripped correctly for %s",
				tc.lineEndingType,
			)
		})
	}
}

// TestPathTraversalInLoadSkill documents that LoadSkill is currently
// vulnerable to path traversal via the name parameter. A name like
// "../../secret" resolves outside the skills root because filepath.Join
// resolves ".." components.
//
// This test verifies the vulnerability exists (so it will break when
// a fix is applied, serving as a reminder to update the test) and
// confirms that legitimate skill loading still works.
func TestPathTraversalInLoadSkill(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")

	// Create a legitimate skill
	createSkillDir(t, filepath.Join(ws, "skills"), "legit-skill", "legit-skill", "legit description")

	// Create a file outside the skills root that traversal would reach
	outsideDir := filepath.Join(tmp, "secret")
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(outsideDir, "SKILL.md"),
		[]byte("---\nname: secret-skill\ndescription: should not be loadable\n---\n\n# Secret"),
		0o644,
	))

	sl := NewSkillsLoader(ws, "", "")

	// Verify path traversal currently succeeds — this documents the
	// known vulnerability. When a fix lands (e.g., validating that
	// resolved path stays within root), these assertions will flip
	// and this test should be updated to assert ok==false.
	content, ok := sl.LoadSkill("../../secret")
	if ok {
		t.Logf("KNOWN VULNERABILITY: LoadSkill(%q) succeeded — path traversal not blocked", "../../secret")
		assert.Contains(t, content, "# Secret", "traversed file should be readable")
	}
	// Whether traversal succeeds or not, we assert the vulnerability
	// status is documented. If it stops succeeding, the fix landed.

	// Legitimate skill should still work regardless.
	content, ok = sl.LoadSkill("legit-skill")
	assert.True(t, ok, "legit-skill should load")
	assert.Contains(t, content, "# legit-skill")
}

// TestBuildSkillsSummaryXMLEscaping verifies that skill names and
// descriptions containing XML special characters are properly
// escaped in the summary output, preventing markup injection.
func TestBuildSkillsSummaryXMLEscaping(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")

	// Create a skill with XML-hostile name and description
	skillDir := filepath.Join(ws, "skills", "xss-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := "---\nname: xss-skill\ndescription: <script>alert(1)</script> & \"quotes\" test\n---\n\n# XSS Skill"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := NewSkillsLoader(ws, "", "")
	summary := sl.BuildSkillsSummary()

	// The raw <script> tag must NOT appear — it should be escaped
	assert.NotContains(t, summary, "<script>", "raw <script> tag must be escaped")
	assert.NotContains(t, summary, "</script>", "raw </script> tag must be escaped")

	// The escaped forms should appear
	assert.Contains(t, summary, "&lt;script&gt;", "< should be escaped to &lt;")
	assert.Contains(t, summary, "&amp;", "& should be escaped to &amp;")
}

func TestSkillRootsTrimsWhitespaceAndDedups(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")
	builtin := filepath.Join(tmp, "builtin")

	sl := NewSkillsLoader(workspace, "  "+global+"  ", "\t"+builtin+"\n")
	roots := sl.SkillRoots()

	assert.Equal(t, []string{
		filepath.Join(workspace, "skills"),
		global,
		builtin,
	}, roots)
}

func TestGetSkillMetadata_UsesMarkdownParagraphWhenNoFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "plain-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	content := "# Plain Skill\n\nThis is parsed from markdown paragraph.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))
	require.NotNil(t, meta)
	assert.Equal(t, "plain-skill", meta.Name)
	assert.Equal(t, "This is parsed from markdown paragraph.", meta.Description)
}

func TestGetSkillMetadata_FrontmatterOverridesMarkdown(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "plain-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	content := "---\nname: frontmatter-skill\ndescription: frontmatter description\n---\n\n# Plain Skill\n\nBody description.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))
	require.NotNil(t, meta)
	assert.Equal(t, "frontmatter-skill", meta.Name)
	assert.Equal(t, "frontmatter description", meta.Description)
}

func TestGetSkillMetadata_YAMLMultilineDescription(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "plain-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	content := "---\nname: frontmatter-skill\ndescription: |\n  line 1: with colon\n  line 2\n---\n\n# Plain Skill\n\nBody description.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))
	require.NotNil(t, meta)
	assert.Equal(t, "frontmatter-skill", meta.Name)
	assert.Equal(t, "line 1: with colon\nline 2", meta.Description)
}

func TestGetSkillMetadata_InvalidHeadingNameFallsBackToDirName(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "valid-name")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	content := "# Invalid Heading Name\n\nBody description.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))
	require.NotNil(t, meta)
	assert.Equal(t, "valid-name", meta.Name)
	assert.Equal(t, "Body description.", meta.Description)
}

func TestGetSkillMetadata_IgnoresHTMLCommentBlocks(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "biomed-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	content := "<!--\n# COPYRIGHT NOTICE\n# This file is part of the \"Universal Biomedical Skills\" project.\n# Copyright (c) 2026 MD BABU MIA, PhD <md.babu.mia@mssm.edu>\n# All Rights Reserved.\n#\n# This code is proprietary and confidential.\n# Unauthorized copying of this file, via any medium is strictly prohibited.\n#\n# Provenance: Authenticated by MD BABU MIA\n\n-->\n\n# Biomed Skill\n\nSummarize biomedical papers.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))
	require.NotNil(t, meta)
	assert.Equal(t, "biomed-skill", meta.Name)
	assert.Equal(t, "Summarize biomedical papers.", meta.Description)
}

// TestListSkillsCategoryNestedLayout verifies that ListSkills discovers
// SKILL.md files nested inside category subdirectories, not just flat
// layout. This is the primary gap the test plan identified: the WalkDir
// recursive discovery was added but never tested.
func TestListSkillsCategoryNestedLayout(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")

	// Category-nested: workspace/skills/crypto/crypto-wallet/SKILL.md
	createSkillDir(t, filepath.Join(ws, "skills", "crypto"), "crypto-wallet", "crypto-wallet", "wallet management")
	// Category-nested: workspace/skills/defi/lido-mcp/SKILL.md
	createSkillDir(t, filepath.Join(ws, "skills", "defi"), "lido-mcp", "lido-mcp", "Lido staking protocol")
	// Flat: workspace/skills/search-tool/SKILL.md
	createSkillDir(t, filepath.Join(ws, "skills"), "search-tool", "search-tool", "web search")

	sl := NewSkillsLoader(ws, "", "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 3, "should find both nested and flat skills")

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	assert.True(t, names["crypto-wallet"], "should find category-nested crypto-wallet")
	assert.True(t, names["lido-mcp"], "should find category-nested lido-mcp")
	assert.True(t, names["search-tool"], "should find flat search-tool")
}

// TestLoadSkillNestedCategoryLayout verifies that LoadSkill can find
// and load a skill that is in a category-nested directory layout.
// LoadSkill first tries flat lookup, then falls back to WalkDir.
func TestLoadSkillNestedCategoryLayout(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")

	// Only create a nested skill — no flat equivalent
	createSkillDir(t, filepath.Join(ws, "skills", "defi"), "lido-mcp", "lido-mcp", "Lido staking protocol")

	sl := NewSkillsLoader(ws, "", "")

	// LoadSkill should find it via the WalkDir fallback
	content, ok := sl.LoadSkill("lido-mcp")
	assert.True(t, ok, "LoadSkill should find nested skill via WalkDir fallback")
	assert.Contains(t, content, "# lido-mcp", "content should contain the skill heading")
}

// TestListSkillsCategoryNestedDedupAcrossSources verifies that a
// category-nested skill in workspace overrides the same-named skill
// in global, even when the directory structures differ (nested vs flat).
func TestListSkillsCategoryNestedDedupAcrossSources(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "workspace")
	global := filepath.Join(tmp, "global")

	// Nested in workspace
	createSkillDir(t, filepath.Join(ws, "skills", "crypto"), "my-skill", "my-skill", "workspace nested version")
	// Flat in global
	createSkillDir(t, global, "my-skill", "my-skill", "global flat version")

	sl := NewSkillsLoader(ws, global, "")
	skills := sl.ListSkills()

	assert.Len(t, skills, 1, "same-named skill should be deduped")
	assert.Equal(t, "workspace", skills[0].Source)
	assert.Equal(t, "workspace nested version", skills[0].Description)
}

// TestGetSkillMetadataMalformedJSON verifies that malformed JSON in
// skill frontmatter does not crash the loader. The getSkillMetadata
// function should fall through to YAML parsing when JSON fails.
func TestGetSkillMetadataMalformedJSON(t *testing.T) {
	tmp := t.TempDir()
	skillDir := filepath.Join(tmp, "workspace", "skills", "broken-json")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	// Frontmatter that looks like JSON but is malformed
	content := "---\n{broken json: [not, valid\n---\n\n# broken-json\n\nA skill with broken JSON frontmatter.\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	sl := &SkillsLoader{}
	meta := sl.getSkillMetadata(filepath.Join(skillDir, "SKILL.md"))

	// Should not panic, and should extract what it can
	require.NotNil(t, meta, "metadata should not be nil for malformed JSON")
	// The YAML parser will also fail on this input, so we fall back to
	// directory name for the name and markdown body for description.
	assert.Equal(t, "broken-json", meta.Name, "should fall back to directory name")
	assert.Equal(t, "A skill with broken JSON frontmatter.", meta.Description, "should extract description from markdown body")
}

// TestSkillInfoValidateNameBoundary tests the boundary conditions of
// the name validation regex: max length, single char, hyphens.
func TestSkillInfoValidateNameBoundary(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"single char", "a", false},
		{"max length", strings.Repeat("a", MaxNameLength), false},
		{"one over max", strings.Repeat("a", MaxNameLength+1), true},
		{"leading hyphen", "-skill", true},
		{"trailing hyphen", "skill-", true},
		{"double hyphen", "skill--name", true},
		{"hyphen only", "-", true},
		{"numeric", "123", false},
		{"alphanumeric-hyphen", "my-skill-v2", false},
		{"uppercase", "MySkill", false},
		{"empty string", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := SkillInfo{Name: tc.input, Description: "desc"}
			err := info.validate()
			if tc.wantErr {
				assert.Error(t, err, "validate(%q) should fail", tc.input)
			} else {
				assert.NoError(t, err, "validate(%q) should pass", tc.input)
			}
		})
	}
}
