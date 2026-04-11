// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import "math"

// SigmoidResult applies the logistic sigmoid function to each value
// in the underlying series.
type SigmoidResult struct {
	a Series
}

func (s *SigmoidResult) Last(i int) float64 {
	return 1. / (1. + math.Exp(-s.a.Last(i)))
}

func (s *SigmoidResult) Index(i int) float64 {
	return s.Last(i)
}

func (s *SigmoidResult) Length() int {
	return s.a.Length()
}

// Sigmoid returns a series where each value is transformed through
// the logistic sigmoid function: 1 / (1 + exp(-x)).
// Output values are in the range (0, 1).
func Sigmoid(a Series) SeriesExtend {
	return NewSeries(&SigmoidResult{a})
}
