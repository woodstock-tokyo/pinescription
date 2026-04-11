// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

// Package series provides time series data structures and mathematical
// operations for the Pinescription runtime.
//
// The package implements Pine Script's series semantics: every value
// carries a historical buffer, allowing relative indexing and bar-by-bar
// evaluation without full recomputation.
//
// # Indexing Model
//
// The two primary access patterns are Last and Index, both using
// zero-based relative indexing where Last(0) or Index(0) refers to
// the most recent (latest) bar:
//
//	Series.Last(0)   // latest bar value
//	Series.Last(1)   // one bar ago
//	Series.Last(n)   // n bars ago
//
// The Length method returns the number of available bars. Accessing
// an index beyond Length returns zero (or false for BoolSeries),
// not an error.
//
// # Window and Lookback Behavior
//
// Functions that accept a lookback or window parameter operate on
// the most recent N bars. For example, Highest(s, 20) examines
// the latest 20 bars, and Mean(s, 14) computes a 14-bar average
// from the most recent data. If the series has fewer bars than
// requested, the function operates on all available data.
//
// # NaN and Zero Handling
//
// Division by zero returns math.NaN(). Accessing beyond the series
// length returns 0 for numeric series or false for boolean series.
// Empty series return 0 or false for all lookups.
//
// # Series Composition
//
// Series can be combined using Add, Sub, Div, Mul, or their equivalent
// methods on SeriesExtend. These operate element-wise: the result at
// each bar is the combination of the inputs at that bar.
//
// # Example
//
//	q := NewQueue(10)
//	for i := 0; i < 5; i++ {
//	    q.Update(float64(i * 2))
//	}
//	// q now holds [0, 2, 4, 6, 8]
//	sum := q.Sum()         // returns 20.0
//	mean := q.Mean()       // returns 4.0
//	highest := q.Highest(3) // examines [8, 6, 4], returns 8.0
package series
