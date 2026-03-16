# TESTING.md — dep-audit Test Plan

Run all tests from the `studio/skills/dep-audit/` directory.

## Prerequisites

```bash
# Ensure jq is installed
jq --version    # should output jq-1.6 or later

# Ensure npm is available
npm --version
```

## Test 1: Detection — Empty Directory

```bash
mkdir -p /tmp/dep-audit-test-empty
bash scripts/detect.sh /tmp/dep-audit-test-empty
```

**Expected:** JSON output with `"lockfiles": []` and tool availability listed.

## Test 2: Detection — Multiple Ecosystems

```bash
mkdir -p /tmp/dep-audit-test-multi
echo '{}' > /tmp/dep-audit-test-multi/package-lock.json
echo 'requests==2.28.0' > /tmp/dep-audit-test-multi/requirements.txt

bash scripts/detect.sh /tmp/dep-audit-test-multi
```

**Expected:** JSON `lockfiles` array contains entries for both `npm` and `python`.

**Verify:**
```bash
bash scripts/detect.sh /tmp/dep-audit-test-multi | jq '.lockfiles | length'
# Should output: 2
```

## Test 3: npm Audit — Real Project

```bash
mkdir -p /tmp/dep-audit-test-npm && cd /tmp/dep-audit-test-npm
npm init -y
npm install lodash@4.17.15 --save --package-lock-only 2>/dev/null

bash scripts/audit-npm.sh /tmp/dep-audit-test-npm
```

**Expected:** JSON with `.ecosystem = "npm"`, `.summary.total > 0`, and at least one vulnerability entry.

**Verify:**
```bash
bash scripts/audit-npm.sh /tmp/dep-audit-test-npm | jq '.summary.total'
# Should be > 0
```

## Test 4: pip Audit (if pip-audit installed)

```bash
mkdir -p /tmp/dep-audit-test-pip
echo 'requests==2.25.0' > /tmp/dep-audit-test-pip/requirements.txt

bash scripts/audit-pip.sh /tmp/dep-audit-test-pip
```

**Expected:** JSON with findings for `requests` 2.25.0 (known CVEs).

## Test 5: Aggregation

```bash
# Create two mock result files
cat > /tmp/npm-result.json << 'JSON'
{
  "ecosystem": "npm",
  "directory": "/tmp/project",
  "scan_time": "2026-02-14T22:00:00Z",
  "summary": {"critical":1,"high":2,"moderate":0,"low":1,"info":0,"total":4},
  "vulnerabilities": [
    {"id":"GHSA-0001","package":"lodash","installed_version":"4.17.15","severity":"critical","title":"Prototype Pollution","url":"https://example.com","fix_available":true,"fix_command":"npm audit fix","patched_version":">=4.17.21"}
  ]
}
JSON

cat > /tmp/pip-result.json << 'JSON'
{
  "ecosystem": "python",
  "directory": "/tmp/project",
  "scan_time": "2026-02-14T22:00:00Z",
  "summary": {"critical":0,"high":1,"moderate":1,"low":0,"info":0,"total":2},
  "vulnerabilities": [
    {"id":"PYSEC-2023-001","package":"requests","installed_version":"2.25.0","severity":"high","title":"Unintended Proxy","url":"https://osv.dev","fix_available":true,"fix_command":"pip install requests>=2.31.0","patched_version":"2.31.0"}
  ]
}
JSON

bash scripts/aggregate.sh /tmp/npm-result.json /tmp/pip-result.json 1>/tmp/unified.json 2>/tmp/report.md
```

**Expected:**
- JSON in `/tmp/unified.json`: `summary.total = 6`, `summary.critical = 1`, `summary.high = 3`
- Markdown report in `/tmp/report.md`

**Verify JSON:**
```bash
jq '.summary.total' /tmp/unified.json
# Should output: 6
```

**Verify Markdown:**
```bash
grep -q "Critical & High Findings" /tmp/report.md && echo "PASS: Markdown has findings table" || echo "FAIL"
grep -q "lodash" /tmp/report.md && echo "PASS: Markdown mentions lodash" || echo "FAIL"
```

## Test 6: Missing Tool / Missing Lockfile Graceful Degradation

```bash
# Rust scan on an empty directory:
# - If cargo-audit is missing, should return "cargo-audit not found"
# - If cargo-audit is installed, should return "Cargo.lock not found"
mkdir -p /tmp/dep-audit-test-rust-empty
bash scripts/audit-cargo.sh /tmp/dep-audit-test-rust-empty
echo "Exit code: $?"
```

**Expected:** JSON error message on stderr, exit code 1. No stack trace or ambiguous success JSON.

## Test 7: No Lockfiles Message

```bash
mkdir -p /tmp/dep-audit-empty
bash scripts/detect.sh /tmp/dep-audit-empty | jq '.lockfiles | length'
# Should output: 0
```

## Test 8: Invalid Directory Must Fail Hard

```bash
bash scripts/audit-npm.sh /path/does/not/exist
echo "Exit code: $?"
```

**Expected:** Exit code 1 and JSON error on stderr:
- contains `"directory not found or not accessible"`
- does **not** output a "0 vulnerabilities" success JSON

## Test 9: Aggregation with Mixed Inputs (Success + Error)

```bash
cat > /tmp/dep-audit-error.json << 'JSON'
{"error":"pip-audit not found","ecosystem":"python","directory":"/tmp/project"}
JSON

bash scripts/aggregate.sh /tmp/npm-result.json /tmp/dep-audit-error.json 1>/tmp/unified-mixed.json 2>/tmp/report-mixed.md
```

**Expected:**
- `/tmp/unified-mixed.json` includes:
  - `status: "ok"` (because one valid ecosystem result exists)
  - `errors` array with the python error object
- `/tmp/report-mixed.md` includes "Skipped / Error Inputs"

## Cleanup

```bash
rm -rf /tmp/dep-audit-test-* /tmp/npm-result.json /tmp/pip-result.json /tmp/dep-audit-error.json /tmp/unified.json /tmp/unified-mixed.json /tmp/report.md /tmp/report-mixed.md /tmp/dep-audit-empty
```
