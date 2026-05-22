# True-Up Window end date caps at 12-month cycle boundary for closed periods

The `--true-up` report covers a True-Up Window: from the first day of the start month through either yesterday (in-progress cycle) or the last day of the 12-month window (closed historical cycle), whichever comes first. We chose `min(yesterday, start_month + 12 months - 1 day)` rather than always using yesterday.

## Considered Options

**Always use yesterday** (prior behavior): simpler, but silently returns more than 12 months of data for historical queries — a 2024 True-Up start date queried in 2026 would span 17+ months, making the totals meaningless as a True-Up balance.

**Always use 12-month boundary**: would make in-progress queries show a future end date with incomplete data, which is misleading.

**`min(yesterday, cycle end)`** (chosen): correctly handles both cases — in-progress cycles stop at the latest complete day; closed cycles stop at the actual 12-month boundary. This matches the PG&E True-Up definition exactly.

## Consequences

The end date shown in the report will differ depending on whether the cycle is open or closed, which may surprise a reader who expects a fixed rule. See `trueUpWindowEnd` in `internal/app/trueup.go`.
