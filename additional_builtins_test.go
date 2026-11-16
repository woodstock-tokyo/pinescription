// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestAdditionalArrayAPIs(t *testing.T) {
	for _, tc := range []struct {
		script string
		want   float64
	}{
		{`a = array.from(1,2,3,4)
array.fill(a, 9, 1, 3)
a.reverse()
result = a.get(0)*1000+a.get(1)*100+a.get(2)*10+a.get(3)`, 4991},
		{`a = array.from(3,1,2)
indices = a.sort_indices()
a.sort()
result = a.get(0)*100+indices.get(0)*10+a.binary_search(2)`, 111},
		{`a = array.from(3,1,2)
a.sort(order.descending)
result = a.get(0)*10+a.get(2)`, 31},
		{`a = array.from("c","a","b")
array.sort(a)
result = a.get(0) == "a" ? 1 : 0`, 1},
		{`a = array.from(1,2,3)
result = array.variance(a,false)`, 1},
		{`a = array.from(1,na,3)
result = array.stdev(a)`, 1},
		{`a = array.from(1,2,3)
z = array.standardize(a)
result = z.get(1)`, 0},
		{`a = array.from(5,5)
z = a.standardize()
result = z.get(0)`, 1},
		{`a = array.from(false,true)
result = a.some() ? 1 : 0`, 1},
		{`a = array.new<line>(2)
b = array.new_linefill(3)
c = array.new_color(4)
d = array.new<table>(1)
result = a.size()+b.size()+c.size()+d.size()`, 10},
		{`a = map.new()
b = map.new()
a.put("x",1)
b.put("x",2)
b.put("y",3)
a.put_all(b)
result = a.get("x")+a.get("y")`, 5},
	} {
		t.Run(tc.script, func(t *testing.T) {
			got := completionValue(t, tc.script, 1)
			n, ok := toFloat(got)
			if !ok || math.IsNaN(n) || math.Abs(n-tc.want) > 1e-9 {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdditionalStringAPIs(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want interface{}
	}{
		{`str.tonumber(" -12.5 ")`, float64(-12.5)},
		{`str.trim("  hi\n")`, "hi"},
		{`str.repeat("ab",3,"-")`, "ab-ab-ab"},
		{`str.repeat("ab",0)`, ""},
		{`str.replace_all("aaaa","aa","X")`, "XX"},
		{`str.pos("aéz","z")`, 2},
		{`str.match("abc123def","[0-9]+")`, "123"},
		{`str.match("abc","[0-9]+")`, ""},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got := completionValue(t, "result = "+tc.expr, 1)
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
	if !isNA(completionValue(t, `result = str.tonumber("1e3")`, 1)) {
		t.Fatal("exponent is not a Pine decimal string")
	}
	if !isNA(completionValue(t, `result = str.pos("abc","x")`, 1)) {
		t.Fatal("missing target must return na")
	}
}

func TestVolumeIndicators(t *testing.T) {
	p := &testProvider{data: map[string]SeriesExtended{
		"TEST|open": mkSeries(9, 10, 12), "TEST|high": mkSeries(12, 14, 14),
		"TEST|low": mkSeries(8, 10, 10), "TEST|close": mkSeries(10, 12, 11),
		"TEST|volume": mkSeries(100, 200, 50),
	}, timeframe: "1D"}
	// Calculated from the definitions, independently of PineTS snapshots.
	for name, want := range map[string]float64{
		"ta.accdist": -25, "ta.iii": -25, "ta.obv": 150, "ta.pvt": 35.833333333333336,
		"ta.pvi": 1.2, "ta.nvi": 11.0 / 12, "ta.wad": -1, "ta.wvad": -12.5,
	} {
		t.Run(name, func(t *testing.T) {
			e := NewEngine()
			e.RegisterMarketDataProvider(p)
			code, err := e.Compile("result = " + name + "\nprevious = " + name + "[1]")
			if err != nil {
				t.Fatal(err)
			}
			rt, _, err := e.ExecuteWithRuntime(code)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			got, _ := rt.Value("result")
			if n, _ := toFloat(got); math.IsNaN(n) || math.Abs(n-want) > 1e-9 {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
	if !isNA(completionValue(t, "result = ta.iii", 1, 2)) {
		t.Fatal("missing volume must not fall back to close")
	}
}

func TestRCIAndRuntimeError(t *testing.T) {
	for _, tc := range []struct {
		values []float64
		want   float64
	}{
		{[]float64{0, 1, 2, 3}, 100},
		{[]float64{4, 3, 2, 1}, -100},
		{[]float64{0, 1, 1, 2}, math.Sqrt(3) / 2 * 100},
	} {
		got := completionValue(t, "result = ta.rci(close,3)", tc.values...)
		if n, _ := toFloat(got); math.IsNaN(n) || math.Abs(n-tc.want) > 1e-9 {
			t.Fatalf("got %v,want %v", got, tc.want)
		}
	}
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
	code, _ := e.Compile(`runtime.error("stop")`)
	_, err := e.Execute(code)
	if err == nil || !strings.Contains(err.Error(), "bar 0 (TEST): stop") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTimestampAndTimeframeCorrections(t *testing.T) {
	if got := completionValue(t, `result = timestamp("2025-01-01T00:00:00Z")`, 1); got != float64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()) {
		t.Fatal(got)
	}
	for sec, want := range map[int]string{2: "5S", 16: "30S", 31: "1", 61: "2", 86401: "2D"} {
		if got := timeframeFromSeconds(sec); got != want {
			t.Fatalf("%d: got %s want %s", sec, got, want)
		}
	}
}
