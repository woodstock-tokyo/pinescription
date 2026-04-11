// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

// FilterResult holds a series and a predicate function. When queried,
// it returns only values for which the predicate returns true.
// It caches matched indices to avoid recomputation.
type FilterResult struct {
	a      Series
	b      func(int, float64) bool
	length int
	c      []int
}

func (f *FilterResult) Last(j int) float64 {
	if j >= f.length {
		return 0
	}
	if len(f.c) > j {
		return f.a.Last(f.c[j])
	}
	l := f.a.Length()
	k := len(f.c)
	i := 0
	if k > 0 {
		i = f.c[k-1] + 1
	}
	for ; i < l; i++ {
		tmp := f.a.Last(i)
		if f.b(i, tmp) {
			f.c = append(f.c, i)
			if j == k {
				return tmp
			}
			k++
		}
	}
	return 0
}

func (f *FilterResult) Index(j int) float64 {
	return f.Last(j)
}

func (f *FilterResult) Length() int {
	return f.length
}

// FilterSeries returns a new series containing only values from the input series
// for which the predicate function returns true. The predicate receives the
// bar index and value at each position.
//
// The returned series will contain at most the most recent 'length' matching elements.
// Accessing beyond that index returns 0.
//
// The predicate function should be pure (no side effects). Subsequent calls to
// the predicate for the same index may return different results if the
// underlying series changes.
func FilterSeries(a Series, b func(i int, value float64) bool, length int) SeriesExtend {
	return NewSeries(&FilterResult{a, b, length, nil})
}
