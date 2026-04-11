// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import (
	"math"

	"gonum.org/v1/gonum/stat"
)

// Sum returns the sum of the most recent N values in the series.
// If limit is not provided, sums all values in the series.
// Returns 0 for empty series.
func Sum(a Series, limit ...int) (sum float64) {
	l := a.Length()
	if len(limit) > 0 && limit[0] < l {
		l = limit[0]
	}
	for i := 0; i < l; i++ {
		sum += a.Last(i)
	}
	return sum
}

// Mean returns the arithmetic mean of the most recent N values.
// If limit is not provided, computes the mean over all values.
// Returns 0 for empty series.
func Mean(a Series, limit ...int) (mean float64) {
	l := a.Length()
	if l == 0 {
		return 0
	}
	if len(limit) > 0 && limit[0] < l {
		l = limit[0]
	}
	return Sum(a, l) / float64(l)
}

// Maximum returns the maximum value among the most recent N bars.
// Examines Last(0) through Last(limit-1). If limit is not provided,
// examines all available bars. Returns 0 for empty series.
func Maximum(a Series, limit ...int) (max float64) {
	l := a.Length()
	if l == 0 {
		return 0
	}
	if len(limit) > 0 && limit[0] < l {
		l = limit[0]
	}
	max = a.Last(0)
	for i := 1; i < l; i++ {
		v := a.Last(i)
		if v > max {
			max = v
		}
	}
	return max
}

// Minimum returns the minimum value among the most recent N bars.
// Examines Last(0) through Last(limit-1). If limit is not provided,
// examines all available bars. Returns 0 for empty series.
func Minimum(a Series, limit ...int) (min float64) {
	l := a.Length()
	if l == 0 {
		return 0
	}
	if len(limit) > 0 && limit[0] < l {
		l = limit[0]
	}
	min = a.Last(0)
	for i := 1; i < l; i++ {
		v := a.Last(i)
		if v < min {
			min = v
		}
	}
	return min
}

// Abs returns a new series where each value is the absolute value
// of the corresponding input value.
func Abs(a Series) SeriesExtend {
	return NewSeries(&AbsResult{a})
}

// Predict returns the forecasted value using linear regression.
// The forecast is offset bars ahead (negative offset means past values).
// Uses the most recent lookback points for the regression.
func Predict(a Series, lookback int, offset ...int) float64 {
	alpha, beta := LinearRegression(a, lookback)
	o := -1.0
	if len(offset) > 0 {
		o = -float64(offset[0])
	}
	return alpha + beta*o
}

// NextCross predicts when series a will cross series b using linear regression.
// It examines the most recent lookback bars of both series.
//
// Returns (bars-until-crossing, crossing-value, will-cross).
// - bars-until-crossing is positive if the cross occurs in the future.
// - crossing-value is the y-value at the predicted crossing point.
// - will-cross is false if the lines are parallel or converging away.
//
// This is similar to Excel's FORECAST function extended to find intersection points.
func NextCross(a Series, b Series, lookback int) (int, float64, bool) {
	if a.Length() < lookback {
		lookback = a.Length()
	}
	if b.Length() < lookback {
		lookback = b.Length()
	}
	x := make([]float64, lookback)
	y1 := make([]float64, lookback)
	y2 := make([]float64, lookback)
	var weights []float64
	for i := 0; i < lookback; i++ {
		x[i] = float64(i)
		y1[i] = a.Last(i)
		y2[i] = b.Last(i)
	}
	alpha1, beta1 := stat.LinearRegression(x, y1, weights, false)
	alpha2, beta2 := stat.LinearRegression(x, y2, weights, false)
	if beta2 == beta1 {
		return 0, 0, false
	}
	indexf := (alpha1 - alpha2) / (beta2 - beta1)

	// crossed in different direction
	if indexf >= 0 {
		return 0, 0, false
	}
	return int(math.Ceil(-indexf)), alpha1 + beta1*indexf, true
}

// Highest returns the maximum value among the most recent lookback bars.
// If the series has fewer than lookback bars, examines all available.
// Returns 0 for empty series.
func Highest(a Series, lookback int) float64 {
	if lookback > a.Length() {
		lookback = a.Length()
	}
	highest := a.Last(0)
	for i := 1; i < lookback; i++ {
		current := a.Last(i)
		if highest < current {
			highest = current
		}
	}
	return highest
}

// Lowest returns the minimum value among the most recent lookback bars.
// If the series has fewer than lookback bars, examines all available.
// Returns 0 for empty series.
func Lowest(a Series, lookback int) float64 {
	if lookback > a.Length() {
		lookback = a.Length()
	}
	lowest := a.Last(0)
	for i := 1; i < lookback; i++ {
		current := a.Last(i)
		if lowest > current {
			lowest = current
		}
	}
	return lowest
}

// LinearRegression fits a line to the most recent lookback values using
// ordinary least squares. Returns (alpha, beta) where the fitted line is
// y = alpha + beta*x, with x being the bar index (0, 1, 2, ...).
func LinearRegression(a Series, lookback int) (alpha float64, beta float64) {
	if a.Length() < lookback {
		lookback = a.Length()
	}
	x := make([]float64, lookback)
	y := make([]float64, lookback)
	var weights []float64
	for i := 0; i < lookback; i++ {
		x[i] = float64(i)
		y[i] = a.Last(i)
	}
	alpha, beta = stat.LinearRegression(x, y, weights, false)
	return
}

// Array extracts the most recent N values into a []float64 slice.
// If limit is not provided, returns all values in the series.
// The slice order matches Last indexing, so result[0] is Last(0).
func Array(a Series, limit ...int) (result []float64) {
	l := a.Length()
	if len(limit) > 0 && l > limit[0] {
		l = limit[0]
	}
	if l > a.Length() {
		l = a.Length()
	}
	result = make([]float64, l)
	for i := 0; i < l; i++ {
		result[i] = a.Last(i)
	}
	return
}

// OLS performs ordinary least squares regression using series a as the
// independent variable and series b as the dependent variable.
// Returns (alpha, beta) where b ≈ alpha + beta*a.
func OLS(a SeriesExtend, b SeriesExtend, n int) (float64, float64) {
	if a.Length() < n {
		n = a.Length()
	}
	if b.Length() < n {
		n = b.Length()
	}
	numerator := 0.0
	denominator := 0.0
	meana := a.Mean(n)
	meanb := b.Mean(n)
	for i := 0; i < n; i++ {
		x := a.Last(i)
		y := b.Last(i)
		numerator += (x - meana) * (y - meanb)
		denominator += (x - meana) * (x - meana)
	}
	if denominator == 0 {
		return 0, 0
	}
	beta := numerator / denominator
	alpha := meanb - beta*meana
	return alpha, beta
}

// Reverse returns a series with values in reverse order.
// The returned series presents oldest value first when iterated.
// This is useful for caching computed results as a float64 array,
// ensuring no recalculation occurs on subsequent accesses.
// The returned type implements the Series interface.
func Reverse(a Series, limit ...int) SeriesExtend {
	l := a.Length()
	if len(limit) > 0 && l > limit[0] {
		l = limit[0]
	}
	result := NewQueue(l)
	for i := 0; i < l; i++ {
		result.Update(a.Last(l - i - 1))
	}
	return result
}

// Rank computes fractional ranks for each value in the most recent length bars.
// Returned ranks are one-based and tied values receive the average of their ranks.
func Rank(a Series, length int) SeriesExtend {
	if length > a.Length() {
		length = a.Length()
	}
	rank := make([]float64, length)
	mapper := make([]float64, length+1)
	for i := length - 1; i >= 0; i-- {
		ii := a.Last(i)
		counter := 0.
		for j := 0; j < length; j++ {
			if a.Last(j) <= ii {
				counter += 1.
			}
		}
		rank[i] = counter
		mapper[int(counter)] += 1.
	}
	output := NewQueue(length)
	for i := length - 1; i >= 0; i-- {
		output.Update(rank[i] - (mapper[int(rank[i])]-1.)/2)
	}
	return output
}

// Change returns a series representing the difference between the current
// value and the value offset bars ago. Default offset is 1.
func Change(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&ChangeResult{a, o})
}

// PercentageChange returns a series representing the percentage change
// between the current value and the value offset bars ago.
// Computes (current / prior) - 1. Default offset is 1.
// Division by zero produces NaN.
func PercentageChange(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&PercentageChangeResult{a, o})
}

// Stdev returns the sample standard deviation over the most recent N values.
// The first parameter sets the window size (defaults to full series length).
// The second parameter sets ddof (delta degrees of freedom, default 0).
// With ddof=0, computes population standard deviation.
func Stdev(a Series, params ...int) float64 {
	length := a.Length()
	if length == 0 {
		return 0
	}
	if len(params) > 0 && params[0] < length {
		length = params[0]
	}
	ddof := 0
	if len(params) > 1 {
		ddof = params[1]
	}
	avg := Mean(a, length)
	s := .0
	for i := 0; i < length; i++ {
		diff := a.Last(i) - avg
		s += diff * diff
	}
	if length-ddof == 0 {
		return 0
	}
	return math.Sqrt(s / float64(length-ddof))
}

// Pearson returns the Pearson correlation coefficient between series a and b
// over the most recent length values.
func Pearson(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	x := make([]float64, length)
	y := make([]float64, length)
	for i := 0; i < length; i++ {
		x[i] = a.Last(i)
		y[i] = b.Last(i)
	}
	return stat.Correlation(x, y, nil)
}

// Correlation returns the correlation coefficient between series a and b
// over the most recent length bars. Default method is Pearson correlation.
// Other methods can be specified using CorrFunc: Pearson, Spearman, or Kendall.
func Correlation(a Series, b Series, length int, method ...CorrFunc) float64 {
	var runner CorrFunc
	if len(method) == 0 {
		runner = Pearson
	} else {
		runner = method[0]
	}
	return runner(a, b, length)
}

// AutoCorrelation returns the autocorrelation of the series at the specified lag.
// It computes the Pearson correlation between the series and itself shifted by lag.
// Default lag is 1.
func AutoCorrelation(a Series, length int, lags ...int) float64 {
	lag := 1
	if len(lags) > 0 {
		lag = lags[0]
	}
	return Pearson(a, Shift(a, lag), length)
}

// Covariance returns the population covariance between series a and b
// over the most recent length bars. Uses ddof=0.
func Covariance(a Series, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}

	meana := Mean(a, length)
	meanb := Mean(b, length)
	sum := 0.0
	for i := 0; i < length; i++ {
		sum += (a.Last(i) - meana) * (b.Last(i) - meanb)
	}
	sum /= float64(length)
	return sum
}

// Variance returns the population variance over the most recent length bars.
// This is equivalent to Covariance(a, a, length).
func Variance(a Series, length int) float64 {
	return Covariance(a, a, length)
}

// Skew returns the sample skewness over the most recent length bars.
// Uses the unbiased estimator formula. Returns NaN if length is too small
// (less than 3) or if variance is zero.
func Skew(a Series, length int) float64 {
	if length > a.Length() {
		length = a.Length()
	}
	mean := Mean(a, length)
	sum2 := 0.0
	sum3 := 0.0
	for i := 0; i < length; i++ {
		diff := a.Last(i) - mean
		sum2 += diff * diff
		sum3 += diff * diff * diff
	}
	if length <= 2 || sum2 == 0 {
		return math.NaN()
	}
	l := float64(length)
	return l * math.Sqrt(l-1) / (l - 2) * sum3 / math.Pow(sum2, 1.5)
}

// Shift returns a series shifted by the given offset.
// Positive offset moves values into the past (discards recent values).
// Negative offset brings future values into the present (filled with 0).
func Shift(a Series, offset int) SeriesExtend {
	return NewSeries(&ShiftResult{a, offset})
}

// Rolling groups the series into non-overlapping windows of the given size.
// Returns a RollingResult which can be used to access each window.
func Rolling(a Series, window int) *RollingResult {
	return &RollingResult{a, window}
}

// Softmax computes the softmax activation over the most recent window values.
// Returns a new series where each value is exp(x) / sum(exp(x)) for the window.
// Output values are in the range (0, 1) and sum to 1.0.
func Softmax(a Series, window int) SeriesExtend {
	s := 0.0
	max := Highest(a, window)
	for i := 0; i < window; i++ {
		s += math.Exp(a.Last(i) - max)
	}
	out := NewQueue(window)
	for i := window - 1; i >= 0; i-- {
		out.Update(math.Exp(a.Last(i)-max) / s)
	}
	return out
}

// Entropy computes the Shannon entropy of the distribution over the most recent
// window values. Uses the natural logarithm. Zero values are ignored.
func Entropy(a Series, window int) (e float64) {
	for i := 0; i < window; i++ {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(v)
		}
	}
	return e
}

// CrossEntropy computes the cross-entropy between series a and series b
// over the most recent window values. Both series should be probability
// distributions (non-negative values). Uses the natural logarithm.
func CrossEntropy(a, b Series, window int) (e float64) {
	for i := 0; i < window; i++ {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(b.Last(i))
		}
	}
	return e
}

// Kendall returns Kendall's tau correlation coefficient between series a and b
// over the most recent length bars. This is a rank-based correlation measure
// that counts concordant and discordant pairs.
func Kendall(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	aRanks := Rank(a, length)
	bRanks := Rank(b, length)
	concordant, discordant := 0, 0
	for i := 0; i < length; i++ {
		for j := i + 1; j < length; j++ {
			value := (aRanks.Last(i) - aRanks.Last(j)) * (bRanks.Last(i) - bRanks.Last(j))
			if value > 0 {
				concordant++
			} else {
				discordant++
			}
		}
	}
	return float64(concordant-discordant) * 2.0 / float64(length*(length-1))
}

// Spearman returns Spearman's rank correlation coefficient between series a and b
// over the most recent length bars. This is equivalent to the Pearson correlation
// of the rank-transformed values.
func Spearman(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	aRank := Rank(a, length)
	bRank := Rank(b, length)
	return Pearson(aRank, bRank, length)
}

// SeriesBase is a wrapper of the Series interface.
// You can embed any data structure that implements Series into SeriesBase
// to create a new Series that Supports SeriesExtend methods.
type SeriesBase struct {
	Series
}

func (s *SeriesBase) Index(i int) float64 {
	return s.Last(i)
}

func (s *SeriesBase) Last(i int) float64 {
	if s.Series == nil {
		return 0
	}
	return s.Series.Last(i)
}

func (s *SeriesBase) Length() int {
	if s.Series == nil {
		return 0
	}
	return s.Series.Length()
}

func (s *SeriesBase) Sum(limit ...int) float64 {
	return Sum(s, limit...)
}

func (s *SeriesBase) Mean(limit ...int) float64 {
	return Mean(s, limit...)
}

func (s *SeriesBase) Max(limit ...int) float64 {
	return Maximum(s, limit...)
}

func (s *SeriesBase) Min(limit ...int) float64 {
	return Minimum(s, limit...)
}

func (s *SeriesBase) Abs() SeriesExtend {
	return Abs(s)
}

func (s *SeriesBase) Predict(lookback int, offset ...int) float64 {
	return Predict(s, lookback, offset...)
}

func (s *SeriesBase) NextCross(b Series, lookback int) (int, float64, bool) {
	return NextCross(s, b, lookback)
}

func (s *SeriesBase) CrossOver(b Series) BoolSeries {
	return CrossOver(s, b)
}

func (s *SeriesBase) CrossUnder(b Series) BoolSeries {
	return CrossUnder(s, b)
}

func (s *SeriesBase) Highest(lookback int) float64 {
	return Highest(s, lookback)
}

func (s *SeriesBase) Lowest(lookback int) float64 {
	return Lowest(s, lookback)
}

func (s *SeriesBase) Add(b interface{}) SeriesExtend {
	return Add(s, b)
}

func (s *SeriesBase) Minus(b interface{}) SeriesExtend {
	return Sub(s, b)
}

func (s *SeriesBase) Div(b interface{}) SeriesExtend {
	return Div(s, b)
}

func (s *SeriesBase) Mul(b interface{}) SeriesExtend {
	return Mul(s, b)
}

func (s *SeriesBase) Dot(b interface{}, limit ...int) float64 {
	return Dot(s, b, limit...)
}

func (s *SeriesBase) Array(limit ...int) []float64 {
	return Array(s, limit...)
}

func (s *SeriesBase) Reverse(limit ...int) SeriesExtend {
	return Reverse(s, limit...)
}

func (s *SeriesBase) Change(offset ...int) SeriesExtend {
	return Change(s, offset...)
}

func (s *SeriesBase) PercentageChange(offset ...int) SeriesExtend {
	return PercentageChange(s, offset...)
}

func (s *SeriesBase) Stdev(params ...int) float64 {
	return Stdev(s, params...)
}

func (s *SeriesBase) Rolling(window int) *RollingResult {
	return Rolling(s, window)
}

func (s *SeriesBase) Shift(offset int) SeriesExtend {
	return Shift(s, offset)
}

func (s *SeriesBase) Skew(length int) float64 {
	return Skew(s, length)
}

func (s *SeriesBase) Variance(length int) float64 {
	return Variance(s, length)
}

func (s *SeriesBase) Covariance(b Series, length int) float64 {
	return Covariance(s, b, length)
}

func (s *SeriesBase) Correlation(b Series, length int, method ...CorrFunc) float64 {
	return Correlation(s, b, length, method...)
}

func (s *SeriesBase) AutoCorrelation(length int, lag ...int) float64 {
	return AutoCorrelation(s, length, lag...)
}

func (s *SeriesBase) Rank(length int) SeriesExtend {
	return Rank(s, length)
}

func (s *SeriesBase) Sigmoid() SeriesExtend {
	return Sigmoid(s)
}

func (s *SeriesBase) Softmax(window int) SeriesExtend {
	return Softmax(s, window)
}

func (s *SeriesBase) Entropy(window int) float64 {
	return Entropy(s, window)
}

func (s *SeriesBase) CrossEntropy(b Series, window int) float64 {
	return CrossEntropy(s, b, window)
}

func (s *SeriesBase) Filter(b func(int, float64) bool, length int) SeriesExtend {
	return FilterSeries(s, b, length)
}

func (s *SeriesBase) OLS(b SeriesExtend, window int) (float64, float64) {
	return OLS(s, b, window)
}
