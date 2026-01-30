// Package parser - parser_bench_test.go
//
// PURPOSE
// -------
// Performance benchmarks for parser functions to identify optimization opportunities.
// Benchmarks cover JSON parsing, interval summing, and response reading.
//
// RUNNING BENCHMARKS
// ------------------
// Run all benchmarks:
//   go test -bench=. ./internal/parser
//
// Run specific benchmark:
//   go test -bench=BenchmarkParseTelemetryResponse ./internal/parser
//
// With memory stats:
//   go test -bench=. -benchmem ./internal/parser

// Package parser - parser_bench_test.go
//
// TEST SETUP
// ----------
// This file contains benchmark tests for parser performance measurement.
// Benchmarks help identify bottlenecks and verify optimization effectiveness.
//
// BENCHMARK PLAN
// --------------
// 1. JSON Parsing Benchmarks
//    - Benchmark flat array parsing (production, consumption)
//    - Benchmark nested array parsing (grid import/export)
//    - Measure memory allocations (bytes/op, allocs/op)
//
// 2. Interval Summing Benchmarks
//    - Benchmark summing across different field names
//    - Test with varying interval counts (1, 10, 96 intervals)
//
// RUNNING BENCHMARKS
// ------------------
// Run all benchmarks:
//   go test -bench=. ./internal/parser
//
// Run with memory stats:
//   go test -bench=. -benchmem ./internal/parser
//
// Compare before/after optimization:
//   go test -bench=. -benchmem > before.txt
//   # Make optimization changes
//   go test -bench=. -benchmem > after.txt
//   benchcmp before.txt after.txt
//
// PERFORMANCE TARGETS
// -------------------
// Based on typical API responses (96 intervals per day):
// - Parsing: < 50 µs per response
// - Memory: < 10 KB per parse
// - Allocations: < 100 allocs per parse
//
// PATTERN USED
// ------------
// - Pattern 9: Benchmark Tests (testing.B)
//
// See TESTING.md for detailed pattern explanations.
package parser

import (
	"bytes"
	"io"
	"testing"
)

// BenchmarkParseTelemetryResponse benchmarks parsing flat array responses
func BenchmarkParseTelemetryResponse(b *testing.B) {
	// Sample production meter response (flat array format)
	data := []byte(`{
		"intervals": [
			{"end_at": 1706054400, "wh_del": 150.5, "enwh": 145.2},
			{"end_at": 1706055300, "wh_del": 160.3, "enwh": 155.8},
			{"end_at": 1706056200, "wh_del": 170.1, "enwh": 165.4},
			{"end_at": 1706057100, "wh_del": 155.7, "enwh": 150.9},
			{"end_at": 1706058000, "wh_del": 165.2, "enwh": 160.5}
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseTelemetryResponse(data)
		if err != nil {
			b.Fatalf("ParseTelemetryResponse failed: %v", err)
		}
	}
}

// BenchmarkParseNestedTelemetryResponse benchmarks parsing nested array responses
func BenchmarkParseNestedTelemetryResponse(b *testing.B) {
	// Sample energy import response (nested array format)
	data := []byte(`{
		"intervals": [
			[
				{"end_at": 1706054400, "wh_imported": 120.5},
				{"end_at": 1706055300, "wh_imported": 130.3}
			],
			[
				{"end_at": 1706056200, "wh_imported": 140.1},
				{"end_at": 1706057100, "wh_imported": 125.7}
			]
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseNestedTelemetryResponse(data)
		if err != nil {
			b.Fatalf("ParseNestedTelemetryResponse failed: %v", err)
		}
	}
}

// BenchmarkSumIntervalValues benchmarks summing interval values
func BenchmarkSumIntervalValues(b *testing.B) {
	// Create sample intervals
	intervals := []TelemetryInterval{
		{EndAt: 1706054400, WhDel: 150.5, Enwh: 145.2},
		{EndAt: 1706055300, WhDel: 160.3, Enwh: 155.8},
		{EndAt: 1706056200, WhDel: 170.1, Enwh: 165.4},
		{EndAt: 1706057100, WhDel: 155.7, Enwh: 150.9},
		{EndAt: 1706058000, WhDel: 165.2, Enwh: 160.5},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SumIntervalValues(intervals, "wh_del")
	}
}

// BenchmarkSumIntervalValues_Large benchmarks with larger dataset (96 intervals = 1 day)
func BenchmarkSumIntervalValues_Large(b *testing.B) {
	// Create 96 intervals (15-min intervals for 24 hours)
	intervals := make([]TelemetryInterval, 96)
	baseTime := int64(1706054400)
	for i := 0; i < 96; i++ {
		intervals[i] = TelemetryInterval{
			EndAt: baseTime + int64(i*900), // 900 seconds = 15 minutes
			WhDel: 150.0 + float64(i)*0.5,
			Enwh:  145.0 + float64(i)*0.5,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SumIntervalValues(intervals, "wh_del")
	}
}

// BenchmarkReadResponseBody benchmarks HTTP response body reading
func BenchmarkReadResponseBody(b *testing.B) {
	// Sample response body
	bodyBytes := []byte(`{
		"intervals": [
			{"end_at": 1706054400, "wh_del": 150.5, "enwh": 145.2},
			{"end_at": 1706055300, "wh_del": 160.3, "enwh": 155.8},
			{"end_at": 1706056200, "wh_del": 170.1, "enwh": 165.4}
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := io.NopCloser(bytes.NewReader(bodyBytes))
		_, err := ReadResponseBody(body)
		if err != nil {
			b.Fatalf("ReadResponseBody failed: %v", err)
		}
	}
}

// BenchmarkParseTelemetryResponse_WithUnmarshal measures complete parse cycle
func BenchmarkParseTelemetryResponse_WithUnmarshal(b *testing.B) {
	data := []byte(`{
		"intervals": [
			{"end_at": 1706054400, "wh_del": 150.5, "enwh": 145.2},
			{"end_at": 1706055300, "wh_del": 160.3, "enwh": 155.8},
			{"end_at": 1706056200, "wh_del": 170.1, "enwh": 165.4},
			{"end_at": 1706057100, "wh_del": 155.7, "enwh": 150.9},
			{"end_at": 1706058000, "wh_del": 165.2, "enwh": 160.5}
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		intervals, err := ParseTelemetryResponse(data)
		if err != nil {
			b.Fatalf("ParseTelemetryResponse failed: %v", err)
		}
		// Simulate real usage - sum the values
		_ = SumIntervalValues(intervals, "wh_del")
	}
}

// BenchmarkParseNestedTelemetryResponse_Complete measures nested parse with flatten
func BenchmarkParseNestedTelemetryResponse_Complete(b *testing.B) {
	data := []byte(`{
		"intervals": [
			[
				{"end_at": 1706054400, "wh_imported": 120.5},
				{"end_at": 1706055300, "wh_imported": 130.3},
				{"end_at": 1706056200, "wh_imported": 140.1}
			],
			[
				{"end_at": 1706057100, "wh_imported": 125.7},
				{"end_at": 1706058000, "wh_imported": 135.2},
				{"end_at": 1706058900, "wh_imported": 145.8}
			]
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		intervals, err := ParseNestedTelemetryResponse(data)
		if err != nil {
			b.Fatalf("ParseNestedTelemetryResponse failed: %v", err)
		}
		// Simulate real usage
		_ = SumIntervalValues(intervals, "wh_imported")
	}
}
