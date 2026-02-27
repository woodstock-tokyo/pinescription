// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

// The result structure that maps to the crossing result of `CrossOver` and `CrossUnder`
// Accessible through BoolSeries interface
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

// a series cross above b series.
// If in current KLine, a is higher than b, and in previous KLine, a is lower than b, then return true.
// Otherwise return false.
// If accessing index >= length, will always return false
func CrossOver(a Series, b Series) BoolSeries {
	return &CrossResult{a, b, true}
}

// a series cross under b series.
// If in current KLine, a is lower than b, and in previous KLine, a is higher than b, then return true.
// Otherwise return false.
// If accessing index >= length, will always return false
func CrossUnder(a Series, b Series) BoolSeries {
	return &CrossResult{a, b, false}
}
