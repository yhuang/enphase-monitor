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

**Location**: `internal/aggregator/aggregator.go:105-108`

Standard Go pattern: function returns `(result, error)`. We check `err` immediately and return early if non-nil. This is idiomatic Go - errors are values, not exceptions.

```go
accessToken, err := a.getAccessToken(ctx, apiConfig)
if err != nil {
    return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
}
```

### Error Handling with Early Return

**Location**: `internal/parser/parser.go:85`

We check `err` immediately and return early if non-nil. This is idiomatic Go - handle errors as soon as they occur.

```go
if err := json.Unmarshal(bodyBytes, &data); err != nil {
    return nil, fmt.Errorf("failed to decode: %w", err)
}
```

### Error Wrapping with %w

**Location**: `internal/aggregator/aggregator.go:107`

`fmt.Errorf` with `%w` verb wraps the original error, preserving the error chain. This allows callers to use `errors.Is()` or `errors.Unwrap()` to inspect the chain. We add context ("failed to refresh token for system X") while preserving the original error for debugging.

```go
return nil, fmt.Errorf("%s for system %s: %w", constants.ErrTokenRefreshFailed, sys.Name, err)
```

### Error Inspection

**Location**: `internal/aggregator/aggregator.go:116-128`

We check the error type to determine how to handle it. For rate limit errors, we collect them and continue (do not fail immediately). This allows us to query other systems even if one hits rate limit. For context cancellation errors (Ctrl+C or deadline exceeded), we return immediately to abort all systems. For other errors, we warn and continue.

```go
if err != nil && constants.IsRateLimitError(err) {
    rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
    allFromCache = false
    continue
}
if err != nil {
    if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
        return nil, fmt.Errorf("failed to get metrics from Cloud API for system %s: %w", sys.Name, err)
    }
    fmt.Printf("WARNING: [%s] Failed to get metrics, skipping: %v\n", sys.Name, err)
    allFromCache = false
    continue
}
```

---

## Structs and Pointers

### Constructor Function Pattern

**Location**: `internal/api/client.go:192-205`

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

**Location**: `internal/api/client.go:195-204`

`&EnlightenCloudClient{...}` creates a struct and returns a pointer to it. This is idiomatic Go - create struct, take address, return pointer.

```go
return &EnlightenCloudClient{
    systemID: systemID,
    // ...
}
```

### Struct Initialization with Pointer Return

**Location**: `internal/aggregator/aggregator.go:81-86`

We use `&AggregatedMetrics{}` to create a pointer to a new struct. This is more efficient than returning by value (avoids copying large struct).

```go
metrics := &AggregatedMetrics{
    Timestamp: time.Now(),
    // ...
}
```

### Nested Struct Initialization

**Location**: `internal/api/client.go:201-203`

We initialize `httpClient` field with a struct literal. `http.Client` is from standard library - we set Timeout for safety.

```go
httpClient: &http.Client{
    Timeout: constants.APIRequestTimeout,
}
```

### Pointer Receiver Method

**Location**: `internal/config/config.go:127`

`(c *ColorConfig)` means this is a method on `ColorConfig` with a pointer receiver. We use a pointer receiver because:
1. We modify the struct (set ANSI codes in place)
2. Avoids copying the struct (more efficient)
3. Changes are visible to the caller

If we used `(c ColorConfig)` (value receiver), modifications would only affect the copy.

```go
func (c *ColorConfig) convertHexFields() {
    // Collect pointers to all 12 color fields, then convert in one loop
    fields := []*string{
        &c.Production, &c.Discharge, &c.Import, &c.Export,
        // ...all 12 fields...
    }
    for _, field := range fields {
        *field = convertIfHex(*field)
    }
}
```

### Struct Definition

**Location**: `internal/oauth/oauth.go:85-89`

Structs group related data together. This struct holds token information. Fields are exported (PascalCase) so they can be accessed from other packages.

```go
type TokenCache struct {
    Token        string
    RefreshToken string
    ExpiresAt    time.Time
}
```

### Struct Design Principles

**Location**: `internal/api/types.go:21-30`

This struct follows Go best practices:
1. Grouped related fields together (energy metrics, battery status)
2. Descriptive field names (`ProductionToday`, not `Prod`)
3. Comments explain units and meaning (kWh, Wh, percentage)
4. Mix of types appropriate to data (float64 for energy, int for percentage)

### Field Types

**Location**: `internal/api/types.go:22-29`

- `time.Time`: Go's standard time type (always has a value, cannot be nil)
- `float64`: 64-bit floating point (precise enough for energy values)
- `int`: Integer (appropriate for percentage 0-100)

---

## Slices and Arrays

### Slice Declaration

**Location**: `internal/parser/parser.go:94`

`var name []Type` declares a nil slice (zero value for slices). We will append to it to build the flattened array.

```go
var allIntervals []TelemetryInterval
```

### Slice Capacity Hint

**Location**: `internal/aggregator/aggregator.go:85`

`make([]Type, length, capacity)` pre-allocates capacity to avoid reallocation. We know we will have `len(systems)` elements, so we pre-allocate that capacity. This is more efficient than letting the slice grow dynamically.

```go
Systems: make([]SystemMetrics, 0, len(systems)),
//                    ^        ^    ^
//                    |        |    └─ Capacity: can hold len(systems) without reallocating
//                    |        └─ Length: currently 0 elements
//                    └─ Type: slice of SystemMetrics
```

### Variadic Append

**Location**: `internal/parser/parser.go:96`

`append(slice, elements...)` can take multiple elements. `intervalArray...` spreads the slice into individual elements. This is equivalent to: `append(allIntervals, intervalArray[0], intervalArray[1], ...)`

```go
allIntervals = append(allIntervals, intervalArray...)
```

### Slice Append

**Location**: `internal/aggregator/aggregator.go:117`

`append()` adds elements to a slice, automatically growing if needed. Since we pre-allocated capacity, this should be efficient.

```go
rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
```

### Array to Slice Conversion

**Location**: `internal/cache/cache.go:172-173`

`hash[:]` converts the `[32]byte` array to a `[]byte` slice. This is necessary because `hex.EncodeToString` expects `[]byte`, not `[32]byte`. The `[:]` syntax creates a slice that views the entire array.

```go
hash := sha256.Sum256([]byte(normalizedURL))
return hex.EncodeToString(hash[:])
```

---

## Control Flow

### Range Loop

**Location**: `internal/parser/parser.go:95-97`

`for _, intervalArray := range data.Intervals` iterates over the slice. The `_` discards the index (we do not need it). `intervalArray` is each nested array in the array of arrays.

```go
for _, intervalArray := range data.Intervals {
    allIntervals = append(allIntervals, intervalArray...)
}
```

### Range Loop Over Slice

**Location**: `internal/parser/parser.go:134`

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

**Location**: `internal/parser/parser.go:135-144`

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

**Location**: `internal/aggregator/aggregator.go:116-120`

`continue` skips to next iteration of the loop. We use it here to skip this system and try the next one when a rate limit error occurs.

```go
if err != nil && constants.IsRateLimitError(err) {
    rateLimitErrors = append(rateLimitErrors, fmt.Sprintf("System %s: %v", sys.Name, err))
    allFromCache = false
    continue
}
```

### Zero Value Pattern

**Location**: `internal/app/setup.go:80-86`

Using `time.Time` (not `*time.Time`) with `.IsZero()` is the idiomatic Go approach. Zero value (`time.Time{}`) means "not set" (use today). Non-zero value means "use this specific date".

```go
func ParseTestDate(dateStr string, reportTZ *time.Location) (ParseDateInput, error) {
    if dateStr == "" {
        return ParseDateInput{
            Date:      time.Time{},  // Returns zero value
            QueryType: constants.QueryTypeDay,
        }, nil
    }
    // ... parse and return non-zero time with query type
}
```

### Zero Value Initialization

**Location**: `internal/parser/parser.go:133`

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

**Location**: `internal/parser/parser.go:118`

`io.ReadCloser` is an interface that combines `io.Reader` and `io.Closer`. `http.Response.Body` satisfies this interface, so we can pass it here:

```go
func ReadResponseBody(respBody io.ReadCloser) ([]byte, error) {
    // ...
}
```

**Location**: `internal/parser/parser.go:119`

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
    return nil, fmt.Errorf("failed to decode nested telemetry response (body preview: %s): %w", bodyPreview, err)
}
```

---

## Time and Duration

### Duration Literals

**Location**: `internal/api/client.go:202`

`time.Second` is a typed constant (`time.Duration`). Multiplying an integer by `time.Second` — e.g. `30 * time.Second` — is idiomatic Go for expressing durations. In this codebase the value is extracted to `constants.APIRequestTimeout` for clarity.

```go
httpClient: &http.Client{
    Timeout: constants.APIRequestTimeout,
}
```

### Time Ticker

**Location**: `internal/app/runner.go:67-68`

`time.NewTicker` creates a ticker that sends a value on its channel at regular intervals. `time.Duration(rc.Cfg.RefreshIntervalSeconds) * time.Second` converts seconds to Duration. We use `defer` to ensure the ticker is stopped when the function returns.

```go
ticker := time.NewTicker(time.Duration(rc.Cfg.RefreshIntervalSeconds) * time.Second)
defer ticker.Stop()
```

---

## Channels and Signals

This section explains how the program uses Go channels and Unix signals to implement graceful shutdown and continuous monitoring. These are key concepts that all Go developers should learn.

> **📚 Prerequisites**: This guide assumes familiarity with basic Go concepts. If you are new to Go, start with **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** to understand channels, goroutines, and the `select` statement.

### Overview

The program uses channels and signals **only in continuous monitoring mode** (`./enphase-monitor --continuous`). This allows the application to:
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
// main.go:179-180
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
// internal/app/runner.go:67-68
ticker := time.NewTicker(time.Duration(rc.Cfg.RefreshIntervalSeconds) * time.Second)
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
// internal/app/runner.go:75-88
for {
    select {
    case <-ticker.C:
        // Timer ticked - do periodic work
        if err := fetchAndDisplay(ctx, rc); err != nil {
            return err
        }

    case <-ctx.Done():
        // Context cancelled when signal received - handle shutdown
        rc.Disp.ShowInfo("Shutting down gracefully...")
        return nil
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
┌─────────────────────────────────────────────────────────────┐
│ 1. Program Starts (main.go)                                 │
│    └─► Create context with signal handling:                 │
│        ctx, stop := signal.NotifyContext(...)               │
│        defer stop()                                         │
└─────────────────────────────────────────────────────────────┘
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
│ 3. Enter Continuous Mode (RunContinuous)                    │
│    └─► Create ticker: ticker = time.NewTicker(interval)     │
│        • ticker.C is a channel that fires every interval    │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Main Loop Starts                                         │
│    └─► for { select { ... } }                               │
│        • Blocks waiting for ticker.C or ctx.Done()          │
└─────────────────────────────────────────────────────────────┘
                             │
        ┌────────────────────┴──────────────────┐
        │                                       │
        ▼                                       ▼
┌───────────────────────┐           ┌─────────────────────────┐
│ Case 1: Timer Ticked  │           │ Case 2: Signal Received │
│ <-ticker.C            │           │ <-ctx.Done()            │
│                       │           │                         │
│ • Fetch metrics       │           │ • Print shutdown msg    │
│ • Display results     │           │ • return (exits loop)   │
│ • Loop continues      │           │ • defer ticker.Stop()   │
│                       │           │   executes              │
└───────────────────────┘           └─────────────────────────┘
        │                                        │
        └────────────────────┬───────────────────┘
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

**Location**: `internal/app/runner.go:68`

`defer` schedules a function call to execute when the surrounding function returns. This ensures cleanup happens even if the function returns early or panics. Here we ensure the ticker is stopped to prevent resource leaks.

```go
defer ticker.Stop()
```

---

## Package-Level Variables

### Package-Level Variable

**Location**: `internal/oauth/oauth.go:91`

Variables declared outside functions are package-level (shared across all functions). We use `*TokenCache` (pointer) so it can be `nil` (meaning "no cache yet"). This is a singleton pattern - one cache for the entire application.

```go
var (
    tokenCache *TokenCache  // Shared cache
)
```

---

## Hash Functions

### Hash Functions and Array Slices

**Location**: `internal/cache/cache.go:172-173`

`sha256.Sum256()` computes a SHA-256 hash and returns a `[32]byte` array (fixed size). We convert it to a string using hex encoding for a readable cache key.

```go
hash := sha256.Sum256([]byte(normalizedURL))
return hex.EncodeToString(hash[:])
//                        ^^^^^ Converts [32]byte array to []byte slice
```

**Key Points:**
- `sha256.Sum256()` returns a fixed-size array `[32]byte`, not a slice
- `hash[:]` converts the array to a slice (required by `hex.EncodeToString`)
- The resulting hex string is 64 characters (2 hex chars per byte × 32 bytes)

---

## Variable Declarations

### Variable Declaration with Type

**Location**: `internal/aggregator/aggregator.go:89`

`var name []Type` declares a variable with zero value (nil slice for slices). We could use `:= []string{}` but `var` is clearer when we are not initializing.

```go
var rateLimitErrors []string
```

---

## Multiple Return Values

### Multiple Return Values

**Location**: `internal/aggregator/aggregator.go:115`

Functions can return multiple values: `(result1, result2, error)`. Here we get: metrics, `cacheUsed` flag, and error. The `cacheUsed` flag tells us if cached data was used (important for rate limiting).

```go
localMetrics, cacheUsed, err := cloudClient.GetMetricsFromCloud(ctx, testDate, queryType)
```

---

## File Organization Patterns

### Internal Package Structure

**Location**: `/internal/` directory

Go projects use the `internal/` directory for packages that should not be imported by external code. This is enforced by the Go compiler - code outside the module cannot import packages under `internal/`.

```
internal/
├── aggregator/                  # Multi-system data aggregation
│   ├── aggregator.go            # Core aggregation logic
│   ├── types.go                 # Data types (AggregatedMetrics, SystemMetrics)
│   └── *_test.go                # Tests and benchmarks
├── api/                         # API client for Enphase Cloud API
│   ├── client.go                # HTTP client implementation
│   ├── interface.go             # CloudClient interface definition
│   ├── types.go                 # LocalMetrics type
│   └── *_test.go                # Unit and functional tests
├── app/                         # Application setup and execution
│   ├── setup.go                 # Configuration and initialization
│   ├── runner.go                # Execution modes (once/continuous)
│   └── *_test.go                # Setup and runner tests
├── cache/                       # Disk-based API response caching
│   ├── cache.go                 # Core caching logic
│   ├── cli.go                   # Cache inspection utilities
│   └── *_test.go                # Cache and CLI tests
├── cli/                         # Command-line interface
│   ├── flags.go                 # Flag parsing
│   ├── cache_commands.go        # Cache management commands
│   └── *_test.go                # Flag and command tests
├── config/                      # Configuration loading and validation
│   ├── config.go                # YAML config parsing
│   └── config_test.go           # Configuration tests
├── constants/                   # Application-wide constants
│   ├── constants.go             # All magic numbers and strings
│   └── constants_test.go        # Constants tests
├── display/                     # Terminal output formatting
│   ├── display.go               # Colored output with metrics
│   └── display_test.go          # Display tests
├── oauth/                       # OAuth 2.0 authentication
│   ├── oauth.go                 # Token acquisition/refresh
│   ├── setup.go                 # Interactive setup wizard
│   └── *_test.go                # Unit, functional, and edge case tests
├── parser/                      # JSON response parsing
│   ├── parser.go                # Telemetry data parsing
│   └── *_test.go                # Parser tests and benchmarks
├── timezone/                    # Timezone handling
│   ├── timezone.go              # Day boundaries calculation
│   └── timezone_test.go         # Timezone tests
├── types/                       # Shared type definitions
│   └── types.go                 # Types used across packages
├── urlbuilder/                  # API URL construction
│   ├── urlbuilder.go            # URL building utilities
│   └── urlbuilder_test.go       # URL builder tests
└── validation/                  # Test mode validation
    ├── validation.go            # Metrics comparison (uses io.Writer for testability)
    └── *_test.go                # Unit and integration tests
```

---

### Why Some Packages Have *_test.go Files and Others Do Not

Test files in Go follow the pattern `*_test.go`. The Go tooling automatically excludes these files from production builds.

**Packages WITH Tests:**
- `api/client_test.go` - Tests HTTP client logic
- `config/config_test.go` - Tests YAML parsing and validation
- `constants/constants_test.go` - Tests constant values
- `oauth/oauth_test.go` - Tests token handling
- `parser/parser_test.go` - Tests JSON parsing
- `timezone/timezone_test.go` - Tests timezone calculations
- `validation/validation_test.go` - Tests metrics validation
- `cache/cache_test.go` - Tests state management
- `display/display_test.go` - Tests output formatting
- `aggregator/aggregator_test.go` - Tests data aggregation with mocks

**Why Test Coverage Varies:**

A package typically has tests when:
1. **The functionality is critical** - API client, config parsing, validation
2. **The logic is complex** - OAuth token handling, timezone calculations
3. **The code is easy to unit test** - Pure functions, injectable dependencies

Packages may lack tests when:
- **High coupling to external systems** - Requires mocking infrastructure
- **Primarily orchestration code** - Better suited for integration tests
- **Simple pass-through logic** - Low risk of bugs

---

### Why Types Are in Separate Files

This pattern is called **"Type Separation"** or **"Type Extraction"** in Go:

**Single-Package Type Separation:**

```go
// types.go - Contains data structures
type LocalMetrics struct {
    ProductionToday float64
    // ...
}

// client.go - Contains implementation
func (c *Client) GetMetrics() (*LocalMetrics, error) {
    // ...
}

// interface.go - Contains interface definitions
type CloudClient interface {
    GetMetricsFromCloud(ctx context.Context, date time.Time, queryType constants.QueryType) (*LocalMetrics, bool, error)
}
```

**Cross-Package Type Sharing:**

When multiple packages need the same types, a shared types package prevents circular dependencies.

---

### Package-Specific vs Shared Types

This codebase has TWO different patterns for `types.go` files, and understanding the distinction is important:

**The Problem:**

You might notice there are multiple `types.go` files in this codebase:
- `internal/api/types.go`
- `internal/aggregator/types.go`
- `internal/types/types.go`

Why have `internal/types/types.go` when we already have package-specific `types.go` files?

**The Answer: They Serve Different Purposes**

| Pattern | Location | Purpose | Example Types |
|---------|----------|---------|---------------|
| **Package-specific types** | `internal/api/types.go` | Types used ONLY within that package | `LocalMetrics` |
| **Package-specific types** | `internal/aggregator/types.go` | Types used ONLY within that package | `AggregatedMetrics`, `SystemMetrics` |
| **Shared types package** | `internal/types/types.go` | Types used by MULTIPLE packages | `SystemConfig`, `APIConfig` |

**Why Package-Specific Types Stay in Their Package:**

```go
// internal/api/types.go - Used only by the api package
type LocalMetrics struct {
    ProductionToday float64
    // ... only api/client.go uses this
}

// internal/aggregator/types.go - Used only by the aggregator package
type AggregatedMetrics struct {
    ProductionToday float64
    Systems         []SystemMetrics
    // ... only aggregator package uses this
}
```

These types are implementation details of their respective packages. No other package needs to import them directly.

**Why Some Types Need a Shared Package:**

```go
// The Problem: Circular Dependency
// --------------------------------
// config package defines SystemConfig
// aggregator package needs SystemConfig to know which systems to query
// BUT if aggregator imports config, and config imports aggregator...
// Go compiler error: "import cycle not allowed"

// The Solution: Extract shared types
// ----------------------------------
// internal/types/types.go
type SystemConfig struct {
    Name string
    ID   string
}

// Now both packages can import from internal/types without circular dependency:
//   config     → imports types
//   aggregator → imports types
//   oauth      → imports types
//   app        → imports types
```

**Visual Representation:**

```
BEFORE (circular dependency problem):
    config ←──────→ aggregator
         ↖       ↗
           oauth
    ERROR: import cycle not allowed

AFTER (shared types solution):
                types ─
               /  |     \
              /   |      \
        config aggregator oauth
              \   |      /
               \  |     /
                 app ──
    OK: No circular dependencies
```

**Summary:**

- `internal/api/types.go` → Package-specific types (LocalMetrics for API responses)
- `internal/aggregator/types.go` → Package-specific types (AggregatedMetrics for results)
- `internal/types/types.go` → Shared types (SystemConfig, APIConfig used everywhere)

The key question to ask: "Does more than one package need this type?" If yes, it belongs in `internal/types/`. If no, it stays in the package-specific `types.go`.

---

### When to Move a Type to internal/types/

If a package-specific type later needs to be used by another package, use this decision flowchart:

```
Does package B need a type from package A?
    │
    ▼
Does package A also need something from package B?
    │
    ├── NO  → Just import A from B (direct import works)
    │         Example: display imports aggregator.AggregatedMetrics
    │
    └── YES → Would create circular dependency
              │
              ├── Can you use an interface instead?
              │   └── YES → Define interface in the consuming package
              │
              └── Need the concrete type?
                  └── Move type to internal/types/
```

**Three Possible Solutions:**

| Scenario | Solution | Example in Codebase |
|----------|----------|---------------------|
| B needs A's type, A doesn't need B | Direct import | `display` imports `aggregator.AggregatedMetrics` |
| Mutual dependency, behavior needed | Define interface | `api.CloudClient` interface for mocking |
| Mutual dependency, concrete type needed | Move to `internal/types/` | `SystemConfig`, `APIConfig` |

**Current Type Locations:**

| Type | Location | Reason |
|------|----------|--------|
| `AggregatedMetrics` | `aggregator/types.go` | Only `display` imports it; no cycle |
| `LocalMetrics` | `api/types.go` | Only `aggregator` imports it; no cycle |
| `SystemConfig` | `types/types.go` | `config`, `aggregator`, `oauth`, `app` all need it; would create cycles |
| `APIConfig` | `types/types.go` | `config`, `aggregator`, `oauth`, `app` all need it; would create cycles |

**Why Interfaces Can Avoid Shared Types:**

Interfaces can break circular dependencies because of Go's implicit interface satisfaction (duck typing). If you only need to call methods on a type (not access its fields), you can define an interface in the consuming package:

```go
// Package A defines a concrete type
package api

type EnlightenCloudClient struct {
    // fields...
}

func (c *EnlightenCloudClient) GetMetrics() (*LocalMetrics, error) {
    // implementation
}
```

```go
// Package B defines what behavior it NEEDS (not what A provides)
package aggregator

// B defines its OWN interface - doesn't need to import A's type
type CloudClient interface {
    GetMetrics() (*LocalMetrics, error)
}

// B accepts the interface, not the concrete type
func (d *DataAggregator) fetchData(client CloudClient) {
    // works with ANY type that has GetMetrics()
}
```

**Why This Works:**

```
Package A                          Package B
─────────                          ─────────
EnlightenCloudClient               CloudClient (interface)
  └─ GetMetrics()                    └─ GetMetrics()

A satisfies B's interface automatically (duck typing).
B never imports A. A never imports B.
No circular dependency!
```

**Interfaces vs Shared Types - When to Use Each:**

| Need | Solution | Why |
|------|----------|-----|
| Call methods on a type | Define interface in consumer | Behavior only, no fields needed |
| Access struct fields | Move to `internal/types/` | Must have concrete type |
| Construct/embed the type | Move to `internal/types/` | Must have concrete type |

**Example - Why `SystemConfig` Can't Use an Interface:**

```go
// This WON'T work with an interface - need struct fields:
func ProcessConfig(cfg SystemConfig) {
    name := cfg.Name    // Accessing field - need concrete type
    id := cfg.ID        // Accessing field - need concrete type
}

// This WILL work with an interface - only calling methods:
func FetchData(client CloudClient) {
    metrics, _ := client.GetMetrics()  // Method call only
}
```

That's why `SystemConfig` and `APIConfig` are in `internal/types/` (multiple packages need struct fields), while `CloudClient` is an interface in `internal/api/` (consumers only call methods).

---

### Why `internal/types/` instead of `internal/shared/`?

The Go convention is to name packages after WHAT they contain, not HOW they are used:
- `types` describes content: type definitions
- `shared` describes usage: used by multiple packages

This follows established Go ecosystem patterns:
- `go/types` (Go compiler's type system)
- `k8s.io/apimachinery/pkg/types` (Kubernetes common types)
- `database/sql` (not `database/shared_sql`)

The `internal/` prefix already signals these are implementation details, and documentation explains the sharing purpose.

```go
// internal/types/types.go - Shared types
type SystemConfig struct {
    Name string
    ID   string
}

// internal/config/config.go - Uses type alias for backward compatibility
type SystemConfig = types.SystemConfig

// internal/aggregator/aggregator.go - Uses type alias for backward compatibility
type SystemConfig = types.SystemConfig
```

**Benefits of Type Separation:**

1. **Clarity** - Easy to find type definitions in one place
2. **Reduced file size** - Avoids 500+ line files
3. **Import optimization** - Other packages can reference just the types
4. **Interface segregation** - Contracts separate from implementation
5. **Circular dependency avoidance** - Shared types can be imported by any package

---

### Go File Organization Terminology

| Term | Description | Example |
|------|-------------|---------|
| **Package-per-feature** | Each package handles one domain concept | `cache/`, `oauth/`, `display/` |
| **Type extraction** | Moving types to dedicated files | `types.go`, `interface.go` |
| **Shared types package** | Common types in separate package (named `types` by convention, not `shared`) | `internal/types/types.go` |
| **Interface files** | Defining contracts separate from implementation | `api/interface.go` |
| **Internal packages** | Compiler-enforced encapsulation | `internal/*` |
| **Type alias** | Re-exporting a type under a new name | `type Foo = pkg.Foo` |

---

### Type Aliases vs Type Definitions

Go provides two ways to create named types:

**Type Alias (used in this codebase):**
```go
// Type alias - SystemConfig IS types.SystemConfig (identical types)
type SystemConfig = types.SystemConfig

// Can be used interchangeably with types.SystemConfig
// No conversion needed when passing between packages
```

**Type Definition (creates new type):**
```go
// Type definition - MyConfig is a NEW type based on types.SystemConfig
type MyConfig types.SystemConfig

// NOT interchangeable - requires explicit conversion
// var cfg MyConfig = MyConfig(types.SystemConfig{...})
```

This codebase uses **type aliases** to:
1. Maintain backward compatibility when refactoring
2. Allow direct assignment without conversion
3. Keep code simple while eliminating duplication

---

## Related Documentation

- **[GO_BEST_PRACTICES.md](GO_BEST_PRACTICES.md)** - Comprehensive guide to Go patterns and best practices
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design patterns
