# go-linting review

Review date: 2026-01-31. Criteria: go-linting SKILL (golangci-lint, errcheck, goimports, revive, govet, staticcheck). Goal: consistent linting across the codebase.

---

## 1. Configuration added

### .golangci.yml

Created at project root with the minimum recommended linters from the go-linting skill:

| Linter      | Purpose                          |
|------------|-----------------------------------|
| errcheck   | Ensure errors are handled         |
| goimports  | Format code and manage imports    |
| revive     | Style (modern replacement for golint) |
| govet      | Analyze code for common mistakes  |
| staticcheck| Static analysis checks            |

**Settings:**

- **goimports:** `local-prefixes: enphase-monitor` so internal imports are grouped under the module path.
- **revive:** Rules enabled: blank-imports, context-as-argument, error-return, error-strings, exported.
- **run:** `timeout: 5m`.

### Makefile

- **`make lint`** — runs `golangci-lint run ./...`.
- **`.PHONY`** — updated to include `lint`.
- **`make help`** — documents `make lint` and install command.

---

## 2. Fixes applied

### S1009 (staticcheck): omit nil check before len()

**File:** `internal/cache/cli_test.go`  
**Before:** `if entries != nil && len(entries) > 0`  
**After:** `if len(entries) > 0`

In Go, `len(nil)` is 0, so the nil check is redundant. The comment was updated to say "May be empty when no cache directory or no entries for this date".

### errcheck: explicitly ignore return values in test code

**errcheck** requires that every returned error (and other return values from functions that return errors) is either handled or explicitly ignored. In test code we often call APIs for their side effects; ignoring the return values without assigning them triggers errcheck.

**Pattern:** Use `_, _ = fn(...)` to explicitly ignore both the primary return value and the error. That satisfies the linter and documents intent.

**Examples applied:**

| Location | Before | After |
|----------|--------|--------|
| HTTP test handlers (`http.ResponseWriter.Write`) | `w.Write([]byte(...))` | `_, _ = w.Write([]byte(...))` |
| Draining pipes in tests (`io.Copy`) | `io.Copy(&buf, r)` | `_, _ = io.Copy(&buf, r)` |

**Files touched:** `internal/api/client_functional_test.go`, `internal/api/client_test.go`, `internal/cli/cache_commands_test.go`, `internal/oauth/oauth_functional_test.go`.

**When to use:** In test helpers, mock HTTP handlers (e.g. `httptest`), and when draining stdout/pipe in tests, ignoring the error is acceptable. In production code, prefer handling the error (log, return, or wrap) instead of ignoring it.

---

## 3. How to run

**Run linters (no install required; first run downloads the tool via `go run`):**

```bash
make lint
```

This runs `go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...`, so you do not need to install golangci-lint globally.

**Optional: install globally for faster repeated runs:**

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run ./...
```

**Run on a specific path:**

```bash
go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./internal/...
```

---

## 4. Verification

- **go vet:** `go vet ./...` was run and passes.
- **golangci-lint:** Run `make lint` (uses `go run`; first run downloads the tool).

---

## 5. Optional: CI integration

To run in CI, add a step that installs golangci-lint and runs:

```bash
golangci-lint run ./...
```

Exit non-zero on failure so the pipeline fails when lint issues are reported.

---

## 6. Summary

- **.golangci.yml** added with errcheck, goimports, revive, govet, staticcheck and local-prefix for enphase-monitor.
- **Makefile** updated with `make lint` and help text.
- **Code:** S1009 fix in `internal/cache/cli_test.go` (omit nil check before `len`); errcheck fixes across test files (explicit `_, _ =` for `w.Write` and `io.Copy` in HTTP handlers and pipe-draining tests).
- **Next step:** Run `make lint` (no install required; first run may download the tool).
