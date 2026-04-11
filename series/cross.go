// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

// CrossResult represents the crossing state between two series.
// It implements BoolSeries, returning true when a cross event is detected.
type CrossResult struct {
	a      Series
	b      Series
	isOver bool
}

func (c *CrossResult) Last(i int) bool {
	if c.Length() == 0 {
		return false
	}
	if i+1 >= c.Length() {
		return false
	}
	if c.isOver {
		return c.a.Last(i)-c.b.Last(i) > 0 && c.a.Last(i+1)-c.b.Last(i+1) < 0
	} else {
		return c.a.Last(i)-c.b.Last(i) < 0 && c.a.Last(i+1)-c.b.Last(i+1) > 0
	}
}

func (c *CrossResult) Index(i int) bool {
	return c.Last(i)
}

func (c *CrossResult) Length() int {
	la := c.a.Length()
	lb := c.b.Length()
	if la > lb {
		return lb
	}
	return la
}

// CrossOver returns a BoolSeries that is true when series a crosses above series b.
// The cross is detected when a was below b in the previous bar but is above b
// in the current bar. Returns false for indices beyond the series length.
func CrossOver(a Series, b Series) BoolSeries {
	return &CrossResult{a, b, true}
}

// CrossUnder returns a BoolSeries that is true when series a crosses below series b.
// The cross is detected when a was above b in the previous bar but is below b
// in the current bar. Returns false for indices beyond the series length.
func CrossUnder(a Series, b Series) BoolSeries {
	return &CrossResult{a, b, false}
}
