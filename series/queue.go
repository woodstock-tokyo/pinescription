// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

// Queue is a fixed-size circular buffer that implements Series.
// It stores up to size values, discarding oldest values when full.
// New values are added via Update, and existing values are accessed
// via Last or Index with Last(0) returning the most recent value.
type Queue struct {
	SeriesBase
	arr      []float64
	size     int
	start    int
	last     int
	realSize int
}

// getRealSize returns the next power of two minus one that is
// greater than or equal to n. Used internally for efficient
// circular buffer indexing.
func getRealSize(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	if ^uint(0)>>32 != 0 { // 64-bit system
		n |= n >> 32
	}
	return n
}

// NewQueue creates a new Queue with the specified maximum size.
// The queue starts empty and grows as values are added via Update.
func NewQueue(size int) *Queue {
	realSize := getRealSize(size)
	out := &Queue{
		arr:      make([]float64, 0, realSize+1),
		size:     size,
		start:    0,
		last:     -1,
		realSize: realSize,
	}
	out.SeriesBase.Series = out
	return out
}

func (inc *Queue) Last(i int) float64 {
	if i < 0 || len(inc.arr)-i-1 < 0 {
		return 0
	}

	return inc.arr[(inc.last-i)&inc.realSize]
}

func (inc *Queue) Index(i int) float64 {
	return inc.Last(i)
}

func (inc *Queue) Length() int {
	if inc.size < len(inc.arr) {
		return inc.size
	}
	return len(inc.arr)
}

// Clone returns a deep copy of the queue with the same state.
func (inc *Queue) Clone() *Queue {
	arrCopy := make([]float64, len(inc.arr), cap(inc.arr))
	copy(arrCopy, inc.arr)
	out := &Queue{
		arr:      arrCopy,
		size:     inc.size,
		start:    inc.start,
		last:     inc.last,
		realSize: inc.realSize,
	}
	out.SeriesBase.Series = out
	return out
}

// Update appends a new value to the queue. If the queue is at capacity,
// the oldest value is discarded to make room (circular buffer behavior).
func (inc *Queue) Update(v float64) {
	c := len(inc.arr)
	if c <= inc.realSize {
		inc.arr = append(inc.arr, v)
		inc.last++
	} else {
		if inc.size == 0 {
			return
		}
		inc.arr[inc.start] = v
		inc.start = (inc.start + 1) & inc.realSize
		inc.last = (inc.last + 1) & inc.realSize
	}
}
