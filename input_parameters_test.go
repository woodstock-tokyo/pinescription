// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"strings"
	"testing"
)

type testInputResolver func([]InputParameter) (map[string]interface{}, error)

func (f testInputResolver) ResolveInputs(p []InputParameter) (map[string]interface{}, error) {
	return f(p)
}

func TestInputsAreDiscoveredAndResolvedBeforeRuntime(t *testing.T) {
	e := NewEngine()
	code, err := e.Compile(`enum Side
    buy = "Buy"
    sell = "Sell"
price = input.price(10, "Price")
note = input.text_area("hello", title = "Notes")
tf = input.timeframe("60", options = ["30", "60"])
side = input.enum(Side.buy)
src = input.source(close)
length = input.int(2, minval = 1, maxval = 10)
result = price + src + length + (side == Side.sell ? 100 : 0)`)
	if err != nil {
		t.Fatal(err)
	}
	defs, err := e.InputParameters(code) // No provider exists at discovery time.
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 6 || defs[0].ID != "price" || defs[3].Options[1].(EnumValue).Title != "Sell" {
		t.Fatalf("unexpected definitions: %+v", defs)
	}
	p := &testProvider{data: map[string]SeriesExtended{"TEST|close": mkSeries(1, 2, 3), "TEST|high": mkSeries(4, 5, 6)}}
	e.RegisterMarketDataProvider(p)
	calls := 0
	e.SetInputResolver(testInputResolver(func(parameters []InputParameter) (map[string]interface{}, error) {
		calls++
		if len(p.setCalls) != 0 {
			t.Fatal("provider called before parameter resolution")
		}
		return map[string]interface{}{"price": 20.0, "note": "world", "tf": "30", "side": "sell", "src": InputSource("high"), "length": 3}, nil
	}))
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if calls != 1 {
		t.Fatalf("resolver called %d times, want once per execution", calls)
	}
	if got, _ := rt.Value("result"); got != float64(129) {
		t.Fatalf("got %v", got)
	}
	if got, _ := rt.Value("note"); got != "world" {
		t.Fatal(got)
	}
	if got, _ := rt.Value("tf"); got != "30" {
		t.Fatal(got)
	}
	// Cached bytecode/program must not be modified by resolved choices.
	defs, err = e.InputParameters(code)
	if err != nil || defs[0].Default != float64(10) {
		t.Fatalf("defaults mutated: %+v %v", defs, err)
	}
}

func TestInputValidationPrecedesProviderAndScript(t *testing.T) {
	for _, tc := range []struct {
		script    string
		overrides map[string]interface{}
		want      string
	}{
		{`x = input.int(2,minval = 1,maxval = 3)`, map[string]interface{}{"x": 4}, "invalid value"},
		{`x = input.float(2)`, map[string]interface{}{"x": true}, "invalid value"},
		{`x = input.timeframe("60",options = ["60","D"])`, map[string]interface{}{"x": "30"}, "invalid value"},
		{`x = input.bool(false)`, map[string]interface{}{"missing": true}, "unknown input"},
		{`x = input.int(close)`, nil, "must be constant"},
	} {
		t.Run(tc.script, func(t *testing.T) {
			e := NewEngine()
			code, err := e.Compile(tc.script)
			if err != nil {
				t.Fatal(err)
			}
			e.SetInputParameters(tc.overrides)
			_, err = e.Execute(code) // Invalid inputs fail before "no provider".
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v,want %s", err, tc.want)
			}
		})
	}
	e := NewEngine()
	code, _ := e.Compile(`x = input.price(1)`)
	e.SetInputResolver(testInputResolver(func([]InputParameter) (map[string]interface{}, error) { return nil, fmt.Errorf("cancelled") }))
	if _, err := e.Execute(code); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatal(err)
	}
}
