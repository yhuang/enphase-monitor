package main

// ANSI font style constants
const (
	// Reset resets all text formatting and colors
	Reset = "\033[0m"
	// Bold makes text bold
	Bold = "\033[1m"
)

// Error Message Constants
// These constants ensure consistent error identification across the codebase.
const (
	// RateLimitError is the identifier for rate limit (429) errors.
	// Used in aggregator.go, main.go, and cloud_client.go for consistent error detection.
	RateLimitError = "rate limit exceeded (429)"
)
