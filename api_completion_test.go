// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func completionValue(t *testing.T, script string, values ...float64) interface{} {
	t.Helper()
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", values...))
	e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	b, err := e.Compile(script)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(b)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	v, ok := rt.Value("result")
	if !ok {
		t.Fatal("result not assigned")
	}
	return v
}

func TestCompletedStringAPIs(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{`str.replace("aaaa", "aa", "X")`, "Xaa"},
		{`str.replace("aaaa", "aa", "X", 1)`, "aaX"},
		{`str.replace("aaaa", "aa", "X", 2)`, "aaaa"},
		{`str.replace("ababa", "aba", "X", 1)`, "ababa"},
		{`str.replace("éééé", "éé", "X", 1)`, "ééX"},
		{`str.replace("abc", "", "X", 2)`, "abXc"},
		{`str.replace("éé", "", "X", 1)`, "éXé"},
		{`str.replace("abc", "z", "X", 0)`, "abc"},
		{`str.tostring(1.25, "0.0")`, "1.3"},
		{`str.tostring(-1.25, "0.0")`, "-1.3"},
		{`str.tostring(1234.5, "#,##0.00")`, "1,234.50"},
		{`str.tostring(12.5, format.percent)`, "12.50%"},
		{`str.tostring(1250000, format.volume)`, "1.25M"},
		{`str.tostring(na)`, "NaN"},
		{`str.format("{0,number,integer}", 1234.5)`, "1,234"},
		{`str.format("{0,number,percent}", 0.125)`, "12%"},
		{`str.format("{0,number,currency}", 1234.5)`, "$1,234.50"},
		{`str.format("'{0}' {0,number,0.00}", 1.125)`, "{0} 1.12"},
		{`str.format("{0,date,yyyy-MM-dd} {0,time,HH:mm:ss}", 0)`, "1970-01-01 00:00:00"},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got := completionValue(t, "result = "+tc.expr, 1)
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFixnanCarriesValuesAndIsolatesCalls(t *testing.T) {
	got := completionValue(t, `
a = fixnan(close > 1 ? na : close)
b = fixnan(close > 2 ? na : close * 10)
result = a + b`, 1, 2, 3)
	if got != float64(21) {
		t.Fatalf("got %v, want 21", got)
	}
	got = completionValue(t, "result = fixnan(close)", math.NaN(), 5, math.NaN())
	if got != float64(5) {
		t.Fatalf("got %v, want 5", got)
	}
	got = completionValue(t, `hold(x) => fixnan(x)
a = hold(close > 1 ? na : close)
b = hold(close > 2 ? na : close * 10)
result = a + b`, 1, 2, 3)
	if got != float64(21) {
		t.Fatalf("function call-site isolation: got %v", got)
	}
}

func TestTimeCompletion(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("TEST", 1, 2, 3, 4)
	p.timeframe = "30"
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC))
	var observed []interface{}
	e.RegisterFunction("capture", func(args ...interface{}) (interface{}, error) {
		observed = append(observed, args[0])
		return nil, nil
	})
	b, err := e.Compile(`capture(time("60", "0930-1030:4", "UTC"))
result = time("60")`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(b)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if !isNA(observed[0]) || !isNA(observed[3]) {
		t.Fatalf("out-of-session bars must be na: %v", observed)
	}
	if got := observed[1]; got != float64(time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC).UnixMilli()) {
		t.Fatalf("session-aligned opening: %v", got)
	}
	for _, tf := range []string{"15S", "100T"} {
		v, _ := timeframeIdentifierValue("timeframe.isintraday", tf)
		ticks, _ := timeframeIdentifierValue("timeframe.isticks", tf)
		if !v.(bool) || ticks.(bool) != strings.HasSuffix(tf, "T") {
			t.Fatalf("incorrect timeframe classification for %s", tf)
		}
	}
	if !reflect.DeepEqual(observed[1], observed[2]) {
		t.Fatal("bars inside the same requested bucket must share its opening")
	}
}
