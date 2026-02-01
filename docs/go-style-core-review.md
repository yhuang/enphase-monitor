# go-style-core full review

Review date: 2026-01-31. Criteria: go-style-core (clarity, simplicity, concision, maintainability, consistency, formatting, reduce nesting, unnecessary else). **Nesting: flag anything beyond 2 levels.**

---

## 1. Nesting beyond 2 levels (production code) — FIXED

| File | Line(s) | Depth | Context | Fix |
|------|---------|-------|---------|-----|
| `internal/oauth/oauth.go` | 233–241 | 3 | `if != OK` → `if Unauthorized` → `if RefreshToken != ""` | Two sequential `if` with early return; max 2 levels. |
| `internal/timezone/timezone.go` | 41–44 | 3 | `if err` → `if UTC` → `if fallback err == nil` | Early return when `err == nil`; then two level-2 `if`s. |
| `internal/cache/cli.go` | 315–322 | 3 | `for` → `if entry.Date == ""` → `if err` / `if cachedDateStr` | Inlined date check; kept `if err { continue }` at 2 levels. |
| `internal/aggregator/aggregator.go` | 112–120 | 3 | `for` → `if err` → `if IsRateLimitError` | `if err && !IsRateLimitError { return }; if err { append; continue }` — 2 levels. |
| `internal/api/client.go` | 592–596 | 3 | `if CacheDisabled` → `if 429` → `if cacheErr` | Split: `if 429 && cacheErr != nil { return }; if 429 { ... return }` — 2 levels. |
| `internal/validation/validation.go` | 111–118, 144–152 | 3 | `for` → `for` → `if` | Helpers `findSystemByID`, `runMetricTests`; loop body at 2 levels. |

**Test files:** 3+ levels in table-driven tests (`for` → `t.Run` → `if`) left as-is; idiomatic.

---

## 2. Unnecessary else (production code) — FIXED

| File | Line(s) | Context | Fix |
|------|---------|---------|-----|
| `internal/oauth/oauth.go` | 199–207 | `if RefreshToken { } else if Username/Password { } else { return }` | Two `if` blocks + `if formData == nil { return error }`. |
| `internal/display/display.go` | 115–119 | `if cacheUsed { "(cached)" } else { "(live)" }` | Default + override: `sourceLabel := "(live)"`; `if cacheUsed { sourceLabel = "(cached)" }`. |
| `internal/config/config.go` | 259–262 | `if len==6 { } else if len==3 { } else { return "" }` | `if len != 6 && len != 3 { return "" }`; then `if len == 6 { } else { }`. |
| `internal/oauth/setup.go` | 111–120 | `if openBrowser err { manual } else { opened }` | Shared `fmt.Println(authURL)` after branch; different prefix only. |
| `internal/timezone/timezone.go` | 54–58 | `if !targetDate.IsZero() { date = ... } else { date = ... }` | Default + override: `date := time.Now().In(tz)`; `if !targetDate.IsZero() { date = targetDate.In(tz) }`. |

---

## 3. Other go-style-core checks

- **gofmt:** Applied.
- **MixedCaps:** No snake_case in identifiers (except test names).
- **Line length:** No hard limit; no overly long lines noted.
- **Naked returns:** Not audited in this pass.
- **Clarity / simplicity:** No additional violations flagged.

---

## 4. Summary

- **Nesting:** All production code is now at most **2 levels**; 3-level sites were refactored with early returns, `continue`, or helpers.
- **Unnecessary else:** All listed instances replaced with early return or default + override.
- **Tests:** Table-driven structure unchanged.

---

## 5. Second full round (go-style-core, nesting >2 flagged)

**Date:** 2026-01-31. **Scope:** All production `.go` files (excl. `*_test.go`). **Nesting rule:** Flag anything beyond **2 levels** of nesting.

### 5.1 Nesting audit (production code)

All production code was re-scanned for control-flow nesting depth:

| Package / file | Max depth | Notes |
|----------------|-----------|--------|
| `main.go` | 2 | Sequential `if flags.* { if err ... return }; ...`. |
| `internal/config/config.go` | 2 | `for` + `if`; `if len(hex)==6` + `if err`; `if len(hex)==3` + `if err1...`. |
| `internal/oauth/oauth.go` | 2 | `if resp.StatusCode != OK` with two sequential inner `if`s (early return). |
| `internal/oauth/setup.go` | 2 | `if redirectURI == ""` + `if err` / `if redirectURI == ""`; `if strings.Contains("code=")` + `if err == nil`. |
| `internal/validation/validation.go` | 2 | `if err` + `if os.IsNotExist`; `for` + `if actualSys == nil` (helpers keep loop body flat). |
| `internal/display/display.go` | 2 | `if !queryDate.IsZero()`; `if start.Equal(...)`; `for` + no nested `if`. |
| `internal/api/client.go` | 2 | `if err != nil` + `if checkCancelled()` / `if shouldLogError()`; `if cache.CacheDisabled()` + multiple level-2 `if`s; `if resp.StatusCode == 429` + `if cacheErr == nil`; `if resp.StatusCode != 200` + sequential `if`s. |
| `internal/aggregator/aggregator.go` | 2 | `for` + `if apiConfig == nil` / `if err` / `if err && !IsRateLimitError` / `if err` / `if cacheUsed`. |
| `internal/timezone/timezone.go` | 2 | Early return + two level-2 branches in `LoadTimezone`; `if !targetDate.IsZero()`; `if dayEnd.After(now)`. |
| `internal/app/setup.go` | 2 | `if cfg.Colors != nil`; `if dateStr == ""`; `if targetDate.IsZero()`; `if err != nil`; `if !hasCache`. |
| `internal/app/runner.go` | 2 | `if err != nil` + `if IsRateLimitError` / `if ctx.Err()`; `if testMode` + `if testDate.IsZero()` / `if err := validation.ValidateMetrics`. |
| `internal/cli/flags.go` | 1 | No nesting. |
| `internal/cli/cache_commands.go` | 2 | `for` + `if err`; `if date, err := time.Parse(...); err == nil`; `for` + `if err`. |
| `internal/cache/cli.go` | 2 | `for` + multiple `if ... continue`; `FindCacheEntriesByDate` loop + `if entry.Date == ""` (helper); `InspectCacheEntry` single-level `if`s. |
| `internal/cache/cache.go` | 2 | `for` in `getCacheDir` + `if os.Stat` / `if parent == dir`; `NormalizeURLForCache` / `extractQueriedDateFromURL` / `LoadCachedResponse` / `HasCacheForDate`: `if` + single inner `if` or `for` + `if`. |
| `internal/parser/parser.go` | 2 | `if err != nil` + `if len(bodyPreview) > ...`; `for` + `switch`. |
| `internal/urlbuilder/urlbuilder.go` | 1 | No nesting. |

**Result:** No production code has nesting **beyond 2 levels**. Nothing to fix for nesting in this round.

### 5.2 Unnecessary else (second round)

| File | Line(s) | Context | Fix |
|------|---------|---------|-----|
| `internal/cache/cli.go` | 257–262 | `if cached.QueriedDate != "" { fmt.Printf(...) } else { fmt.Printf(...) }` in `InspectCacheEntry` | Default + override: `queriedDateLine := "Queried Date: (not stored - ...)\n"`; `if cached.QueriedDate != "" { queriedDateLine = fmt.Sprintf(...) }`; `fmt.Printf(queriedDateLine)`. |

**Result:** One unnecessary else fixed; no other production `else` violations found.

### 5.3 Other go-style-core (second round)

- **Formatting:** No change; files already gofmt-compliant.
- **Reduce nesting:** Confirmed max depth 2 in production; no new refactors.
- **Unnecessary else:** One fix applied (cache InspectCacheEntry).

### 5.4 Second-round summary

- **Nesting:** Re-audit of all production Go files: **no nesting beyond 2 levels**; nothing flagged.
- **Unnecessary else:** **1** remaining instance in `internal/cache/cli.go` (`InspectCacheEntry`) — **fixed** with default + override.
- **Tests:** Not re-audited for nesting/else; test `else` / `else if` (path routing, assertions) left as-is.

---

## 6. Documentation and code comments (aligned with recent changes)

After the go-style-core refactors, the following were updated so docs and comments match the current codebase:

- **GO_BEST_PRACTICES.md**: File references updated from legacy names (`api_cache.go`, `cloud_client.go`) to actual paths (`internal/cache/cache.go`, `internal/api/client.go`). Added **§4 Control flow (reduce nesting)** describing max 2 levels, default+override, and early returns. Section numbering 4–10 adjusted.
- **ARCHITECTURE.md**: All references to `cloud_client.go` and `api_cache.go` updated to `internal/api/client.go` and `internal/cache/cache.go`. `main.go` line count updated to ~207.
- **GO_CONCEPTS.md**: All `Location` references updated from `response_parser.go`, `cloud_client.go`, `api_cache.go` to `internal/parser/parser.go`, `internal/api/client.go`, `internal/cache/cache.go`.
- **internal/parser/parser.go**: Package comment corrected from "Package main - response_parser.go" to "Package parser" and cross-references to API docs updated to `internal/api/client.go`.
- **internal/cache/cli.go**: Comment on `InspectCacheEntry` QueriedDate display notes "default + override" to match the refactor.

---

## 7. Tests for refactor helpers (aligned with recent changes)

After adding helpers during go-style-core refactoring, unit tests were added so helper behavior is covered and documented:

| Package | Helper | Test(s) | What is tested |
|---------|--------|---------|----------------|
| **validation** | `findSystemByID` | `TestFindSystemByID` | Empty slice (nil), no match, match first/middle/last. |
| **validation** | `runMetricTests` | `TestRunMetricTests` | Empty cases (0,0,false), one pass/fail, mixed, all pass. |
| **cache** | `tryAppendEntryByCachedAt` | `TestTryAppendEntryByCachedAt` | Match (CachedAt date = target), no match, load error (bad path). |
| **api** | `tryLoadPastDateCache` | `TestTryLoadPastDateCache_InvalidURL`, `TestTryLoadPastDateCache_NoMatch` | Invalid URL → (nil, false); valid URL but no matching cache → (nil, false). |

- **internal/validation/validation_test.go**: New section "TESTING REFACTOR HELPERS" with table-driven tests for `findSystemByID` and `runMetricTests`; doc comments explain that these helpers keep `ValidateMetrics` nesting at most 2 levels.
- **internal/cache/cli_test.go**: `TestTryAppendEntryByCachedAt` uses `setupCacheDir` and `createMockCacheFile` to exercise the helper when `entry.Date` is empty (fallback to CachedAt from file).
- **internal/api/client_test.go**: `TestTryLoadPastDateCache_*` tests the past-date cache fallback helper without starting an HTTP server or seeding the cache.
