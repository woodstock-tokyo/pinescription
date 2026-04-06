// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"testing"
	"time"

	series "github.com/woodstock-tokyo/pinescription/series"
)

var (
	benchSeriesSink []float64
	benchMatrixSink *Matrix
	benchFloatSink  float64
)

func benchMaterializeSeries(ser SeriesExtended) []float64 {
	if ser == nil || ser.Length() == 0 {
		return nil
	}
	n := ser.Length()
	out := make([]float64, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, ser.Last(i))
	}
	return out
}

func benchmarkRuntimeWithClose(size int) *Runtime {
	q := series.NewQueue(size + 8)
	for i := 0; i < size; i++ {
		q.Update(float64((i % 97) + 1))
	}

	seriesByKey := map[string]SeriesExtended{makeSeriesKey("BENCH", "close"): q}
	valueTypesBySymbol := map[string]map[string]bool{"BENCH": {"close": true}}
	rt := newRuntime(Program{}, nil, seriesByKey, valueTypesBySymbol, "BENCH", "close", "1D", "regular", time.Now().UTC(), time.Now().UTC(), q.Length(), nil, nil, nil)
	rt.barIndex = q.Length() - 1
	return rt
}

func BenchmarkSeriesFromExprExtraction(b *testing.B) {
	rt := benchmarkRuntimeWithClose(20000)
	expr := &Expr{
		Kind:  "binary",
		Op:    "*",
		Left:  &Expr{Kind: "ident", Name: "close"},
		Right: &Expr{Kind: "number", Number: 2},
	}

	b.Run("slow", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := rt.seriesFromExprSlow(expr)
			if err != nil {
				b.Fatalf("seriesFromExprSlow failed: %v", err)
			}
			benchSeriesSink = benchMaterializeSeries(out)
		}
	})

	b.Run("fast", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := rt.seriesFromExpr(expr)
			if err != nil {
				b.Fatalf("seriesFromExpr failed: %v", err)
			}
			benchSeriesSink = benchMaterializeSeries(out)
		}
	})
}

func benchMatrix(size int) *Matrix {
	m, _ := newMatrix(size, size, 0)
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			m.Data[r][c] = float64((r*size+c)%31 + 1)
		}
	}
	return m
}

func matrixMultNaiveBench(a, b *Matrix) (*Matrix, error) {
	if a.cols() != b.rows() {
		return nil, nil
	}
	out, _ := newMatrix(a.rows(), b.cols(), 0)
	for i := 0; i < a.rows(); i++ {
		for j := 0; j < b.cols(); j++ {
			sum := 0.0
			for k := 0; k < a.cols(); k++ {
				sum += a.Data[i][k] * b.Data[k][j]
			}
			out.Data[i][j] = sum
		}
	}
	return out, nil
}

func BenchmarkMatrixMult(b *testing.B) {
	a := benchMatrix(64)
	c := benchMatrix(64)

	b.Run("naive", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := matrixMultNaiveBench(a, c)
			if err != nil {
				b.Fatalf("naive matrix mult failed: %v", err)
			}
			benchMatrixSink = out
		}
	})

	b.Run("optimized", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := matrixMult(a, c)
			if err != nil {
				b.Fatalf("optimized matrix mult failed: %v", err)
			}
			benchMatrixSink = out
		}
	})
}

func BenchmarkExecuteIndicators(b *testing.B) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("BENCH", makeBenchValues(5000)...))
	e.SetDefaultSymbol("BENCH")

	bytecode, err := e.Compile(`ta.stdev(close, 20) + ta.correlation(close, close * 2, 20) + ta.highest(close, 20) + ta.lowest(close, 20)`)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.Run("legacy_full_series", func(b *testing.B) {
		disableWindowOptimizations = true
		defer func() { disableWindowOptimizations = false }()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})

	b.Run("optimized_window", func(b *testing.B) {
		disableWindowOptimizations = false
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})
}

func BenchmarkExecuteFisherTransform(b *testing.B) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("BENCH", makeBenchValues(5000)...))
	e.SetDefaultSymbol("BENCH")

	bytecode, err := e.Compile(`
len = 9
hh = ta.highest(close, len)
ll = ta.lowest(close, len)
norm = hh - ll == 0 ? 0.0 : ((close - ll) / (hh - ll) - 0.5)
var x = 0.0
x := 0.66 * norm + 0.67 * nz(x[1])
x := math.max(math.min(x, 0.999), -0.999)
var fisher = 0.0
fisher := 0.5 * math.log((1 + x) / (1 - x)) + 0.5 * nz(fisher[1])
fisher
`)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := e.Execute(bytecode)
		if err != nil {
			b.Fatalf("execute failed: %v", err)
		}
		f, ok := v.(float64)
		if !ok {
			b.Fatalf("expected float64 result, got %T", v)
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			b.Fatalf("unexpected fisher result: %v", f)
		}
		benchFloatSink = f
	}
}

func BenchmarkExecuteUserFunctionDefinition(b *testing.B) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("BENCH", makeBenchValues(5000)...))
	e.SetDefaultSymbol("BENCH")

	bytecode, err := e.Compile(`
double(x) => x * 2
mix(a, b) => a + b * 0.5
mix(double(close), double(close[1])) + double(close[2])
`)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.Run("legacy_no_pools", func(b *testing.B) {
		disableCallArgPooling = true
		disableEnvMapPooling = true
		defer func() {
			disableCallArgPooling = false
			disableEnvMapPooling = false
		}()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})

	b.Run("optimized_pooled", func(b *testing.B) {
		disableCallArgPooling = false
		disableEnvMapPooling = false
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})
}

func BenchmarkExecuteLoopOperation(b *testing.B) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("BENCH", makeBenchValues(5000)...))
	e.SetDefaultSymbol("BENCH")

	bytecode, err := e.Compile(`
var s = 0
for i = 0 to 100
    s = s + close
s
`)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.Run("legacy_assign_history", func(b *testing.B) {
		disableLoopIteratorFastPath = true
		defer func() { disableLoopIteratorFastPath = false }()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})

	b.Run("optimized_local_iterator", func(b *testing.B) {
		disableLoopIteratorFastPath = false
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})
}

func BenchmarkExecuteSwitchCases(b *testing.B) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("BENCH", makeBenchValues(5000)...))
	e.SetDefaultSymbol("BENCH")

	bytecode, err := e.Compile(`
var out = 0
switch bar_index % 5
    0 => out = close
    1 => out = close + 1
    2 => out = close + 2
    3 => out = close + 3
    => out = close + 4
out
`)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	b.Run("legacy_eval_case_expr", func(b *testing.B) {
		disableSwitchCaseConstFastPath = true
		defer func() { disableSwitchCaseConstFastPath = false }()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})

	b.Run("optimized_const_case_fastpath", func(b *testing.B) {
		disableSwitchCaseConstFastPath = false
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			v, err := e.Execute(bytecode)
			if err != nil {
				b.Fatalf("execute failed: %v", err)
			}
			if f, ok := v.(float64); ok {
				benchFloatSink = f
			}
		}
	})
}

func makeBenchValues(n int) []float64 {
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = float64((i%157)+1) * 1.01
	}
	return out
}

type benchStreamingProvider struct {
	symbol    string
	closeQ    *series.Queue
	volumeQ   *series.Queue
	timeframe string
	session   string
}

func newBenchStreamingProvider(symbol string, closeVals, volumeVals []float64, maxBars int) *benchStreamingProvider {
	if maxBars < len(closeVals) {
		maxBars = len(closeVals)
	}
	qClose := series.NewQueue(maxBars)
	for _, v := range closeVals {
		qClose.Update(v)
	}
	qVol := series.NewQueue(maxBars)
	for _, v := range volumeVals {
		qVol.Update(v)
	}
	return &benchStreamingProvider{symbol: symbol, closeQ: qClose, volumeQ: qVol, timeframe: "1D", session: "regular"}
}

func (p *benchStreamingProvider) Update(close, volume float64) {
	p.closeQ.Update(close)
	p.volumeQ.Update(volume)
}

func (p *benchStreamingProvider) GetSeries(seriesKey string) (SeriesExtended, error) {
	s, vt, ok := splitSeriesKey(seriesKey)
	if !ok || s != p.symbol {
		return nil, fmt.Errorf("unknown series key: %s", seriesKey)
	}
	switch vt {
	case "close":
		return p.closeQ, nil
	case "volume":
		return p.volumeQ, nil
	default:
		return nil, fmt.Errorf("unknown value type: %s", vt)
	}
}

func (p *benchStreamingProvider) GetSymbols() ([]string, error) {
	return []string{p.symbol}, nil
}

func (p *benchStreamingProvider) GetValuesTypes() ([]string, error) {
	return []string{"close", "volume"}, nil
}

func (p *benchStreamingProvider) SetTimeframe(timeframe string) error {
	p.timeframe = timeframe
	return nil
}

func (p *benchStreamingProvider) GetTimeframe() string {
	if p.timeframe == "" {
		return "1D"
	}
	return p.timeframe
}

func (p *benchStreamingProvider) SetSession(session string) error {
	p.session = session
	return nil
}

func (p *benchStreamingProvider) GetSession() string {
	if p.session == "" {
		return "regular"
	}
	return p.session
}

func benchExpectedBBUpper(closeSeries SeriesExtended, length int, mult float64) float64 {
	if closeSeries == nil || closeSeries.Length() < length || length <= 0 {
		return math.NaN()
	}
	sum := 0.0
	for i := 0; i < length; i++ {
		sum += closeSeries.Last(i)
	}
	mean := sum / float64(length)
	ss := 0.0
	for i := 0; i < length; i++ {
		d := closeSeries.Last(i) - mean
		ss += d * d
	}
	dev := math.Sqrt(ss / float64(length))
	return mean + mult*dev
}

func BenchmarkExecuteBollingerStreamingUpdates(b *testing.B) {
	const (
		symbol = "BENCH"
		length = 20
		mult   = 2.0
	)
	script := "[mid, up, low] = ta.bb(close, 20, 2.0)\nup"

	setup := func(totalBars int) (*benchStreamingProvider, *Runtime) {
		closeVals := makeBenchValues(400)
		volumeVals := makeBenchValues(400)
		provider := newBenchStreamingProvider(symbol, closeVals, volumeVals, totalBars+64)

		e := NewEngine()
		e.RegisterMarketDataProvider(provider)
		e.SetDefaultSymbol(symbol)

		bytecode, err := e.Compile(script)
		if err != nil {
			b.Fatalf("compile failed: %v", err)
		}
		program, err := decodeProgram(bytecode)
		if err != nil {
			b.Fatalf("decode failed: %v", err)
		}

		seriesByKey := map[string]SeriesExtended{
			makeSeriesKey(symbol, "close"):  provider.closeQ,
			makeSeriesKey(symbol, "volume"): provider.volumeQ,
		}
		valueTypesBySymbol := map[string]map[string]bool{symbol: {"close": true, "volume": true}}

		rt := newRuntime(program, nil, seriesByKey, valueTypesBySymbol, symbol, "close", "1D", "regular", time.Now().UTC(), time.Now().UTC(), totalBars+8, func(sym, vt string) (SeriesExtended, error) {
			return provider.GetSeries(makeSeriesKey(sym, vt))
		}, nil, nil)

		initialLen := provider.closeQ.Length()
		for i := 0; i < initialLen; i++ {
			rt.barIndex = i
			if err := rt.execTopLevel(); err != nil {
				b.Fatalf("warmup exec failed: %v", err)
			}
			if err := rt.commitBar(); err != nil {
				b.Fatalf("warmup commit failed: %v", err)
			}
		}
		return provider, rt
	}

	runCase := func(b *testing.B, disableBBFastPath bool) {
		disableIncrementalBB = disableBBFastPath
		defer func() { disableIncrementalBB = false }()

		provider, rt := setup(400 + b.N)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			closeV := float64((i%211)+1) * 1.031
			volumeV := float64((i%113)+50) * 10.0
			provider.Update(closeV, volumeV)

			rt.barIndex = provider.closeQ.Length() - 1
			if err := rt.execTopLevel(); err != nil {
				b.Fatalf("stream exec failed: %v", err)
			}
			if err := rt.commitBar(); err != nil {
				b.Fatalf("stream commit failed: %v", err)
			}
			benchFloatSink = rt.lastValue
		}
		b.StopTimer()

		got := rt.lastValue
		expected := benchExpectedBBUpper(provider.closeQ, length, mult)
		if math.IsNaN(got) || math.IsNaN(expected) {
			b.Fatalf("unexpected NaN result: got=%v expected=%v", got, expected)
		}
		if math.Abs(got-expected) > 1e-7 {
			b.Fatalf("unexpected final bb upper: got=%v expected=%v", got, expected)
		}
	}

	b.Run("legacy_window_recompute", func(b *testing.B) {
		runCase(b, true)
	})
	b.Run("optimized_incremental_bb", func(b *testing.B) {
		runCase(b, false)
	})
}
