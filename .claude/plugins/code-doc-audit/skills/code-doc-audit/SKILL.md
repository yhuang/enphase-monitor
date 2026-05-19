---
name: code-doc-audit
description: Audit documentation and inline comments against the current code for **concept, design, and implementation accuracy**. Verifies that the behaviors, business rules, file/test inventories, and CLI flags described in markdown and source comments still match what the code does — and surfaces key concepts present in the code but missing from the docs. Use when the user asks for a "thorough code base analysis against documentation", a "doc audit", to "make sure docs align with the code conceptually", or after a feature/refactor that changes behavior. For pure file:line / code-snippet drift, use **doc-sync** instead (this skill explicitly does NOT re-check line numbers). Repeat rounds until a full pass finds no further changes.
argument-hint: [optional path to scope the audit]
allowed-tools: Bash(grep:*), Bash(find:*), Bash(wc:*), Bash(ls:*), Bash(jq:*), Bash(go:*), Read, Edit, Write
---

# Code/Doc Audit (Concepts, Design, Implementation)

Verify that every markdown file and every inline source comment describes the codebase **accurately and comprehensively** at the level of **concepts, design, and behavior** — and add documentation for key behaviors that exist in code but aren't documented.

> **Scope boundary.** This skill deliberately does **not** check file:line references or whether quoted code snippets match the current source character-for-character. Those checks belong to **doc-sync**. If a stale line number is the only problem, this skill should leave it alone and (optionally) note that doc-sync should be re-run. Conversely, if a code snippet's *meaning* has drifted from current behavior (inverted condition, removed branch, renamed concept), that **is** in scope here — fix the description regardless of whether the line number is right.

## When to invoke

- "Review the codebase and make sure the docs and inline comments reflect it."
- "Do a thorough doc audit of concepts and behavior."
- "Are there any business rules in the code that aren't documented?"
- After a behavior-changing refactor or new feature.
- Before cutting a release.
- After a long stretch of work where docs were added in passing.

Pair with **doc-sync** afterward for the file:line / snippet check.

## Procedure

### Step 1 — Inventory the codebase

```bash
# Production source
find . -name "*.go" -not -path "*/.git/*" -not -name "*_test.go" | sort

# Test source
find . -name "*_test.go" -not -path "*/.git/*" | sort

# All markdown
find . -name "*.md" -not -path "*/.git/*" -not -path "*/.claude/*" | sort
```

Map the result. You should know: every package, every test file, every markdown file, and which exported symbols exist in each.

### Step 2 — Identify recent behavior changes

If the user implies "since the last audit", inspect recent commits and `git status` for modified files. Pay special attention to changes that move semantics:
- New exported functions / types / constants that change a user-visible contract.
- New CLI flags.
- Behavior changes in flow-control functions (e.g. caching, error handling, retry policies, rate limiting).
- Renamed, deleted, or relocated concepts (a renamed function often leaves its old name in docs).
- New test files that cover a new behavior the docs don't mention.

Run `git log --since="2 weeks ago" --oneline` and skim recent commits for "fix", "refactor", "replace", "rename", "remove" — those are the high-signal signals for doc drift.

### Step 3 — Verify file / test inventories

Many docs include directory trees or test-file tables. For each tree/table entry, confirm the file exists and the description still matches.

```bash
# Find inventory-style mentions (directory trees and test tables)
grep -rn "├──\s*[a-z_.-]*\.go\|^\s*-\s*\`[a-z_-]*\.go\`" docs/ README.md QUICKSTART.md

# For each claimed file, verify
ls path/to/claimed/file
```

Common drifts:
- A test file is renamed / removed but the tree still lists it.
- A file's stated *purpose* (the comment to the right of `├──`) drifted because the file's role changed.

Fix by removing stale rows, updating descriptions, or adding new rows for new files. If coverage moved to a different file, add a one-sentence pointer rather than just removing.

### Step 4 — Verify behavioral claims (the core of this skill)

For each markdown paragraph that describes program behavior — caching policy, rate-limit handling, error fallbacks, timezone rules, query types, etc. — open the source and confirm the claim is still true. Watch for:

- **Inverted conditions.** "X happens when Y" but the code now does the opposite.
- **Removed branches.** "For past dates, …" but past-date handling has been folded into a single rule.
- **Renamed concepts.** Old name (e.g. `MarkAPICall`) cited where the code now uses the new name (`RecordAPICall`).
- **Stale magnitudes.** "60-second cache lifetime" when the rule is now per-query-type. "20+ constants" when there are 56 — significantly off should be updated.
- **Obsolete files / markers.** "a `last_api_call` file holds the timestamp" when the marker has been replaced by a sliding-window log.
- **Promises the code no longer keeps.** "Cache is never deleted" when the test cleanup wipes it.

Be skeptical of behavioral examples in docs. Trace at least one execution path through the code for each behavior the docs claim. If the *meaning* has drifted, rewrite the description — even if the line number reference happens to still be correct.

### Step 5 — Verify CLI / flag documentation

```bash
grep -n "BoolVar\|StringVar\|IntVar\|Float64Var" internal/cli/flags.go
```

For each flag, confirm:
- README's CLI flag list mentions it (correct name, correct description).
- Help text on the flag matches the documented description.
- Any flag with non-trivial output (e.g. `--debug`, `--continuous`, `--true-up`) has a dedicated subsection in README or ARCHITECTURE showing what it does and when to use it.
- Removed flags are not still listed.

### Step 6 — Find missing concept coverage

A concept counts as "key" if **at least one** of these is true:
- It is exported and is a non-trivial behavior contract (e.g. `cacheMaxAge`, `QueryCost`, `RemainingBudget`).
- It changes user-visible behavior (CLI flags, environment variables, output format).
- It is referenced in another doc but defined nowhere readable.
- It encodes a business rule that isn't obvious from the type signature (e.g. "past true-up year cache never expires because the totals are immutable").
- It has multiple call sites and a non-obvious invariant.

For each key concept, check if it appears in:
- `README.md` (user-facing)
- `docs/ARCHITECTURE.md` (designer-facing)
- The package doc comment of the package that owns it
- Inline doc comments on the exported symbol

If the concept is undocumented or only mentioned in passing, **add a section** in the most appropriate doc explaining:
- **What** the concept is (one sentence)
- **When** it applies (the trigger or branch condition)
- **Why** it exists (the business rule or design pressure that forced it)

Prefer adding to the doc that already discusses adjacent concepts (e.g. add a new throttle rule to the existing "Rate Limiting" section, not to a fresh section). Don't create new top-level files unless absolutely necessary.

### Step 7 — Verify inline source comments

```bash
# Find inline comments that describe behavior, magnitudes, or cross-references
grep -rn "// .*[0-9]\+ lines\|// .*TODO\|// .*FIXME\|// .*\.go" internal/ main.go

# Find package doc comments (// Package foo …) and skim for stale claims
grep -rn "^// Package " internal/ main.go
```

Spot-check comments that:
- Cite a line count or file size (these rot).
- Cross-reference another file by purpose ("see foo.go for the X handler").
- Describe a multi-step algorithm — does the code still match the steps?
- Reference a removed helper by name.
- Describe a "why" that has since been superseded.

Comments that simply restate the code below (`// Set x to 5` above `x = 5`) are not in scope — flag those for removal separately if egregious, but the focus here is *meaning drift*, not redundancy.

### Step 8 — Apply fixes

Edit in priority order, smallest blast radius first:

| Issue | Action |
|-------|--------|
| Stale file in inventory | Remove the entry; add prose pointing to where the coverage now lives if appropriate |
| Stale concept description | Rewrite to current behavior |
| Stale magnitude / count | Update if significantly off; leave if still approximate |
| Stale inline comment | Update or delete |
| Missing concept | Add a subsection in the most appropriate doc |
| Stale CLI flag | Sync README's CLI list with `internal/cli/flags.go` |

When rewriting a paragraph, preserve the *structure* of the surrounding doc (headings, table format, bullet style) so the diff stays minimal and reviewable.

### Step 9 — Verify with tests

```bash
go build ./...
go test ./...
```

The build catches stale exported names referenced in code that I forgot to update. Tests catch behavioral regressions if I accidentally edited code while editing comments.

### Step 10 — Repeat

Go back to Step 3. Keep iterating until a full round produces zero changes.

> **Termination**: stop only when a complete pass finds nothing to fix and `go test ./...` is green. A round that fixes items must be followed by a verification round.

### Step 11 — (Optional) hand off to doc-sync

After this skill finishes, line numbers and code snippets may now be drifted because doc text shifted around. Suggest the user run `/doc-sync` next to mop up file:line and snippet drift.

## Output format

When done, summarize for the user in this shape:

```
**Stale concepts fixed:**
| File | Was | Now |
| ... | ... | ... |

**Missing concepts added:**
| File | Concept | Section |
| ... | ... | ... |

**Inventory fixes:**
| File | Change |
| ... | ... |

**Tests:** clean.

Recommend running /doc-sync next to verify file:line refs.
```

Keep the summary terse — the diff is the proof of work. Do NOT include line-number changes in this summary; if line numbers happen to shift, that's doc-sync's territory.

## Common stale patterns to watch for

- "Throttle" / "marker" terminology when the implementation has moved on (e.g. single-timestamp marker → sliding-window counter).
- New CLI flags added to `internal/cli/flags.go` without a corresponding entry in README's flag list or a dedicated subsection explaining the output.
- `--debug` / log-format samples in docs drifting from what the code actually prints.
- "MTD/YTD cache for 60 seconds" when the policy is now per-query-type (∞ / 1h / 24h).
- Test inventory rows for files that were renamed or merged.
- "X test file covers Y" claims when Y has moved to a different test file.
- Package doc comments that list features the package no longer owns.

## Quick first-pass commands

```bash
# Recently modified production code (likely sources of doc drift)
git log --since="2 weeks ago" --name-only --pretty=format: -- '*.go' | grep -v _test.go | sort -u | grep -v '^$'

# Inventory-style file mentions in docs
grep -rn "├──\s*[a-z_.-]*\.go\|^\s*-\s*\`[a-z_-]*\.go\`" docs/ README.md

# Exported symbols added recently (potential undocumented concepts)
git log --since="2 weeks ago" -p -- '*.go' | grep -E "^\+func [A-Z]|^\+type [A-Z]|^\+const [A-Z]|^\+var [A-Z]" | sort -u

# CLI flags currently defined
grep -n "BoolVar\|StringVar\|IntVar" internal/cli/flags.go
```
