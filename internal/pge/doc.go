// Package pge turns the user's own PG&E electric interval data into per-day
// history records, and keeps the original download so those records can be
// re-derived and audited at any time.
//
// PG&E is a wholly separate data source from the Enphase Cloud API the rest of
// this application uses: it is metered at the single utility service meter rather
// than per System, and its Green Button export is ESPI XML delivered through the
// PG&E web portal rather than JSON from an API. The package shares none of the
// Enphase OAuth, credential-pool, or API Budget machinery. The binary reaches it
// through --pge-web-only (the browser download), --pge-reprocess and
// --verify-pge-raw.
//
// The data flow:
//
//   - BrowserPull: opens a headed Chrome session on a persistent profile (sign in
//     once; later runs reuse it), drives PG&E's Green Button export form, and
//     captures the download. The untouched download (ZIP or XML) is first archived
//     under raw/pge/ and listed in the raw manifest. ingestDownload then extracts
//     the electric XML, parses it (ParseReadings), aggregates it by local day
//     (AggregateReadingsByDay), runs the write-time sanity checks (CheckDays) and
//     writes one pge-<date>.json per day (WriteHistory).
//
//   - ReprocessRaw (--pge-reprocess): the same ingest path, run on a copy of an
//     archived download, so a parser fix is applied to data already collected
//     without a new pull.
//
//   - VerifyPGERaw (--verify-pge-raw): re-hashes raw/pge against the manifest,
//     re-derives every archive into a scratch directory and diffs it against
//     history/, all without network access.
//
// What the feed means (registers, the local-clock timestamp encoding, the Wh
// scale) is documented at the top of espi.go; sanity.go holds the write-time
// warnings (negative import, wrong interval count for the local day, net more than
// 10% away from Enphase's).
package pge
