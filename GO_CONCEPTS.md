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

---

## Error Handling

### Error Handling Pattern

**Location**: `aggregator.go:297`

Standard Go pattern: function returns `(result, error)`. We check `err` immediately and return early if non-nil. This is idiomatic Go - errors are values, not exceptions.

```go
accessToken, err := GetAccessToken(ctx, apiConfig)
if err != nil {
    return nil, err  // Handle or propagate
}
```

### Error Handling with Early Return

**Location**: `response_parser.go:19`

We check `err` immediately and return early if non-nil. This is idiomatic Go - handle errors as soon as they occur.

```go
if err := json.Unmarshal(bodyBytes, &data); err != nil {
    return nil, fmt.Errorf("failed to decode: %w", err)
}
```

### Error Wrapping with %w

**Location**: `aggregator.go:307`

`fmt.Errorf` with `%w` verb wraps the original error, preserving the error chain. This allows callers to use `errors.Is()` or `errors.Unwrap()` to inspect the chain. We add context ("failed to get OAuth access token for system X") while preserving the original error for debugging.

```go
return nil, fmt.Errorf("failed to get OAuth access token for system %s: %w", sys.Name, err)
```

### Error Inspection

**Location**: `aggregator.go:326`

We check the error message to determine error type. For rate limit errors, we collect them and continue (do not fail immediately). This allows us to query other systems even if one hits rate limit.

```go
if strings.Contains(err.Error(), "rate limit exceeded (429)") {
    // Handle rate limit error
    continue
}
```

---

## Structs and Pointers

### Constructor Function Pattern

**Location**: `cloud_client.go:131`

Functions starting with "New" are constructors - they create and initialize structs. This is a Go convention (not a language feature, just a naming pattern). We return a pointer `(*EnlightenCloudClient)` because:
1. The struct is moderately sized (more efficient to pass pointer)
2. Methods use pointer receivers (consistent with return type)
3. Allows nil checks if needed

```go
func NewEnlightenCloudClient(...) *EnlightenCloudClient {
    return &EnlightenCloudClient{...}
}
```

### Struct Literal with Pointer

**Location**: `cloud_client.go:139`

`&EnlightenCloudClient{...}` creates a struct and returns a pointer to it. This is idiomatic Go - create struct, take address, return pointer.

```go
return &EnlightenCloudClient{
    systemID: systemID,
    // ...
}
```

### Struct Initialization with Pointer Return

**Location**: `aggregator.go:265`

We use `&AggregatedMetrics{}` to create a pointer to a new struct. This is more efficient than returning by value (avoids copying large struct).

```go
metrics := &AggregatedMetrics{
    Timestamp: time.Now(),
    // ...
}
```

### Nested Struct Initialization

**Location**: `cloud_client.go:147`

We initialize `httpClient` field with a struct literal. `http.Client` is from standard library - we set Timeout for safety.

```go
httpClient: &http.Client{
    Timeout: 30 * time.Second,
}
```

### Pointer Receiver Method

**Location**: `config.go:137`

`(c *ColorConfig)` means this is a method on `ColorConfig` with a pointer receiver. We use a pointer receiver because:
1. We modify the struct (set ANSI codes in place)
2. Avoids copying the struct (more efficient)
3. Changes are visible to the caller

If we used `(c ColorConfig)` (value receiver), modifications would only affect the copy.

```go
func (c *ColorConfig) convertHexFields() {
    c.Production = convertIfHex(c.Production)
    // ...
}
```

### Struct Definition

**Location**: `oauth.go:67`

Structs group related data together. This struct holds token information. Fields are exported (PascalCase) so they can be accessed from other packages.

```go
type TokenCache struct {
    Token        string
    RefreshToken string
    ExpiresAt    time.Time
}
```

### Struct Design Principles

**Location**: `cloud_client.go:188`

This struct follows Go best practices:
1. Grouped related fields together (energy metrics, battery status)
2. Descriptive field names (`ProductionToday`, not `Prod`)
3. Comments explain units and meaning (kWh, Wh, percentage)
4. Mix of types appropriate to data (float64 for energy, int for percentage)

### Field Types

**Location**: `cloud_client.go:195`

- `time.Time`: Go's standard time type (always has a value, cannot be nil)
- `float64`: 64-bit floating point (precise enough for energy values)
- `int`: Integer (appropriate for percentage 0-100)

---

## Slices and Arrays

### Slice Declaration

**Location**: `response_parser.go:31`

`var name []Type` declares a nil slice (zero value for slices). We will append to it to build the flattened array.

```go
var allIntervals []TelemetryInterval
```

### Slice Capacity Hint

**Location**: `aggregator.go:271`

`make([]Type, length, capacity)` pre-allocates capacity to avoid reallocation. We know we will have `len(systems)` elements, so we pre-allocate that capacity. This is more efficient than letting the slice grow dynamically.

```go
Systems: make([]SystemMetrics, 0, len(systems)),
//                    ^        ^    ^
//                    |        |    └─ Capacity: can hold len(systems) without reallocating
//                    |        └─ Length: currently 0 elements
//                    └─ Type: slice of SystemMetrics
```

### Variadic Append

**Location**: `response_parser.go:40`

`append(slice, elements...)` can take multiple elements. `intervalArray...` spreads the slice into individual elements. This is equivalent to: `append(allIntervals, intervalArray[0], intervalArray[1], ...)`

```go
allIntervals = append(allIntervals, intervalArray...)
```

### Slice Append

**Location**: `aggregator.go:332`

`append()` adds elements to a slice, automatically growing if needed. Since we pre-allocated capacity, this should be efficient.

```go
rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
```

### Array to Slice Conversion

**Location**: `api_cache.go:97`

`hash[:]` converts the `[32]byte` array to a `[]byte` slice. This is necessary because `hex.EncodeToString` expects `[]byte`, not `[32]byte`. The `[:]` syntax creates a slice that views the entire array.

```go
hash := sha256.Sum256([]byte(normalizedURL))
return hex.EncodeToString(hash[:])
```

---

## Control Flow

### Range Loop

**Location**: `response_parser.go:35`

`for _, intervalArray := range data.Intervals` iterates over the slice. The `_` discards the index (we do not need it). `intervalArray` is each nested array in the array of arrays.

```go
for _, intervalArray := range data.Intervals {
    allIntervals = append(allIntervals, intervalArray...)
}
```

### Range Loop Over Slice

**Location**: `response_parser.go:98`

`for _, interval := range intervals` iterates over each element. `_` discards the index (we do not need it).

```go
for _, interval := range intervals {
    total += interval.WhImported
}
```

### Switch Statement

**Location**: `response_parser.go:90, 102`

`switch` is like `if/else` but cleaner for multiple conditions. It is idiomatic Go for handling multiple cases based on a single value. `switch` compares `fieldName` against each case and executes the matching one. This is more readable than multiple `if/else if` statements.

```go
switch fieldName {
case "wh_imported":
    total += interval.WhImported
case "wh_exported":
    total += interval.WhExported
}
```

### Continue Statement

**Location**: `aggregator.go:336`

`continue` skips to next iteration of the loop. We use it here to skip this system and try the next one.

```go
if strings.Contains(err.Error(), "rate limit exceeded (429)") {
    rateLimitErrors = append(rateLimitErrors, ...)
    continue  // Skip to next system
}
```

### Zero Value Pattern

**Location**: `main.go:257`

Using `time.Time` (not `*time.Time`) with `.IsZero()` is the idiomatic Go approach. Zero value (`time.Time{}`) means "not set" (use today). Non-zero value means "use this specific date".

```go
var testDateParsed time.Time
if *testDate != "" {
    testDateParsed = parsed
}
// else: testDateParsed remains zero value (today)
```

### Zero Value Initialization

**Location**: `response_parser.go:94`

`var total float64` initializes `total` to `0.0` (zero value for float64). Go's zero values mean we do not need explicit initialization for most types.

```go
var total float64
```

---

## Interfaces

### Interface Types

**Location**: `response_parser.go:67`

`io.ReadCloser` is an interface that combines `io.Reader` and `io.Closer`. `http.Response.Body` satisfies this interface, so we can pass it here. Interfaces in Go are satisfied implicitly - no "implements" keyword needed.

```go
func readResponseBody(respBody io.ReadCloser) ([]byte, error) {
    // ...
}
```

### Interface Usage

**Location**: `response_parser.go:72`

`io.ReadAll` accepts any `io.Reader` (interface type). `respBody` satisfies `io.Reader`, so we can pass it directly. This is the power of Go interfaces - code works with any type that has `Read()` method.

```go
bodyBytes, err := io.ReadAll(respBody)
```

---

## JSON Handling

### JSON Unmarshaling

**Location**: `response_parser.go:12`

`json.Unmarshal` converts JSON bytes into Go structs. We pass `&data` (pointer to struct) so `Unmarshal` can modify it. The struct fields must have json tags matching the JSON field names. If unmarshaling fails, it returns an error (we wrap it with context using `%w`).

```go
var data TelemetryResponseNested
if err := json.Unmarshal(bodyBytes, &data); err != nil {
    return nil, fmt.Errorf("failed to decode: %w", err)
}
```

---

## Time and Duration

### Duration Literals

**Location**: `cloud_client.go:151`

`time.Second` is a constant. `30 * time.Second` converts seconds to `time.Duration`. This is idiomatic Go for time durations.

```go
Timeout: 30 * time.Second
```

### Time Ticker

**Location**: `main.go:315`

`time.NewTicker` creates a ticker that sends a value on its channel at regular intervals. `time.Duration(config.RefreshInterval) * time.Second` converts seconds to Duration. We use `defer` to ensure the ticker is stopped when the function returns.

```go
ticker := time.NewTicker(time.Duration(config.RefreshInterval) * time.Second)
defer ticker.Stop()
```

---

## Channels and Signals

This section explains how the program uses Go channels and Unix signals to implement graceful shutdown and continuous monitoring. These are key concepts that all Go developers should learn.

> **📚 Prerequisites**: This guide assumes familiarity with basic Go concepts. If you are new to Go, start with **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** to understand channels, goroutines, and the `select` statement.

### Overview

The program uses channels and signals **only in continuous monitoring mode** (`./enphase-monitor` without `--once` flag). This allows the application to:
1. Run periodic updates at a configurable interval
2. Respond to user interrupts (Ctrl+C) gracefully
3. Clean up resources before exiting

### The Problem

When running in continuous mode, the program needs to:
- Execute periodic work (fetch metrics every N seconds)
- Listen for user interrupts (Ctrl+C) simultaneously
- Clean up resources (stop timers) before exiting

**Without channels/signals:** The program would need to poll for interrupts, which is inefficient and does not allow immediate response.

**With channels/signals:** The program can wait on both events simultaneously using Go's `select` statement.

---

### How It Works

#### Step 1 & 2: Create Context with Signal Handling

```go
// main.go, line 241
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

**What this does:**
- Creates a context that is automatically cancelled when SIGINT or SIGTERM is received
- SIGINT: Sent when user presses Ctrl+C
- SIGTERM: Sent by system when process is terminated (e.g., `kill` command)
- `stop()` is a cleanup function that should be called when done (typically via defer)

**Why use NotifyContext?**
- Simpler API than manually creating signal channels
- Integrates signal handling directly with context cancellation
- Automatically cancels in-flight HTTP requests when signal is received
- Follows idiomatic Go 1.16+ pattern for signal handling

**How it works:**
- The Go runtime sets up a signal handler
- When SIGINT or SIGTERM is received, `ctx.Done()` channel is closed
- Any code using this context (like HTTP requests) is automatically cancelled
- This enables cascading cancellation throughout the application

#### Step 3: Create Timer Ticker

```go
// main.go, line 319
ticker := time.NewTicker(time.Duration(config.RefreshInterval) * time.Second)
defer ticker.Stop()
```

**What this does:**
- Creates a ticker that sends a value on `ticker.C` channel at regular intervals
- `ticker.C` is a channel that receives a `time.Time` value each interval
- `defer ticker.Stop()` ensures the ticker is stopped when function returns

**Example:**
- If `refresh_interval` is 3600 (1 hour), ticker fires every hour
- Each time it fires, `ticker.C` receives a value

#### Step 4: The Main Loop with Select

```go
// main.go, lines 329-340
for {
    select {
    case <-ticker.C:
        // Timer ticked - do periodic work
        fetchAndDisplay(ctx, aggregator, display, config, testDate, reportTZ)

    case <-ctx.Done():
        // Context cancelled when signal received - handle shutdown
        display.ShowInfo("Shutting down gracefully...")
        return
    }
}
```

**What this does:**
- `for {}` creates an infinite loop
- `select` waits on multiple channel operations
- Blocks until one of the cases is ready
- Executes the first case that is ready

**Flow:**
1. Program enters `select` and blocks
2. Waits for either:
   - `ticker.C` to receive a value (timer ticked)
   - `ctx.Done()` to close (signal received)
3. When one happens, executes that case
4. Loops back to `select` and waits again

---

### Detailed Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. Program Starts (main.go)                                     │
│    └─► Create context with signal handling:                     │
│        ctx, stop := signal.NotifyContext(...)                   │
│        defer stop()                                              │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Signal Handler Registered                                │
│    └─► NotifyContext sets up OS signal handler              │
│        • Go runtime watches for SIGINT, SIGTERM             │
│        • Handler runs in background goroutine               │
│        • Cancels ctx when signal received                   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Enter Continuous Mode (runContinuous)                    │
│    └─► Create ticker: ticker = time.NewTicker(interval)     │
│        • ticker.C is a channel that fires every interval    │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Main Loop Starts                                         │
│    └─► for { select { ... } }                               │
│        • Blocks waiting for ticker.C or ctx.Done()         │
└─────────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        │                                       │
        ▼                                       ▼
┌───────────────────────┐          ┌─────────────────────────┐
│ Case 1: Timer Ticked  │          │ Case 2: Signal Received │
│ <-ticker.C            │          │ <-ctx.Done()            │
│                       │          │                         │
│ • Fetch metrics       │          │ • Print shutdown msg    │
│ • Display results     │          │ • return (exits loop)  │
│ • Loop continues      │          │ • defer ticker.Stop()  │
│                       │          │   executes              │
└───────────────────────┘          └─────────────────────────┘
        │                                       │
        └───────────────────┬───────────────────┘
                            │
                            ▼
                    [Loop continues or exits]
```

---

### Why This Pattern?

#### 1. Non-Blocking Wait

**Without select:**
```go
// ❌ BAD: Cannot wait on both simultaneously
for {
    // Check timer (but this blocks!)
    time.Sleep(interval)
    fetchAndDisplay(...)
    
    // Cannot check for signals while sleeping!
}
```

**With select:**
```go
// ✅ GOOD: Waits on both, responds to whichever happens first
select {
case <-ticker.C:  // Timer ticked
case <-ctx.Done(): // Signal received
}
```

#### 2. Immediate Response

When user presses Ctrl+C:
- Signal is sent to context immediately
- `ctx.Done()` channel is closed
- `select` unblocks and executes the signal case
- Program exits gracefully within milliseconds

Without this pattern, the program might wait until the next timer tick before checking for signals.

#### 3. Resource Cleanup

The `defer ticker.Stop()` ensures:
- Ticker is stopped when function returns
- Happens even if we return early due to signal
- Prevents resource leaks (ticker goroutine continues running)

---

### Signal Types

#### SIGINT (Interrupt Signal)
- **Triggered by:** User pressing Ctrl+C in terminal
- **Default behavior:** Terminates process immediately
- **Our behavior:** Graceful shutdown (cleanup, then exit)

#### SIGTERM (Termination Signal)
- **Triggered by:** System shutdown, `kill` command, process managers
- **Default behavior:** Terminates process
- **Our behavior:** Same graceful shutdown as SIGINT

#### Why Handle Both?

- **SIGINT:** User-initiated (Ctrl+C)
- **SIGTERM:** System-initiated (shutdown, process managers)
- Both should trigger graceful shutdown

---

### Channel Types Used

#### 1. Context Done Channel (`<-chan struct{}`)

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
// ctx.Done() is of type <-chan struct{} (receive-only)
```

- **Type:** `struct{}` (empty struct, zero memory)
- **Direction:** Receive-only (`<-chan`)
- **Purpose:** Receive cancellation signal when SIGINT or SIGTERM is received
- **Behavior:** Channel is closed (not sent a value) when signal is received

#### 2. Ticker Channel (`<-chan time.Time`)

```go
ticker := time.NewTicker(interval)
// ticker.C is of type <-chan time.Time (receive-only)
```

- **Type:** `time.Time` (timestamp of when tick occurred)
- **Direction:** Receive-only (`<-chan`)
- **Buffer:** Internal to ticker implementation
- **Purpose:** Receive periodic timer events

---

### Select Statement Deep Dive

#### How Select Works

```go
select {
case <-ticker.C:
    // This case executes when ticker.C has a value
case <-ctx.Done():
    // This case executes when ctx.Done() is closed
}
```

**Behavior:**
1. **Blocks** until at least one case is ready
2. If **multiple cases** are ready, **randomly selects one** (fairness)
3. If **no cases** are ready, blocks indefinitely
4. **Non-blocking** variant exists (`default` case), but we do not use it here

#### Why Random Selection?

If both `ticker.C` and `ctx.Done()` are ready:
- Go randomly selects one (prevents starvation)
- In practice, this rarely happens (signals are rare events)
- If it does happen, either case is acceptable

---

### Real-World Example

#### Scenario: User Runs Continuous Mode

```bash
$ ./enphase-monitor
Starting continuous monitoring (refresh every 3600 seconds)
Press Ctrl+C to stop
```

**Timeline:**
1. **T=0s:** Program starts, creates context and ticker, enters loop
2. **T=0s:** Immediately fetches and displays metrics (first run)
3. **T=0s:** Enters `select`, blocks waiting for ticker or signal
4. **T=3600s:** Ticker fires, `ticker.C` receives value
5. **T=3600s:** `select` executes ticker case, fetches metrics
6. **T=3600s:** Loops back to `select`, blocks again
7. **T=7200s:** Ticker fires again, process repeats
8. **T=5000s (user presses Ctrl+C):** SIGINT sent, `ctx.Done()` closes
9. **T=5000s:** `select` executes signal case immediately
10. **T=5000s:** Prints "Shutting down gracefully..."
11. **T=5000s:** `return` exits function
12. **T=5000s:** `defer ticker.Stop()` executes, cleans up
13. **T=5000s:** Program exits

**Key Point:** The program responds to Ctrl+C immediately, even if it is in the middle of waiting for the next timer tick.

---

### Edge Cases Handled

#### 1. Signal Arrives Before Select

**Problem:** What if signal arrives before we enter the `select`?

**Solution:** Context cancellation is handled by the Go runtime
- Signal is handled by `signal.NotifyContext`
- When we check `ctx.Done()`, it is already closed if signal was received
- No signal is lost

#### 2. Multiple Signals

**Problem:** What if user presses Ctrl+C multiple times?

**Solution:** We only handle the first signal
- After first signal, we `return` and exit
- Subsequent signals are ignored (process is shutting down)
- This is the desired behavior

#### 3. Ticker Fires During Shutdown

**Problem:** What if ticker fires while we are shutting down?

**Solution:** We have already returned
- Once we receive signal and `return`, we are out of the loop
- Ticker case will not execute
- `defer ticker.Stop()` stops the ticker anyway

---

### Comparison: With vs Without Channels

#### Without Channels (Polling - BAD)

```go
// ❌ Inefficient polling approach
for {
    // Do work
    fetchAndDisplay(...)
    
    // Poll for interrupt (inefficient!)
    for i := 0; i < refreshInterval; i++ {
        time.Sleep(1 * time.Second)
        // Check if we should exit (but how?)
        // Cannot easily check for signals here
    }
}
```

**Problems:**
- Wastes CPU checking for signals
- Delayed response (only checks once per second)
- Complex to implement correctly
- Does not handle system signals well

#### With Channels (Event-Driven - GOOD)

```go
// ✅ Efficient event-driven approach
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

ticker := time.NewTicker(interval)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        fetchAndDisplay(...)
    case <-ctx.Done():
        return  // Immediate response
    }
}
```

**Benefits:**
- No CPU waste (blocks until event occurs)
- Immediate response to signals
- Simple, idiomatic Go code
- Handles all signal types correctly

---

### Key Takeaways

1. **Channels enable concurrent event handling** - Wait on multiple events simultaneously
2. **Signals enable graceful shutdown** - Respond to user/system interrupts properly
3. **Select statement is the key** - Allows waiting on multiple channels
4. **Context cancellation integrates signals** - `signal.NotifyContext` simplifies signal handling
5. **Defer ensures cleanup** - Ticker is always stopped, even on early return

---

### Defer Statement

**Location**: `main.go:320`

`defer` schedules a function call to execute when the surrounding function returns. This ensures cleanup happens even if the function returns early or panics. Here we ensure the ticker is stopped to prevent resource leaks.

```go
defer ticker.Stop()
```

---

## Package-Level Variables

### Package-Level Variable

**Location**: `oauth.go:76`

Variables declared outside functions are package-level (shared across all functions). We use `*TokenCache` (pointer) so it can be `nil` (meaning "no cache yet"). This is a singleton pattern - one cache for the entire application.

```go
var (
    tokenCache *TokenCache  // Shared cache
)
```

---

## Hash Functions

### Hash Functions and Array Slices

**Location**: `api_cache.go:87`

`sha256.Sum256()` returns a `[32]byte` array (fixed size). We convert it to a string using hex encoding for a readable cache key.

```go
hash := sha256.Sum256([]byte(normalizedURL))
return hex.EncodeToString(hash[:])
```

### Hash Function

**Location**: `api_cache.go:94`

`sha256.Sum256()` computes SHA-256 hash, returns `[32]byte` array.

```go
hash := sha256.Sum256([]byte(normalizedURL))
```

---

## Variable Declarations

### Variable Declaration with Type

**Location**: `aggregator.go:282`

`var name []Type` declares a variable with zero value (nil slice for slices). We could use `:= []string{}` but `var` is clearer when we are not initializing.

```go
var rateLimitErrors []string
```

---

## Multiple Return Values

### Multiple Return Values

**Location**: `aggregator.go:319`

Functions can return multiple values: `(result1, result2, error)`. Here we get: metrics, `cacheUsed` flag, and error. The `cacheUsed` flag tells us if cached data was used (important for rate limiting).

```go
localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate)
```

---

## Related Documentation

- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Comprehensive guide to Go patterns and best practices
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design patterns
