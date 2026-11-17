// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

type unavailableExactProvider struct {
	*testProvider
	failType string
	empty    bool
}

func (p *unavailableExactProvider) GetSeries(key string) (SeriesExtended, error) {
	if strings.HasSuffix(key, "|"+p.failType) {
		if p.empty {
			return mkSeries(), nil
		}
		return nil, fmt.Errorf("unavailable %s", p.failType)
	}
	return p.testProvider.GetSeries(key)
}

func TestExactSeriesNeverFallsBackToClose(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, valueType := range []string{"open", "high", "low", "volume", "bid", "ask"} {
			t.Run(fmt.Sprintf("%s/empty=%v", valueType, empty), func(t *testing.T) {
				base := &testProvider{data: map[string]SeriesExtended{}, timeframe: "60"}
				for _, vt := range []string{"open", "high", "low", "close", "volume", "bid", "ask"} {
					base.data["TEST|"+vt] = mkSeries(10)
				}
				e := NewEngine()
				e.RegisterMarketDataProvider(&unavailableExactProvider{base, valueType, empty})
				e.SetSymbolInfo("TEST", SymbolInfo{"mintick": 1.0, "pointvalue": 1.0, "currency": "USD"})
				code, err := e.Compile("result = close")
				if err != nil {
					t.Fatal(err)
				}
				rt, _, err := e.ExecuteWithRuntime(code)
				if err != nil {
					t.Fatal(err)
				}
				defer rt.Release()
				if got := rt.exactSeriesValue(valueType, 0); !math.IsNaN(got) {
					t.Fatalf("exact %s: got %v, want na", valueType, got)
				}
				ser, err := rt.getSeries("TEST", valueType)
				if err != nil || ser.Last(0) != 10 {
					t.Fatalf("legacy fallback changed: %v, %v", ser, err)
				}
				if valueType == "open" || valueType == "high" || valueType == "low" {
					code, err = e.Compile(`strategy("Missing OHLC",margin_long=0,margin_short=0)
result = close`)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = e.Execute(code); err == nil || !strings.Contains(err.Error(), "invalid initial "+valueType) {
						t.Fatalf("got %v, want missing OHLC error", err)
					}
				}
			})
		}
	}
}

func TestVWAPMissingAndZeroBarsDoNotPoisonAccumulation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		source, volume []float64
	}{
		{"missing_source", []float64{10, math.NaN(), 30}, []float64{1, 1, 1}},
		{"missing_volume", []float64{10, 20, 30}, []float64{1, math.NaN(), 1}},
		{"zero_volume", []float64{10, 20, 30}, []float64{1, 0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine()
			e.RegisterMarketDataProvider(&testProvider{data: map[string]SeriesExtended{
				"TEST|close": mkSeries(tc.source...), "TEST|volume": mkSeries(tc.volume...),
			}, timeframe: "60"})
			e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			code, err := e.Compile(`[mean,upper,lower] = ta.vwap(close,bar_index==0,1)
missing_mean = mean[1]
missing_upper = upper[1]
missing_lower = lower[1]`)
			if err != nil {
				t.Fatal(err)
			}
			rt, _, err := e.ExecuteWithRuntime(code)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			for name, want := range map[string]float64{"mean": 20, "upper": 30, "lower": 10} {
				got, _ := rt.Value(name)
				if got != want {
					t.Fatalf("%s: got %v, want %v", name, got, want)
				}
				if missing, _ := rt.Value("missing_" + name); !isNA(missing) {
					t.Fatalf("missing %s: got %v, want na", name, missing)
				}
			}
		})
	}
}

func TestVWAPZeroArgumentCallsHaveIndependentState(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(&testProvider{data: map[string]SeriesExtended{
		"TEST|close": mkSeries(10, 20, 30), "TEST|high": mkSeries(10, 20, 30),
		"TEST|low": mkSeries(10, 20, 30), "TEST|volume": mkSeries(1, 1, 1),
	}, timeframe: "60"})
	e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	code, err := e.Compile(`first = ta.vwap()
second = bar_index == 0 ? na : ta.vwap()
bare = ta.vwap`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	for name, want := range map[string]float64{"first": 20, "second": 25, "bare": 20} {
		if got, _ := rt.Value(name); got != want {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}
}

func TestCalendarBucketReviewRegressions(t *testing.T) {
	for _, tc := range []struct{ name, date, expression, want string }{
		{"overnight_open", "2025-01-07T01:00:00Z", `time("1D","1800-1700:23456","UTC")`, "2025-01-06T18:00:00Z"},
		{"overnight_close", "2025-01-07T01:00:00Z", `time_close("1D","1800-1700:23456","UTC")`, "2025-01-07T17:00:00Z"},
		{"previous_clipped_month_close", "2025-12-31T12:00:00Z", `time_close("5M","","UTC",0,1)`, "2025-11-01T00:00:00Z"},
		{"previous_clipped_month_open", "2025-01-01T12:00:00Z", `time("5M","","UTC",0,1)`, "2024-11-01T00:00:00Z"},
		{"next_clipped_month_close", "2025-11-01T12:00:00Z", `time_close("5M","","UTC",0,-1)`, "2026-06-01T00:00:00Z"},
		{"previous_clipped_day_close", "2026-01-01T12:00:00Z", `time_close("2D","","UTC",0,1)`, "2026-01-01T00:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEngine()
			e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
			start, _ := time.Parse(time.RFC3339, tc.date)
			e.SetStartTime(start)
			code, err := e.Compile("result = " + tc.expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := e.Execute(code)
			want, _ := time.Parse(time.RFC3339, tc.want)
			if err != nil || got != float64(want.UnixMilli()) {
				t.Fatalf("got %v, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestMonthlyInferredTimesKeepFinalAnchor(t *testing.T) {
	for _, date := range []string{"2025-03-31T12:34:56Z", "2024-03-31T12:34:56Z", "2025-03-30T12:34:56Z", "2025-03-15T12:34:56Z"} {
		t.Run(date, func(t *testing.T) {
			final, _ := time.Parse(time.RFC3339, date)
			e := NewEngine()
			p := providerWithClose("TEST", 1, 2)
			p.timeframe = "1M"
			e.RegisterMarketDataProvider(p)
			e.SetCurrentTime(final)
			code, err := e.Compile("result = time\nprevious = time[1]")
			if err != nil {
				t.Fatal(err)
			}
			rt, _, err := e.ExecuteWithRuntime(code)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			if got, _ := rt.Value("result"); got != float64(final.UnixMilli()) {
				t.Fatalf("got %v, want final %s", got, final)
			}
			previous := shiftTimeframe(final, -1, "1M")
			if got, _ := rt.Value("previous"); got != float64(previous.UnixMilli()) {
				t.Fatalf("got previous %v, want %s", got, previous)
			}
			if final.Day() == 31 && shiftTimeframe(previous, 1, "1M") != final {
				t.Fatal("month-end shifts are not inverse")
			}
		})
	}
}

func TestMarketContextAndCalendarAPIs(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("NYSE:TEST", 1, 2))
	e.SetSymbolInfo("NYSE:TEST", SymbolInfo{"timezone": "America/New_York", "mintick": 0.25, "currency": "USD"})
	open := time.Date(2025, 1, 2, 1, 0, 0, 0, time.UTC)
	e.SetBarTimes([]BarTime{{Open: open, Close: open.Add(time.Hour)}, {Open: open.Add(48 * time.Hour), Close: open.Add(49 * time.Hour)}})
	code, err := e.Compile(`result = hour + hour(time, "UTC") + dayofmonth(time) + weekofyear
text = str.format_time(time, "yyyy-MM-dd HH:mm", "America/New_York")
tick = math.round_to_mintick(10.125)
negative_tick = math.round_to_mintick(-10.125)
last = last_bar_time
symbol = syminfo.prefix("NYSE:TEST") + ":" + syminfo.ticker("NYSE:TEST")
formatted = str.tostring(10.125,format.mintick)
unknown = syminfo.pointvalue`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	for name, want := range map[string]interface{}{
		"result": float64(25), "text": "2025-01-03 20:00", "tick": 10.25,
		"negative_tick": -10.0, "last": float64(open.Add(48 * time.Hour).UnixMilli()),
		"symbol": "NYSE:TEST", "formatted": "10.25",
	} {
		if got, _ := rt.Value(name); got != want {
			t.Fatalf("%s: got %#v,want %#v", name, got, want)
		}
	}
	if got, _ := rt.Value("unknown"); !isNA(got) {
		t.Fatalf("metadata was invented: %v", got)
	}
}

func TestHistoricalBarstateAndSessionFlags(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("TEST", 1, 2, 3, 4)
	p.timeframe = "30"
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC))
	e.SetMarketSessions("TEST", MarketSessions{Premarket: "0900-0930:23456", Regular: "0930-1000:23456", Postmarket: "1000-1100:23456"})
	type observation struct{ first, last, regFirst, regLast, market, pre, post, history, confirmed, firstBar, lastHistory bool }
	var got []observation
	e.RegisterFunction("capture", func(a ...interface{}) (interface{}, error) {
		got = append(got, observation{a[0].(bool), a[1].(bool), a[2].(bool), a[3].(bool), a[4].(bool), a[5].(bool), a[6].(bool), a[7].(bool), a[8].(bool), a[9].(bool), a[10].(bool)})
		return nil, nil
	})
	code, err := e.Compile(`capture(session.isfirstbar,session.islastbar,session.isfirstbar_regular,session.islastbar_regular,session.ismarket,session.ispremarket,session.ispostmarket,barstate.ishistory,barstate.isconfirmed,barstate.isfirst,barstate.islastconfirmedhistory)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Execute(code); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || !got[0].first || !got[0].pre || !got[0].firstBar || got[0].lastHistory ||
		!got[1].regFirst || !got[1].regLast || !got[1].market ||
		!got[2].post || got[2].last || !got[3].last || !got[3].lastHistory {
		t.Fatalf("incorrect flags: %+v", got)
	}
	for _, g := range got {
		if !g.history || !g.confirmed {
			t.Fatal(g)
		}
	}
}

func TestFormatTimeDefaultEmitsActualTimezoneOffset(t *testing.T) {
	for _, tc := range []struct{ zone, timestamp, want string }{
		{"America/New_York", "2025-01-02T01:00:00Z", "2025-01-01T20:00:00-0500"},
		{"America/New_York", "2025-07-02T01:00:00Z", "2025-07-01T21:00:00-0400"},
		{"UTC", "2025-01-02T01:00:00Z", "2025-01-02T01:00:00+0000"},
		{"UTC+5:45", "2025-01-02T01:00:00Z", "2025-01-02T06:45:00+0545"},
	} {
		t.Run(tc.zone+"/"+tc.timestamp, func(t *testing.T) {
			e := NewEngine()
			e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
			e.SetSymbolInfo("TEST", SymbolInfo{"timezone": tc.zone})
			code, err := e.Compile(`result = str.format_time(timestamp("` + tc.timestamp + `"))`)
			if err != nil {
				t.Fatal(err)
			}
			rt, _, err := e.ExecuteWithRuntime(code)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			if got, _ := rt.Value("result"); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContinuousSessionRetainsDailyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, spec, timeframe   string
		hour, barsPerDay, count int
	}{
		{"24x7_hourly", "24x7", "60", 0, 24, 48},
		{"midnight_hourly", "0000-0000", "60", 0, 24, 48},
		{"overnight_hourly", "1700-1700", "60", 17, 24, 48},
		{"24x7_daily", "24x7", "1D", 0, 1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := make([]float64, tc.count)
			e := NewEngine()
			p := providerWithClose("TEST", values...)
			p.timeframe = tc.timeframe
			e.RegisterMarketDataProvider(p)
			e.SetStartTime(time.Date(2025, 1, 1, tc.hour, 0, 0, 0, time.UTC))
			e.SetMarketSessions("TEST", MarketSessions{Regular: tc.spec})
			var got [][4]bool
			e.RegisterFunction("capture_session", func(args ...interface{}) (interface{}, error) {
				got = append(got, [4]bool{args[0].(bool), args[1].(bool), args[2].(bool), args[3].(bool)})
				return nil, nil
			})
			code, err := e.Compile(`capture_session(session.isfirstbar,session.islastbar,session.isfirstbar_regular,session.islastbar_regular)`)
			if err != nil {
				t.Fatal(err)
			}
			rt, _, err := e.ExecuteWithRuntime(code)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			if len(got) != tc.count {
				t.Fatalf("got %d observations, want %d", len(got), tc.count)
			}
			for bar, flags := range got {
				first, last := bar%tc.barsPerDay == 0, bar%tc.barsPerDay == tc.barsPerDay-1
				if flags != [4]bool{first, last, first, last} {
					t.Fatalf("bar %d: got %v, want %v", bar, flags, [4]bool{first, last, first, last})
				}
			}
		})
	}
}

func TestVWAPAnchorsBandsAndIsolation(t *testing.T) {
	e := NewEngine()
	p := &testProvider{data: map[string]SeriesExtended{
		"TEST|close": mkSeries(10, 20, 30), "TEST|high": mkSeries(10, 20, 30), "TEST|low": mkSeries(10, 20, 30), "TEST|volume": mkSeries(1, 1, 2),
	}, timeframe: "60"}
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	code, err := e.Compile(`[mean,upper,lower] = ta.vwap(close,bar_index == 0,2)
reset = ta.vwap(close,bar_index == 2)
daily = ta.vwap
result = mean
before = reset[1]`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	for name, want := range map[string]float64{"mean": 22.5, "daily": 22.5, "reset": 30, "upper": 22.5 + 2*math.Sqrt(68.75), "lower": 22.5 - 2*math.Sqrt(68.75)} {
		got, _ := rt.Value(name)
		n, ok := toFloat(got)
		if !ok || math.IsNaN(n) || math.Abs(n-want) > 1e-9 {
			t.Fatalf("%s: got %v,want %v", name, got, want)
		}
	}
	if got, _ := rt.Value("before"); !isNA(got) {
		t.Fatalf("anchor should not start early: %v", got)
	}
}

func TestBidAskAndMissingContext(t *testing.T) {
	e := NewEngine()
	p := &testProvider{data: map[string]SeriesExtended{"TEST|close": mkSeries(10), "TEST|bid": mkSeries(9), "TEST|ask": mkSeries(11)}, timeframe: "1T"}
	e.RegisterMarketDataProvider(p)
	code, _ := e.Compile(`result = ask - bid`)
	if got, err := e.Execute(code); err != nil || got != float64(2) {
		t.Fatalf("got %v,%v", got, err)
	}
	p.timeframe = "1"
	e.SetTimeframe("1")
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if got, _ := rt.Value("result"); !isNA(got) {
		t.Fatalf("bid/ask defined outside 1T: %v", got)
	}
	code, _ = e.Compile(`math.round_to_mintick(1)`)
	if _, err := e.Execute(code); err == nil || !strings.Contains(err.Error(), "mintick") {
		t.Fatal(err)
	}
}

func TestTickerIdentifiers(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{`ticker.new("NYSE","TEST")`, "NYSE:TEST"},
		{`ticker.new("NYSE","TEST",session.extended)`, `={"session":"extended","symbol":"NYSE:TEST"}`},
		{`ticker.modify(ticker.new("NYSE","TEST",session.extended),session.regular)`, "NYSE:TEST"},
		{`ticker.standard(ticker.heikinashi("NYSE:TEST"))`, "NYSE:TEST"},
		{`ticker.heikinashi(ticker.heikinashi("NYSE:TEST"))`, "NYSE:TEST;heikinashi"},
		{`ticker.inherit(ticker.new("NYSE","TEST",session.extended),"NASDAQ:OTHER")`, `={"session":"extended","symbol":"NASDAQ:OTHER"}`},
		{`ticker.modify("NYSE:TEST",session.regular,adjustment.splits,backadjustment.on,settlement_as_close.off)`, `={"adjustment":"splits","backadjustment":"default","settlement-as-close":false,"symbol":"NYSE:TEST"}`},
	} {
		got := completionValue(t, "result = "+tc.expr, 1)
		if got != tc.want {
			t.Fatalf("%s: got %v,want %v", tc.expr, got, tc.want)
		}
	}
}

func TestCalendarBucketsRestartAndClipAtBoundaries(t *testing.T) {
	for _, tc := range []struct{ date, tf, open, close string }{
		{"2025-01-01T12:00:00Z", "2D", "2025-01-01T00:00:00Z", "2025-01-03T00:00:00Z"},
		{"2025-12-31T12:00:00Z", "2D", "2025-12-31T00:00:00Z", "2026-01-01T00:00:00Z"},
		{"2025-12-31T12:00:00Z", "5M", "2025-11-01T00:00:00Z", "2026-01-01T00:00:00Z"},
		{"2025-01-10T12:00:00Z", "2W", "2025-01-06T00:00:00Z", "2025-01-20T00:00:00Z"},
	} {
		d, _ := time.Parse(time.RFC3339, tc.date)
		open, close, err := timeBucket(d, tc.tf, time.Time{})
		if err != nil || open.Format(time.RFC3339) != tc.open || close.Format(time.RFC3339) != tc.close {
			t.Fatalf("%+v: got %s,%s,%v", tc, open, close, err)
		}
	}
	if got := completionValue(t, `result = barstate.isfirst[1] ? 1 : 0`, 1, 2); got != float64(1) {
		t.Fatalf("historical barstate: %v", got)
	}
}

func TestVWAPDailyResetAndTentativeBar(t *testing.T) {
	e := NewEngine()
	p := &testProvider{data: map[string]SeriesExtended{
		"TEST|close": mkSeries(10, 20, 30), "TEST|high": mkSeries(10, 20, 30), "TEST|low": mkSeries(10, 20, 30), "TEST|volume": mkSeries(1, 1, 2),
	}, timeframe: "60"}
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC))
	code, _ := e.Compile(`result = ta.vwap(close)`)
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if got, _ := rt.Value("result"); math.Abs(got.(float64)-80.0/3) > 1e-9 {
		t.Fatal(got)
	}
	raw := []*Expr{{Kind: "ident", Name: "close"}}
	rt.SetBarIndex(0)
	v, err := rt.builtinVWAP(raw, []interface{}{10.0})
	if err != nil || v != 10.0 {
		t.Fatalf("%v %v", v, err)
	}
	v, err = rt.builtinVWAP(raw, []interface{}{20.0})
	if err != nil || v != 20.0 {
		t.Fatalf("tentative update accumulated twice: %v %v", v, err)
	}
}
