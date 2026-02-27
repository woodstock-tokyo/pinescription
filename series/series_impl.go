// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package series

import (
	"math"

	"gonum.org/v1/gonum/stat"
)

// Calculate sum of the series
// if limit is given, will only sum first limit numbers (a.Index[0..limit])
// otherwise will sum all elements
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

// Calculate the average value of the series
// if limit is given, will only calculate the average of first limit numbers (a.Index[0..limit])
// otherwise will operate on all elements
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

// Return series that having all the elements positive
func Abs(a Series) SeriesExtend {
	return NewSeries(&AbsResult{a})
}

func Predict(a Series, lookback int, offset ...int) float64 {
	alpha, beta := LinearRegression(a, lookback)
	o := -1.0
	if len(offset) > 0 {
		o = -float64(offset[0])
	}
	return alpha + beta*o
}

// This will make prediction using Linear Regression to get the next cross point
// Return (offset from latest, crossed value, could cross)
// offset from latest should always be positive
// lookback param is to use at most `lookback` points to determine linear regression functions
//
// You may also refer to excel's FORECAST function
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

// Array extracts elements from the Series to a float64 array, following the order of Index(0..limit)
// if limit is given, will only take the first limit numbers (a.Index[0..limit])
// otherwise will operate on all elements
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

// Ordinary Least Squares fit result, only support 1d array
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

// Similar to Array but in reverse order.
// Useful when you want to cache series' calculated result as float64 array
// the then reuse the result in multiple places (so that no recalculation will be triggered)
//
// notice that the return type is a Float64Slice, which implements the Series interface
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

// Difference between current value and previous, a - a[offset]
// offset: if not given, offset is 1.
func Change(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&ChangeResult{a, o})
}

// Percentage change between current and a prior element, a / a[offset] - 1.
// offset: if not given, offset is 1.
func PercentageChange(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&PercentageChangeResult{a, o})
}

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

// similar to pandas.Series.corr() function.
//
// method could either be `types.Pearson`, `types.Spearman` or `types.Kendall`
func Correlation(a Series, b Series, length int, method ...CorrFunc) float64 {
	var runner CorrFunc
	if len(method) == 0 {
		runner = Pearson
	} else {
		runner = method[0]
	}
	return runner(a, b, length)
}

// similar to pandas.Series.autocorr() function.
//
// The method computes the Pearson correlation between Series and shifted itself
func AutoCorrelation(a Series, length int, lags ...int) float64 {
	lag := 1
	if len(lags) > 0 {
		lag = lags[0]
	}
	return Pearson(a, Shift(a, lag), length)
}

// similar to pandas.Series.cov() function with ddof=0
//
// Compute covariance with Series
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

func Variance(a Series, length int) float64 {
	return Covariance(a, a, length)
}

// similar to pandas.Series.skew() function.
//
// Return unbiased skew over input series
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

func Shift(a Series, offset int) SeriesExtend {
	return NewSeries(&ShiftResult{a, offset})
}

func Rolling(a Series, window int) *RollingResult {
	return &RollingResult{a, window}
}

// Softmax returns the input value in the range of 0 to 1
// with sum of all the probabilities being equal to one.
// It is commonly used in machine learning neural networks.
// Will return Softmax SeriesExtend result based in latest [window] numbers from [a] Series
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

// Entropy computes the Shannon entropy of a distribution or the distance between
// two distributions. The natural logarithm is used.
// - sum(v * ln(v))
func Entropy(a Series, window int) (e float64) {
	for i := 0; i < window; i++ {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(v)
		}
	}
	return e
}

// CrossEntropy computes the cross-entropy between the two distributions
func CrossEntropy(a, b Series, window int) (e float64) {
	for i := 0; i < window; i++ {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(b.Last(i))
		}
	}
	return e
}

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
