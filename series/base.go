// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import (
	"fmt"
	"math"
	"reflect"
)

type Series interface {
	Last(i int) float64
	Index(i int) float64
	Length() int
}

func NewSeries(a Series) SeriesExtend {
	return &SeriesBase{
		Series: a,
	}
}

type SeriesExtend interface {
	Series
	Sum(limit ...int) float64
	Mean(limit ...int) float64
	Max(limit ...int) float64
	Min(limit ...int) float64
	Abs() SeriesExtend
	Predict(lookback int, offset ...int) float64
	NextCross(b Series, lookback int) (int, float64, bool)
	CrossOver(b Series) BoolSeries
	CrossUnder(b Series) BoolSeries
	Highest(lookback int) float64
	Lowest(lookback int) float64
	Add(b interface{}) SeriesExtend
	Minus(b interface{}) SeriesExtend
	Div(b interface{}) SeriesExtend
	Mul(b interface{}) SeriesExtend
	Dot(b interface{}, limit ...int) float64
	Array(limit ...int) (result []float64)
	Reverse(limit ...int) SeriesExtend
	Change(offset ...int) SeriesExtend
	PercentageChange(offset ...int) SeriesExtend
	Stdev(params ...int) float64
	Rolling(window int) *RollingResult
	Shift(offset int) SeriesExtend
	Skew(length int) float64
	Variance(length int) float64
	Covariance(b Series, length int) float64
	Correlation(b Series, length int, method ...CorrFunc) float64
	OLS(b SeriesExtend, length int) (float64, float64)
	AutoCorrelation(length int, lag ...int) float64
	Rank(length int) SeriesExtend
	Sigmoid() SeriesExtend
	Softmax(window int) SeriesExtend
	Entropy(window int) float64
	CrossEntropy(b Series, window int) float64
	Filter(b func(i int, value float64) bool, length int) SeriesExtend
}

// Index(0) maps to Last(0)
type BoolSeries interface {
	Last(i int) bool
	Index(i int) bool
	Length() int
}

type RollingResult struct {
	a      Series
	window int
}

func (r *RollingResult) Last() SeriesExtend {
	return NewSeries(&SliceView{r.a, 0, r.window})
}

func (r *RollingResult) Index(i int) SeriesExtend {
	if i*r.window > r.a.Length() {
		return nil
	}
	return NewSeries(&SliceView{r.a, i * r.window, r.window})
}

func (r *RollingResult) Length() int {
	mod := r.a.Length() % r.window
	if mod > 0 {
		return r.a.Length()/r.window + 1
	} else {
		return r.a.Length() / r.window
	}
}

type CorrFunc func(Series, Series, int) float64

type SliceView struct {
	a      Series
	start  int
	length int
}

func (s *SliceView) Last(i int) float64 {
	if i >= s.length {
		return 0
	}

	return s.a.Last(i + s.start)
}

func (s *SliceView) Index(i int) float64 {
	return s.Last(i)
}

func (s *SliceView) Length() int {
	return s.length
}

type AbsResult struct {
	a Series
}

func (a *AbsResult) Last(i int) float64 {
	return math.Abs(a.a.Last(i))
}

func (a *AbsResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *AbsResult) Length() int {
	return a.a.Length()
}

type AddSeriesResult struct {
	a Series
	b Series
}

// Add two series, result[i] = a[i] + b[i]
func Add(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&AddSeriesResult{aa, bb})
}

func (a *AddSeriesResult) Last(i int) float64 {
	return a.a.Last(i) + a.b.Last(i)
}

func (a *AddSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *AddSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &AddSeriesResult{}

// Sub two series, result[i] = a[i] - b[i]
func Sub(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&MinusSeriesResult{aa, bb})
}

type MinusSeriesResult struct {
	a Series
	b Series
}

func (a *MinusSeriesResult) Last(i int) float64 {
	return a.a.Last(i) - a.b.Last(i)
}

func (a *MinusSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *MinusSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &MinusSeriesResult{}

func SwitchIface(b interface{}) Series {
	switch tp := b.(type) {
	case float64:
		return NumberSeries(tp)
	case int32:
		return NumberSeries(float64(tp))
	case int64:
		return NumberSeries(float64(tp))
	case float32:
		return NumberSeries(float64(tp))
	case int:
		return NumberSeries(float64(tp))
	case Series:
		return tp
	default:
		fmt.Println(reflect.TypeOf(b))
		panic("input should be either *Series or numbers")

	}
}

func Div(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&DivSeriesResult{aa, bb})
}

type DivSeriesResult struct {
	a Series
	b Series
}

func (a *DivSeriesResult) Last(i int) float64 {
	denom := a.b.Last(i)
	if denom == 0 {
		return math.NaN()
	}
	return a.a.Last(i) / denom
}

func (a *DivSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *DivSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &DivSeriesResult{}

// Multiple two series, result[i] = a[i] * b[i]
func Mul(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&MulSeriesResult{aa, bb})
}

type MulSeriesResult struct {
	a Series
	b Series
}

func (a *MulSeriesResult) Last(i int) float64 {
	return a.a.Last(i) * a.b.Last(i)
}

func (a *MulSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *MulSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &MulSeriesResult{}

type NumberSeries float64

func (a NumberSeries) Last(_ int) float64 {
	return float64(a)
}

func (a NumberSeries) Index(_ int) float64 {
	return float64(a)
}

func (a NumberSeries) Length() int {
	return math.MaxInt32
}

func (a NumberSeries) Clone() NumberSeries {
	return a
}

var _ Series = NumberSeries(0)

// Calculate (a dot b).
// if limit is given, will only calculate the first limit numbers (a.Index[0..limit])
// otherwise will operate on all elements
func Dot(a interface{}, b interface{}, limit ...int) float64 {
	var aaf float64
	var aas Series
	var bbf float64
	var bbs Series
	var isaf, isbf bool

	switch tp := a.(type) {
	case float64:
		aaf = tp
		isaf = true
	case int32:
		aaf = float64(tp)
		isaf = true
	case int64:
		aaf = float64(tp)
		isaf = true
	case float32:
		aaf = float64(tp)
		isaf = true
	case int:
		aaf = float64(tp)
		isaf = true
	case Series:
		aas = tp
		isaf = false
	default:
		panic("input should be either *Series or numbers")
	}
	switch tp := b.(type) {
	case float64:
		bbf = tp
		isbf = true
	case int32:
		bbf = float64(tp)
		isbf = true
	case int64:
		bbf = float64(tp)
		isbf = true
	case float32:
		bbf = float64(tp)
		isbf = true
	case int:
		bbf = float64(tp)
		isbf = true
	case Series:
		bbs = tp
		isbf = false
	default:
		panic("input should be either *Series or numbers")
	}
	l := 1
	if len(limit) > 0 {
		l = limit[0]
	} else if isaf && isbf {
		l = 1
	} else {
		if !isaf {
			l = aas.Length()
		}
		if !isbf {
			if l > bbs.Length() {
				l = bbs.Length()
			}
		}
	}
	if isaf && isbf {
		return aaf * bbf * float64(l)
	} else if isaf && !isbf {
		sum := 0.
		for i := 0; i < l; i++ {
			sum += aaf * bbs.Last(i)
		}
		return sum
	} else if !isaf && isbf {
		sum := 0.
		for i := 0; i < l; i++ {
			sum += aas.Last(i) * bbf
		}
		return sum
	} else {
		sum := 0.
		for i := 0; i < l; i++ {
			sum += aas.Last(i) * bbs.Index(i)
		}
		return sum
	}
}

type ChangeResult struct {
	a      Series
	offset int
}

func (c *ChangeResult) Last(i int) float64 {
	if i+c.offset >= c.a.Length() {
		return 0
	}
	return c.a.Last(i) - c.a.Last(i+c.offset)
}

func (c *ChangeResult) Index(i int) float64 {
	return c.Last(i)
}

func (c *ChangeResult) Length() int {
	length := c.a.Length()
	if length >= c.offset {
		return length - c.offset
	}
	return 0
}

type PercentageChangeResult struct {
	a      Series
	offset int
}

func (c *PercentageChangeResult) Last(i int) float64 {
	if i+c.offset >= c.a.Length() {
		return 0
	}
	denom := c.a.Last(i + c.offset)
	if denom == 0 {
		return math.NaN()
	}
	return c.a.Last(i)/denom - 1
}

func (c *PercentageChangeResult) Index(i int) float64 {
	return c.Last(i)
}

func (c *PercentageChangeResult) Length() int {
	length := c.a.Length()
	if length >= c.offset {
		return length - c.offset
	}
	return 0
}

type ShiftResult struct {
	a      Series
	offset int
}

func (inc *ShiftResult) Last(i int) float64 {
	if inc.offset+i < 0 {
		return 0
	}
	if inc.offset+i >= inc.a.Length() {
		return 0
	}

	return inc.a.Last(inc.offset + i)
}

func (inc *ShiftResult) Index(i int) float64 {
	return inc.Last(i)
}

func (inc *ShiftResult) Length() int {
	length := inc.a.Length()
	if length >= inc.offset {
		return length - inc.offset
	}
	return 0
}
