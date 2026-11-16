// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"testing"
)

type footprintTestSource struct {
	calls  []string
	levels []FootprintLevel
	err    error
}

func (s *footprintTestSource) GetFootprintLevels(symbol, tf string, bar BarTime) ([]FootprintLevel, error) {
	s.calls = append(s.calls, symbol+"/"+tf)
	return s.levels, s.err
}
func footprintFixture() []FootprintLevel {
	return []FootprintLevel{
		{Price: 10, BuyVolume: 1, SellVolume: 3}, {Price: 11, BuyVolume: 12, SellVolume: 1},
		{Price: 12, BuyVolume: 2, SellVolume: 2}, {Price: 13, BuyVolume: 3, SellVolume: 9},
		{Price: 14, BuyVolume: 1, SellVolume: 1},
	}
}
func TestFootprintAggregatesRowsAndValueArea(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 12))
	e.SetSymbolInfo("TEST", SymbolInfo{"mintick": 1.0})
	source := &footprintTestSource{levels: footprintFixture()}
	e.SetFootprintProvider(source)
	code, err := e.Compile(`fp = request.footprint(1,70,300)
poc = fp.poc()
val = fp.val()
vah = fp.vah()
selected = fp.get_row_by_price(11.5)
rows = fp.rows()
array.clear(rows)
rows_again = footprint.rows(fp)
total = fp.total_volume()
delta = fp.delta()
result = poc.down_price() + val.down_price() + vah.up_price() + selected.buy_volume()
buy_imbalance = selected.has_buy_imbalance()
outside = fp.get_row_by_price(99)
second = request.footprint(ticks_per_row = 2)`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	for name, want := range map[string]float64{"total": 35, "delta": 3, "result": 47} {
		got, _ := rt.Value(name)
		if n, ok := toFloat(got); !ok || math.IsNaN(n) || n != want {
			t.Fatalf("%s: %v,want %v", name, got, want)
		}
	}
	if got, _ := rt.Value("buy_imbalance"); got != true {
		t.Fatal(got)
	}
	if got, _ := rt.Value("outside"); !isNA(got) {
		t.Fatal(got)
	}
	if got, _ := rt.Value("rows_again"); len(got.(*pineArray).items) != 5 {
		t.Fatal("row list was mutated through returned array")
	}
	if len(source.calls) != 1 {
		t.Fatalf("raw levels fetched %d times", len(source.calls))
	}
}
func TestFootprintInsideRequestedContext(t *testing.T) {
	e, data := requestTestEngine(10, 20)
	context := data.contexts["OTHER/120"]
	context.Info["mintick"] = 1.0
	data.contexts["OTHER/120"] = context
	source := &footprintTestSource{levels: footprintFixture()}
	e.SetFootprintProvider(source)
	code, err := e.Compile(`result = request.security("OTHER","120",footprint.total_volume(request.footprint(1)))`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.Execute(code); err != nil || got != float64(35) {
		t.Fatalf("%v %v", got, err)
	}
	if len(source.calls) != 1 || source.calls[0] != "OTHER/120" {
		t.Fatalf("wrong footprint context: %v", source.calls)
	}
}
func TestFootprintAvailabilityValidationAndRowLimit(t *testing.T) {
	if got := completionValue(t, `result = request.footprint(1)`, 1); !isNA(got) {
		t.Fatal("missing order flow was fabricated")
	}
	fp, err := buildFootprint([]FootprintLevel{{Price: 0, BuyVolume: 1}, {Price: 2000, BuyVolume: 1}}, 1, 1, 70, 300, 0, 2001)
	if err != nil || fp != nil {
		t.Fatalf("row limit: %v %v", fp, err)
	}
	_, err = buildFootprint([]FootprintLevel{{Price: 1, BuyVolume: -1}}, 1, 1, 70, 300, 0, 2)
	if err == nil {
		t.Fatal("invalid source volumes accepted")
	}
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
	e.SetSymbolInfo("TEST", SymbolInfo{"mintick": 1.0})
	e.SetFootprintProvider(&footprintTestSource{err: fmt.Errorf("offline")})
	code, _ := e.Compile(`request.footprint(1)`)
	if _, err := e.Execute(code); err == nil {
		t.Fatal("provider error hidden")
	}
}
