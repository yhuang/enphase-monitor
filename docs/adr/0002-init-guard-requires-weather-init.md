# Every report mode requires a prior `--init` (weather location), gating the core on an enrichment

All report-generating modes (`--date`, `--continuous`, `--true-up`, `--backfill-from`, and the default today query) refuse to run until `--init` has cached the systems' location, exiting with `not initialized — run --init first`. Only cache-management commands and `--update-refresh-tokens` are exempt. `--init`'s primary job is resolving location for Weather Enrichment (it also writes the static WMO weather-code legend), so this makes the weather subsystem a hard prerequisite for energy reporting — even though energy reporting needs no coordinates.

## Considered Options

**Scope the guard to weather-dependent paths only**: require `--init` for Backfill Mode (whose records should carry weather) but let a plain energy `--date`/today report run without it, printing the existing "run --init for weather" notice and omitting weather. Dependency-pure — an enrichment never blocks the core — but adds a branch and a half-initialized state (energy works, weather and the weather-code legend silently absent).

**Hard guard on all report modes** (chosen): one simple rule; weather is a first-class prerequisite. Justified by this project's end goal — a year of per-day energy **and** weather records for correlation analysis. For that purpose a record without weather is half-useless, so failing fast until weather is wired up is desirable, and backfill records carry weather consistently.

## Consequences

Energy reporting depends on the weather subsystem being initialized: if geocoding/Open-Meteo setup fails at first run, no energy report runs until `--init` succeeds, and a user who does not care about weather still pays the one-time initialization cost. This is intentional given the correlation objective. The guard lives in `main.go` and checks `location.NewResolver().CachedPrimaryCoordinates()`. See the "Initialization" entry in CONTEXT.md.
