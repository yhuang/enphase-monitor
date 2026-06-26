# Go Concepts Reference

This document explains all the intermediate Go concepts that are used throughout the codebase. These concepts were previously explained inline with "GO CONCEPT:" comments, but have been moved here to reduce code noise while still providing learning resources.

> **📚 Learning Path**: 
> - Start with [GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md) for comprehensive Go patterns
> - Use this document as a quick reference when reading code
> - See [ARCHITECTURE.md](ARCHITECTURE.md) for system design patterns

## Table of Contents

1. [Error Handling](#error-handling)
2. [Structs and Pointers](#structs-and-pointers)
3. [Slices and Arrays](#slices-and-arrays)
4. [Control Flow](#control-flow)
5. [Interfaces](#interfaces)
6. [JSON Handling](#json-handling)
7. [Time and Duration](#time-and-duration)
8. [Channels and Signals](#channels-and-signals)
9. [Package-Level Variables](#package-level-variables)
10. [Hash Functions](#hash-functions)
11. [Variable Declarations](#variable-declarations)
12. [Multiple Return Values](#multiple-return-values)
13. [File Organization Patterns](#file-organization-patterns) - includes shared types, interfaces for dependency breaking, and type aliases

---

## Error Handling

### Error Handling Pattern

**Location**: `internal/aggregator/aggregator.go:136-144`

Standard Go pattern: function returns `(result, error)`. We check `err` immediately and return early if non-nil. This is idiomatic Go - errors are values, not exceptions.

```go
accessToken, err := a.getAccessToken(ctx, cred)
if err != nil {
    return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
}
```

### Error Handling with Early Return

**Location**: `internal/parser/parser.go:85`

We check `err` immediately and return early if non-nil. This is idiomatic Go - handle errors as soon as they occur.

```go
if err := json.Unmarshal(bodyBytes, &data); err != nil {
    return nil, fmt.Errorf("failed to decode nested telemetry response (body preview: %s): %w", bodyPreview, err)
}
```

### Error Wrapping with %w

**Location**: `internal/aggregator/aggregator.go:144`

`fmt.Errorf` with `%w` verb wraps the original error, preserving the error chain. This allows callers to use `errors.Is()` or `errors.Unwrap()` to inspect the chain. We add context ("failed to refresh token for system X") while preserving the original error for debugging.

```go
return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
```

### Error Inspection

**Location**: `internal/aggregator/aggregator.go:157-175`

We check the error type to determine how to handle it. For rate-limit (429) errors we cool the throttled credential down and fail over to a spare from the pool, retrying the same system; only when every credential is exhausted is the system recorded as rate-limited (after the loop) and skipped. For context cancellation errors (Ctrl+C or deadline exceeded), we return immediately to abort all systems. For other errors, we warn and skip the system.

```go
lm, cu, err := cloudClient.GetMetricsFromCloud(ctx, testDate, queryMode)
if err != nil && constants.IsRateLimitError(err) {
    // Throttled: cool this credential down and fail over to a spare.
    pool.MarkUnavailable(cred)
    if next, ok := pool.Failover(tried); ok {
        cred = next
        continue
    }
    rateLimited = true
    break
}
if err != nil {
    if isContextError(ctx, err) {
        return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
    }
    fmt.Fprintf(os.Stderr, "WARNING: [%s] Failed to get metrics, skipping: %v\n", sys.Name, err)
    allFromCache = false
    break // localMetrics stays nil → skipped below
}
```

---

## Structs and Pointers

### Constructor Function Pattern

**Location**: `internal/api/client.go:219-232`

Functions starting with "New" are constructors - they create and initialize structs. This is a Go naming convention, not a language feature. We return a pointer `(*EnlightenCloudClient)` because:
1. the return type specifies a pointer type;
2. we have defined methods that will require this pointer receiver;
3. it allows a nil check if needed; and
4. the struct is moderately sized, so passing a pointer to the struct would be more efficient.

```go
func NewEnlightenCloudClient(...) *EnlightenCloudClient {
    return &EnlightenCloudClient{...}
}
```

### Struct Literal with Pointer

**Location**: `internal/api/client.go:222-231`

`&EnlightenCloudClient{...}` creates a struct and returns a pointer to it. This is idiomatic Go - create struct, take address, return pointer.

```go
return &EnlightenCloudClient{
    systemID: systemID,
    // ...
}
```

### Struct Initialization with Pointer Return

**Location**: `internal/aggregator/aggregator.go:104-109`

We use `&AggregatedMetrics{}` to create a pointer to a new struct. This is more efficient than returning by value (avoids copying large struct).

```go
metrics := &AggregatedMetrics{
    Timestamp: time.Now(),
    // ...
}
```

### Nested Struct Initialization

**Location**: `internal/api/client.go:228-230`

We initialize `httpClient` field with a struct literal. `http.Client` is from standard library - we set Timeout for safety.

```go
httpClient: &http.Client{
    Timeout: constants.APIRequestTimeout,
}
```

### Pointer Receiver Method

**Location**: `internal/config/config.go:152`

`(c *ColorConfig)` means this is a method on `ColorConfig` with a pointer receiver. We use a pointer receiver because:
1. We modify the struct (set ANSI codes in place)
2. Avoids copying the struct (more efficient)
3. Changes are visible to the caller

If we used `(c ColorConfig)` (value receiver), modifications would only affect the copy.

```go
func (c *ColorConfig) convertHexFields() {
    // Foreground fields → 256-color cube via convertIfHex
    foregroundFields := []*string{
        &c.Production, &c.Discharge, &c.Import, &c.Export,
        &c.NetImport, &c.NetExport,
        // ...remaining foreground fields...
    }
    for _, field := range foregroundFields {
        *field = convertIfHex(*field)
    }

    // Background fields → 24-bit truecolor via convertIfHexBackground
    backgroundFields := []*string{
        &c.NetImportBackground, &c.NetExportBackground,
    }
    for _, field := range backgroundFields {
        *field = convertIfHexBackground(*field)
    }
}
```

### Struct Definition

**Location**: `internal/oauth/oauth.go:94-98`

Structs group related data together. This struct holds token information. Fields are exported (PascalCase) so they can be accessed from other packages.

```go
type TokenCache struct {
    Token        string
    RefreshToken string
    ExpiresAt    time.Time
}
```

### Struct Design Principles

**Location**: `internal/api/types.go:11-20`

This struct follows Go best practices:
1. Grouped related fields together (energy metrics, battery status)
2. Descriptive field names (`ProductionToday`, not `Prod`)
3. Comments explain units and meaning (kWh, Wh, percentage)
4. Mix of types appropriate to data (float64 for energy, int for percentage)

### Field Types

**Location**: `internal/api/types.go:12-19`

- `time.Time`: Go's standard time type (always has a value, cannot be nil)
- `float64`: 64-bit floating point (precise enough for energy values)
- `int`: Integer (appropriate for percentage 0-100)

---

## Slices and Arrays

### Slice Declaration

**Location**: `internal/parser/parser.go:98`

`make([]Type, 0, capacity)` creates a slice with zero length but pre-allocated capacity. We pre-count the total elements first, then allocate exactly the right capacity before appending.

```go
allIntervals := make([]TelemetryInterval, 0, total)
```

### Slice Capacity Hint

**Location**: `internal/aggregator/aggregator.go:108`

`make([]Type, length, capacity)` pre-allocates capacity to avoid reallocation. We know we will have `len(systems)` elements, so we pre-allocate that capacity. This is more efficient than letting the slice grow dynamically.

```go
Systems: make([]SystemMetrics, 0, len(systems)),
//                    ^        ^    ^
//                    |        |    └─ Capacity: can hold len(systems) without reallocating
//                    |        └─ Length: currently 0 elements
//                    └─ Type: slice of SystemMetrics
```

### Variadic Append

**Location**: `internal/parser/parser.go:100`

`append(slice, elements...)` can take multiple elements. `intervalArray...` spreads the slice into individual elements. This is equivalent to: `append(allIntervals, intervalArray[0], intervalArray[1], ...)`

```go
allIntervals = append(allIntervals, intervalArray...)
```

### Slice Append

**Location**: `internal/aggregator/aggregator.go:182`

`append()` adds elements to a slice, automatically growing if needed. Since we pre-allocated capacity, this should be efficient.

```go
rateLimitErrors = append(rateLimitErrors, sys.Name)
```

### Array to Slice Conversion

**Location**: `internal/cache/cache.go:175-176`

`hash[:]` converts the `[32]byte` array to a `[]byte` slice. This is necessary because `hex.EncodeToString` expects `[]byte`, not `[32]byte`. The `[:]` syntax creates a slice that views the entire array.

```go
hash := sha256.Sum256([]byte(normalizedURL))
return hex.EncodeToString(hash[:])
```

---

## Control Flow

### Range Loop

**Location**: `internal/parser/parser.go:99-101`

`for _, intervalArray := range data.Intervals` iterates over the slice. The `_` discards the index (we do not need it). `intervalArray` is each nested array in the array of arrays.

```go
for _, intervalArray := range data.Intervals {
    allIntervals = append(allIntervals, intervalArray...)
}
```

### Range Loop Over Slice

**Location**: `internal/parser/parser.go:138`

`for _, interval := range intervals` iterates over each element. `_` discards the index (we do not need it). The body uses a `switch` to select the correct field (see Switch Statement below).

```go
for _, interval := range intervals {
    switch fieldName {
    case constants.FieldWhImported:
        total += interval.WhImported
    // ...
    }
}
```

### Switch Statement

**Location**: `internal/parser/parser.go:139-148`

`switch` is like `if/else` but cleaner for multiple conditions. It is idiomatic Go for handling multiple cases based on a single value. `switch` compares `fieldName` against each case and executes the matching one. This is more readable than multiple `if/else if` statements.

```go
switch fieldName {
case constants.FieldWhImported:
    total += interval.WhImported
case constants.FieldWhExported:
    total += interval.WhExported
}
```

### Continue Statement

**Location**: `internal/aggregator/aggregator.go:181-185`

`continue` skips to next iteration of the loop. We use it here to skip this system and move on to the next once a system has been recorded as rate-limited (every credential for it was exhausted).

```go
if rateLimited {
    rateLimitErrors = append(rateLimitErrors, sys.Name)
    allFromCache = false
    continue
}
```

### Zero Value Pattern

**Location**: `internal/app/setup.go:79-84`

Using `time.Time` (not `*time.Time`) with `.IsZero()` is the idiomatic Go approach. Zero value (`time.Time{}`) means "not set" (use today). Non-zero value means "use this specific date".

```go
func ParseTestDate(dateStr string, reportTZ *time.Location) (ParseDateInput, error) {
    if dateStr == "" {
        return ParseDateInput{
            Date:      time.Time{},  // Returns zero value
            QueryMode: constants.QueryModeDay,
        }, nil
    }
    // ... parse and return non-zero time with query mode
}
```

### Zero Value Initialization

**Location**: `internal/parser/parser.go:137`

`var total float64` initializes `total` to `0.0` (zero value for float64). Go's zero values mean we do not need explicit initialization for most types.

```go
var total float64
```

---

## Interfaces

> **📚 Advanced Topic:** For how interfaces can break circular dependencies (duck typing), see [Why Interfaces Can Avoid Shared Types](#why-interfaces-can-avoid-shared-types) in the File Organization Patterns section.

### What Is an Interface?

An **interface** is a contract that defines **behavior** (methods), not data. Think of it like a job posting:

> "We need someone who can `Read()` and `Close()`."

Any type that has those methods can apply for the job. The interface does not care:
- What the type is called
- What fields it has
- How it implements the methods internally

It only cares: **"Can you do these things?"**

### The Mental Model

```
┌───────────────────────────────────────────────────────────────┐
│  STRUCT (concrete type)    vs    INTERFACE (behavior type)    │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  What do you HAVE?              What can you DO?              │
│  ────────────────               ──────────────                │
│  - Fields (data)                - Methods only                │
│  - Methods                      - No fields                   │
│  - Implementation               - No implementation           │
│                                                               │
│  type File struct {             type Reader interface {       │
│      name string                    Read([]byte) (int, error) │
│      data []byte                }                             │
│  }                                                            │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

### Implicit Satisfaction (No "implements" Keyword)

In Go, you do not declare that a type implements an interface. If the type has the right methods, it automatically satisfies the interface:

```go
// Define an interface
type Writer interface {
    Write([]byte) (int, error)
}

// Define a struct - NO "implements Writer" needed!
type FileWriter struct {
    file *os.File
}

// Just implement the method
func (fw *FileWriter) Write(data []byte) (int, error) {
    return fw.file.Write(data)
}

// FileWriter now automatically satisfies Writer!
```

This is different from Java/C#/TypeScript where you must explicitly declare:
```java
// Java - explicit implements
class FileWriter implements Writer { ... }
```

### Why This Matters: Compile-Time Safety

The key insight is that Go checks interface satisfaction at **compile time**, not runtime:

```
Go:     "Prove you can do X before I let you in."
Ruby:   "I trust you can do X, we'll see at runtime."
```

| Aspect | Go Interface | Ruby Duck Typing |
|--------|--------------|------------------|
| Check happens | Compile time | Runtime |
| Missing method | Won't compile | Runtime error |
| IDE support | Full autocomplete | Limited |
| Refactoring | Compiler catches breaks | Tests must catch breaks |

### Interface Types in This Codebase

**Location**: `internal/parser/parser.go:122`

`io.ReadCloser` is an interface that combines `io.Reader` and `io.Closer`. `http.Response.Body` satisfies this interface, so we can pass it here:

```go
func ReadResponseBody(respBody io.ReadCloser) ([]byte, error) {
    // ...
}
```

**Location**: `internal/parser/parser.go:123`

`io.ReadAll` accepts any `io.Reader` (interface type). `respBody` satisfies `io.Reader`, so we can pass it directly. This is the power of Go interfaces - code works with any type that has a `Read()` method:

```go
bodyBytes, err := io.ReadAll(respBody)
```

### The io.Writer Pattern for Testable I/O

**Location**: `internal/validation/validation.go`

A common pattern in Go is accepting `io.Writer` instead of writing directly to `os.Stdout`. This makes code testable:

```go
// Production code passes os.Stdout
func ValidateMetrics(w io.Writer, metrics *AggregatedMetrics, date string) error {
    fmt.Fprintf(w, "Validating %s...\n", date)
    // ...
}

// Test code passes a buffer to capture output
func TestValidateMetrics(t *testing.T) {
    var buf bytes.Buffer
    err := ValidateMetrics(&buf, metrics, "2026-01-20")

    // Now we can assert on the output!
    if !strings.Contains(buf.String(), "ALL VALIDATIONS PASSED") {
        t.Error("Expected success message")
    }
}
```

This works because:
- `os.Stdout` (type `*os.File`) has a `Write()` method → satisfies `io.Writer`
- `bytes.Buffer` has a `Write()` method → satisfies `io.Writer`
- The function works with both, and neither knows about the other!

### Summary: Interface Is Always a Type

In Go, an interface is always a type - you cannot have an interface that is "something else." Unlike some languages where "interface" might mean different things in different contexts, Go's interface has exactly one meaning:

> **A type defined by a set of method signatures.**

You can use interfaces as:
- Function parameter types: `func Process(r io.Reader)`
- Function return types: `func NewReader() io.Reader`
- Struct field types: `type Config struct { Logger Logger }`
- Variable types: `var w io.Writer = os.Stdout`

---

## JSON Handling

### JSON Unmarshaling

**Location**: `internal/parser/parser.go:84-90`

`json.Unmarshal` converts JSON bytes into Go structs. We pass `&data` (pointer to struct) so `Unmarshal` can modify it. The struct fields must have json tags matching the JSON field names. If unmarshaling fails, it returns an error (we wrap it with context using `%w`).

```go
var data TelemetryResponseNested
if err := json.Unmarshal(bodyBytes, &data); err != nil {
    bodyPreview := string(bodyBytes)
    if len(bodyPreview) > constants.ResponseBodyPreviewLength {
        bodyPreview = bodyPreview[:constants.ResponseBodyPreviewLength] + "..."
    }
    return nil, fmt.Errorf("failed to decode nested telemetry response (body preview: %s): %w", bodyPreview, err)
}
```

---

## Time and Duration

### Duration Literals

**Location**: `internal/api/client.go:228-230`

`time.Second` is a typed constant (`time.Duration`). Multiplying an integer by `time.Second` — e.g. `30 * time.Second` — is idiomatic Go for expressing durations. In this codebase the value is extracted to `constants.APIRequestTimeout` for clarity.

```go
httpClient: &http.Client{
    Timeout: constants.APIRequestTimeout,
}
```

### Time Ticker

`time.NewTicker` creates a ticker that fires a value on its channel at regular intervals. This pattern — combined with `select` — was used in the now-removed `RunContinuous` loop to allow the program to wait for the next tick while also responding instantly to Ctrl+C (SIGINT/SIGTERM).

```go
ticker := time.NewTicker(interval)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        // do periodic work
    case <-ctx.Done():
        return nil  // graceful shutdown; defer ticker.Stop() runs
    }
}
```

`defer ticker.Stop()` ensures the ticker is always cleaned up, even on early return.

---

## Channels and Signals

> **Note:** The `for`/`select` ticker loop that drove periodic re-fetches (`RunContinuous`) has been removed. The concepts below are standard Go — they remain here as a reference for when you encounter or write similar patterns elsewhere.

### Signal Context

`main.go` creates a signal context so that SIGINT (Ctrl+C) and SIGTERM cancel in-flight work:

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

When a signal is received, `ctx.Done()` is closed and any code holding this context (e.g. HTTP requests) is cancelled.

### The `select` Statement

`select` blocks until one of its channel cases is ready, then executes that case. If multiple cases are ready, one is chosen at random (fairness):

```go
select {
case <-ticker.C:
    // timer fired
case <-ctx.Done():
    // signal received
}
```

This gives interruptible waiting at zero CPU cost — the program sleeps but listens for multiple wake-up events simultaneously. `time.Sleep()` cannot do this; it is deaf to signals.

### Channel Types

- **`<-chan struct{}`** (context done): closed (not sent to) when the context is cancelled
- **`<-chan time.Time`** (ticker): receives a `time.Time` value at each interval

### Key Takeaways

1. `signal.NotifyContext` integrates OS signals with Go's context system
2. `select` enables waiting on multiple channels without goroutines
3. `defer` guarantees cleanup (e.g. `ticker.Stop()`) even on early return



---

## Package-Level Variables
