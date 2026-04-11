// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import (
	"fmt"
	"math"
	"reflect"
)

// Series represents a sequence of values indexed by bar position.
// Implementations hold a historical buffer; Last(0) returns the latest bar,
// Last(1) returns one bar ago, and so on. Accessing beyond Length returns 0.
type Series interface {
	// Last returns the value at the given offset from the latest bar.
	// Last(0) is the most recent bar, Last(1) is one bar ago.
	// Returns 0 if the offset exceeds the available data.
	Last(i int) float64
	// Index is an alias for Last in most implementations.
	// Some implementations may interpret this differently (e.g., absolute indexing).
	Index(i int) float64
	// Length returns the number of bars available in the series.
	Length() int
}

// NewSeries wraps a Series into a SeriesExtend, enabling the full suite
// of mathematical and statistical methods. The returned value implements
// SeriesExtend and can be used for chaining operations.
//
//	q := NewSeries(mySeries)
//	result := q.Sum(20) // sum of last 20 bars
func NewSeries(a Series) SeriesExtend {
	return &SeriesBase{
		Series: a,
	}
}

// SeriesExtend augments Series with a rich set of mathematical and
// statistical operations. All methods that return a SeriesExtend produce
// a new series that can be further chained. Methods that accept a
// lookback or window parameter examine the most recent N bars; if fewer
// than N bars are available, they operate on all available data.
type SeriesExtend interface {
	Series

	// Sum returns the sum of the most recent N values.
	// If limit is not provided, sums all values.
	Sum(limit ...int) float64

	// Mean returns the arithmetic mean of the most recent N values.
	// Returns 0 for empty series.
	Mean(limit ...int) float64

	// Max returns the maximum value among the most recent N bars.
	// If the series has fewer than lookback bars, examines all available.
	Max(limit ...int) float64

	// Min returns the minimum value among the most recent N bars.
	// If the series has fewer than lookback bars, examines all available.
	Min(limit ...int) float64

	// Abs returns a series where each value is the absolute value of the input.
	Abs() SeriesExtend

	// Predict returns the forecasted value using linear regression.
	// The forecast is offset bars ahead (negative offset means past).
	// Uses the most recent lookback points for the regression.
	Predict(lookback int, offset ...int) float64

	// NextCross predicts when series a will cross series b.
	// Returns (bars until-crossing, crossing-value, whether-crossing-will-happen).
	// Uses linear regression over lookback bars for prediction.
	NextCross(b Series, lookback int) (int, float64, bool)

	// CrossOver returns a BoolSeries that is true when a crosses above b.
	CrossOver(b Series) BoolSeries

	// CrossUnder returns a BoolSeries that is true when a crosses below b.
	CrossUnder(b Series) BoolSeries

	// Highest returns the maximum value in the most recent lookback bars.
	Highest(lookback int) float64

	// Lowest returns the minimum value in the most recent lookback bars.
	Lowest(lookback int) float64

	// Add returns the element-wise sum of the series and b.
	// b may be a number or another Series.
	Add(b interface{}) SeriesExtend

	// Minus returns the element-wise difference (a - b).
	// b may be a number or another Series.
	Minus(b interface{}) SeriesExtend

	// Div returns the element-wise division (a / b).
	// Division by zero produces NaN. b may be a number or another Series.
	Div(b interface{}) SeriesExtend

	// Mul returns the element-wise product of the series and b.
	// b may be a number or another Series.
	Mul(b interface{}) SeriesExtend

	// Dot returns the dot product of the series with b.
	// If limit is given, only the first N elements are used.
	// b may be a number or another Series.
	Dot(b interface{}, limit ...int) float64

	// Array extracts the most recent N values as a []float64.
	// If limit is not provided, returns all values, with result[0] holding
	// the most recent value.
	Array(limit ...int) (result []float64)

	// Reverse returns a series with values in reverse order (oldest to newest).
	Reverse(limit ...int) SeriesExtend

	// Change returns the difference between the current value and
	// the value offset bars ago. Default offset is 1.
	Change(offset ...int) SeriesExtend

	// PercentageChange returns (current / value-offset-ago) - 1.
	// Division by zero produces NaN. Default offset is 1.
	PercentageChange(offset ...int) SeriesExtend

	// Stdev returns the sample standard deviation.
	// The first parameter sets the window (defaults to series length).
	// The second parameter sets ddof (delta degrees of freedom, default 0).
	Stdev(params ...int) float64

	// Rolling groups values into non-overlapping windows of the given size.
	Rolling(window int) *RollingResult

	// Shift returns a series shifted by the given offset.
	// Positive offset moves values into the past (discards recent values).
	// Negative offset brings future values into the present (filled with 0).
	Shift(offset int) SeriesExtend

	// Skew returns the sample skewness over the most recent length bars.
	// Returns NaN if length is too small or variance is zero.
	Skew(length int) float64

	// Variance returns the population variance over the most recent length bars.
	Variance(length int) float64

	// Covariance returns the covariance between this series and b
	// over the most recent length bars. Uses ddof=0 (population covariance).
	Covariance(b Series, length int) float64

	// Correlation returns the correlation coefficient between this series and b
	// over the most recent length bars. Default method is Pearson correlation.
	Correlation(b Series, length int, method ...CorrFunc) float64

	// OLS performs ordinary least squares regression using this series as the
	// independent variable and b as the dependent variable.
	// Returns (alpha, beta) where b ≈ alpha + beta*thisSeries.
	OLS(b SeriesExtend, length int) (float64, float64)

	// AutoCorrelation returns the autocorrelation at the given lag.
	// Default lag is 1.
	AutoCorrelation(length int, lag ...int) float64

	// Rank returns a series of fractional ranks over the window.
	// Tied values receive the average of their ranks.
	Rank(length int) SeriesExtend

	// Sigmoid returns the logistic sigmoid (1 / (1 + exp(-x))) for each value.
	Sigmoid() SeriesExtend

	// Softmax returns the softmax activation over the most recent window values.
	// Output values sum to 1.0.
	Softmax(window int) SeriesExtend

	// Entropy returns the Shannon entropy of the distribution over the window.
	// Uses natural logarithm. Ignores zero values.
	Entropy(window int) float64

	// CrossEntropy returns the cross-entropy between this series and series b
	// over the most recent window values. Both series should be probability
	// distributions (non-negative, typically summing to 1).
	CrossEntropy(b Series, window int) float64

	// Filter returns a series containing only values where the predicate
	// returns true. Examines the most recent length matching elements.
	Filter(b func(i int, value float64) bool, length int) SeriesExtend
}

// BoolSeries represents a sequence of boolean values indexed by bar position.
// Like Series, it uses Last(0) for the latest bar and Last(1) for one bar ago.
// Accessing beyond Length returns false.
type BoolSeries interface {
	// Last returns the boolean value at the given offset from the latest bar.
	// Last(0) is the most recent bar, Last(1) is one bar ago.
	// Returns false if the offset exceeds the available data.
	Last(i int) bool
	// Index is an alias for Last in most implementations.
	Index(i int) bool
	// Length returns the number of bars available.
	Length() int
}

// RollingResult represents a view into a series divided into
// non-overlapping windows. It is created by the Rolling method.
// Window 0 is the most recent window, window 1 is the next older window,
// and so on.
type RollingResult struct {
	a      Series
	window int
}

// Last returns the most recent window as a SeriesExtend.
func (r *RollingResult) Last() SeriesExtend {
	return NewSeries(&SliceView{r.a, 0, r.window})
}

// Index returns the window at the given window index.
// Returns nil if the index exceeds the number of available windows.
func (r *RollingResult) Index(i int) SeriesExtend {
	if i*r.window > r.a.Length() {
		return nil
	}
	return NewSeries(&SliceView{r.a, i * r.window, r.window})
}

// Length returns the number of complete windows.
func (r *RollingResult) Length() int {
	mod := r.a.Length() % r.window
	if mod > 0 {
		return r.a.Length()/r.window + 1
	} else {
		return r.a.Length() / r.window
	}
}

// CorrFunc defines a correlation computation method.
// It receives two series and a length, and returns the correlation coefficient.
type CorrFunc func(Series, Series, int) float64

// SliceView presents a contiguous slice of a Series as its own Series.
// It is used internally by RollingResult to provide window views.
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

// AbsResult holds an input series and returns the absolute value
// of each element when queried.
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

// AddSeriesResult performs element-wise addition of two series.
type AddSeriesResult struct {
	a Series
	b Series
}

// Add returns the element-wise sum of a and b.
// Each argument may be a Series or a numeric value (float64, int, etc.).
// The result length is the minimum of the two input lengths.
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

// MinusSeriesResult performs element-wise subtraction (a - b).
type MinusSeriesResult struct {
	a Series
	b Series
}

// Sub returns the element-wise difference (a - b).
// Each argument may be a Series or a numeric value.
// The result length is the minimum of the two input lengths.
func Sub(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&MinusSeriesResult{aa, bb})
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

// SwitchIface converts a numeric value or Series into a Series.
// Numeric values are wrapped in a NumberSeries with infinite length.
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

// DivSeriesResult performs element-wise division (a / b).
type DivSeriesResult struct {
	a Series
	b Series
}

// Div returns the element-wise division of a by b.
// Each argument may be a Series or a numeric value.
// Division by zero produces NaN. The result length is the minimum of the inputs.
func Div(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&DivSeriesResult{aa, bb})
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

// MulSeriesResult performs element-wise multiplication of two series.
type MulSeriesResult struct {
	a Series
	b Series
}

// Mul returns the element-wise product of a and b.
// Each argument may be a Series or a numeric value.
// The result length is the minimum of the two input lengths.
func Mul(a interface{}, b interface{}) SeriesExtend {
	aa := SwitchIface(a)
	bb := SwitchIface(b)
	return NewSeries(&MulSeriesResult{aa, bb})
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

// NumberSeries wraps a constant numeric value as a Series.
// It always returns the same value regardless of the index.
// Length is reported as math.MaxInt32 to indicate effectively unlimited data.
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

// Clone returns a copy of the NumberSeries.
func (a NumberSeries) Clone() NumberSeries {
	return a
}

var _ Series = NumberSeries(0)

// Dot computes the dot product of a and b.
// If limit is provided, only the first N elements are used.
// Each argument may be a Series or a numeric value.
// For two scalar values, returns their product multiplied by the limit.
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

// ChangeResult computes the difference between a value and its value
// at a prior offset. It is created by the Change function.
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

// PercentageChangeResult computes the percentage change between a value
// and its value at a prior offset. It is created by the PercentageChange function.
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

// ShiftResult offsets the values of a series by a fixed amount.
// It is created by the Shift function.
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
