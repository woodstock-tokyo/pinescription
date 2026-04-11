// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import "fmt"

func ExampleNewQueue() {
	q := NewQueue(3)
	q.Update(10)
	q.Update(20)
	q.Update(30)
	q.Update(40)

	fmt.Printf("latest=%.0f previous=%.0f oldest=%.0f length=%d\n", q.Last(0), q.Last(1), q.Last(2), q.Length())
	// Output:
	// latest=40 previous=30 oldest=20 length=3
}

func ExampleSeriesExtend_Mean() {
	q := NewQueue(5)
	for _, value := range []float64{2, 4, 6, 8} {
		q.Update(value)
	}

	var values SeriesExtend = NewSeries(q)
	fmt.Printf("all=%.1f last3=%.1f\n", values.Mean(), values.Mean(3))
	// Output:
	// all=5.0 last3=6.0
}

func ExampleCrossOver() {
	fast := NewQueue(2)
	slow := NewQueue(2)

	fast.Update(1)
	slow.Update(2)
	fast.Update(3)
	slow.Update(2)

	fmt.Println(CrossOver(fast, slow).Last(0))
	// Output:
	// true
}
