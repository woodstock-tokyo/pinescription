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

type brokerCandle struct{ open, high, low, close float64 }

func strategyEngine(candles ...brokerCandle) *Engine {
	values := map[string][]float64{"open": {}, "high": {}, "low": {}, "close": {}}
	for _, c := range candles {
		values["open"] = append(values["open"], c.open)
		values["high"] = append(values["high"], c.high)
		values["low"] = append(values["low"], c.low)
		values["close"] = append(values["close"], c.close)
	}
	p := &testProvider{data: map[string]SeriesExtended{}, timeframe: "1D"}
	for vt, v := range values {
		p.data["TEST|"+vt] = mkSeries(v...)
	}
	e := NewEngine()
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	e.SetSymbolInfo("TEST", SymbolInfo{"mintick": 1.0, "pointvalue": 1.0, "mincontract": 1.0, "currency": "USD"})
	return e
}
func strategyRun(t *testing.T, e *Engine, script string) *Runtime {
	t.Helper()
	code, err := e.Compile(script)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Release)
	return rt
}
func assertStrategyNumber(t *testing.T, rt *Runtime, name string, want float64) {
	t.Helper()
	v, ok := rt.Value(name)
	if !ok {
		t.Fatalf("missing %s", name)
	}
	n, numeric := toFloat(v)
	if !numeric || math.IsNaN(n) || math.Abs(n-want) > 1e-9 {
		t.Fatalf("%s: got %v,want %v", name, v, want)
	}
}
func TestStrategyMarketTimingFeesAndTradeLedger(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 105, 95, 100}, brokerCandle{110, 115, 108, 112}, brokerCandle{120, 122, 118, 121})
	rt := strategyRun(t, e, `strategy("Timing",initial_capital=1000,margin_long=0,margin_short=0,slippage=1,commission_type=strategy.commission.cash_per_contract,commission_value=1)
if bar_index == 0
    strategy.entry("L",strategy.long,qty=2)
if bar_index == 1
    strategy.close("L")
net = strategy.netprofit
equity = strategy.equity
trades = strategy.closedtrades
entry = strategy.closedtrades.entry_price(0)
exit = strategy.closedtrades.exit_price(0)
entry_bar = strategy.closedtrades.entry_bar_index(0)
exit_bar = strategy.closedtrades.exit_bar_index(0)
fees = strategy.closedtrades.commission(0)
position = strategy.position_size
result = strategy.closedtrades.profit(0)`)
	for name, want := range map[string]float64{"net": 12, "equity": 1012, "trades": 1, "entry": 111, "exit": 119, "entry_bar": 1, "exit_bar": 2, "fees": 4, "position": 0, "result": 12} {
		assertStrategyNumber(t, rt, name, want)
	}
}
func TestStrategyBracketUsesOHLCPathAndOCO(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{105, 112, 89, 100})
	rt := strategyRun(t, e, `strategy("Bracket",initial_capital=1000,margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long)
    strategy.exit("X","L",limit=110,stop=90)
result = strategy.netprofit
trades = strategy.closedtrades
exit = strategy.closedtrades.exit_price(0)
position = strategy.position_size`)
	for name, want := range map[string]float64{"result": 5, "trades": 1, "exit": 110, "position": 0} {
		assertStrategyNumber(t, rt, name, want)
	}
	e = strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 111, 89, 100})
	rt = strategyRun(t, e, `strategy("Low first",margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long)
    strategy.exit("X","L",limit=110,stop=90)
result = strategy.netprofit`)
	assertStrategyNumber(t, rt, "result", -10)
}
func TestStrategyStopLimitActivationAndGapPrices(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 106, 99, 104}, brokerCandle{102, 104, 101, 103})
	rt := strategyRun(t, e, `strategy("Stop limit",margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long,stop=105,limit=103)
entry = strategy.opentrades.entry_price(0)
result = strategy.opentrades.entry_bar_index(0)`)
	assertStrategyNumber(t, rt, "entry", 102)
	assertStrategyNumber(t, rt, "result", 2)
	e = strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{110, 112, 108, 111})
	rt = strategyRun(t, e, `strategy("Gap",margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long,stop=105)
result = strategy.position_avg_price`)
	assertStrategyNumber(t, rt, "result", 110)
}
func TestStrategyOCAAndCancellation(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 106, 94, 100})
	rt := strategyRun(t, e, `strategy("OCA",margin_long=0,margin_short=0)
if bar_index == 0
    strategy.order("up",strategy.long,qty=1,stop=105,oca_name="g",oca_type=strategy.oca.cancel)
    strategy.order("down",strategy.short,qty=1,stop=95,oca_name="g",oca_type=strategy.oca.cancel)
    strategy.entry("cancelled",strategy.long,limit=99)
    strategy.cancel("cancelled")
result = strategy.position_size
trades = strategy.opentrades`)
	assertStrategyNumber(t, rt, "result", -1)
	assertStrategyNumber(t, rt, "trades", 1)
}
func TestStrategyPyramidingReversalAndExitAcrossLots(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 101, 99, 100}, brokerCandle{100, 101, 99, 100}, brokerCandle{100, 101, 99, 100})
	rt := strategyRun(t, e, `strategy("Reverse",margin_long=0,margin_short=0,pyramiding=2)
if bar_index == 0
    strategy.entry("one",strategy.long)
if bar_index == 1
    strategy.entry("two",strategy.long)
if bar_index == 2
    strategy.entry("reverse",strategy.short,qty=3)
result = strategy.position_size
closed = strategy.closedtrades`)
	assertStrategyNumber(t, rt, "result", -3)
	assertStrategyNumber(t, rt, "closed", 2)
	e = strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 101, 99, 100}, brokerCandle{100, 101, 99, 100}, brokerCandle{100, 110, 99, 105})
	rt = strategyRun(t, e, `strategy("Exit lots",margin_long=0,margin_short=0,pyramiding=2)
if bar_index < 2
    strategy.entry("L",strategy.long)
if bar_index == 2
    strategy.exit("X","L",limit=105)
result = strategy.position_size
closed = strategy.closedtrades`)
	assertStrategyNumber(t, rt, "result", 0)
	assertStrategyNumber(t, rt, "closed", 2)
}
func TestStrategyExplicitLimitsAndHooks(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100})
	for _, tc := range []struct{ script, want string }{
		{`strategy("Margin")`, "margin liquidation"},
		{`strategy("Recalc",margin_long=0,margin_short=0,calc_on_order_fills=true)`, "calc_on_order_fills"},
		{`strategy("Trailing",margin_long=0,margin_short=0)
strategy.exit("X",trail_points=10,trail_offset=2)`, "trail_points"},
	} {
		code, err := e.Compile(tc.script)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Execute(code); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("got %v,want %s", err, tc.want)
		}
	}
	e.RegisterFunction("strategy.entry", func(...interface{}) (interface{}, error) { return 9.0, nil })
	code, _ := e.Compile(`strategy.entry("custom",strategy.long)`)
	if got, err := e.Execute(code); err != nil || got != float64(9) {
		t.Fatalf("hook precedence: %v %v", got, err)
	}
}

func TestStrategyCloseProcessingSizingAndBasicRisk(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{200, 210, 190, 205})
	rt := strategyRun(t, e, `strategy("Sizing",margin_long=0,margin_short=0,default_qty_type=strategy.cash,default_qty_value=500)
if bar_index == 0
    strategy.entry("L",strategy.long)
result = strategy.position_size
sizing = strategy.default_entry_qty(200)`)
	assertStrategyNumber(t, rt, "result", 2)
	assertStrategyNumber(t, rt, "sizing", 2)
	e = strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 110, 95, 105})
	rt = strategyRun(t, e, `strategy("Immediate",margin_long=0,margin_short=0)
if bar_index == 0
    strategy.entry("L",strategy.long)
if bar_index == 1
    strategy.close_all(immediately=true)
result = strategy.netprofit
position = strategy.position_size`)
	assertStrategyNumber(t, rt, "result", 5)
	assertStrategyNumber(t, rt, "position", 0)
	e = strategyEngine(brokerCandle{100, 100, 100, 100}, brokerCandle{100, 105, 95, 100})
	rt = strategyRun(t, e, `strategy("Risk",margin_long=0,margin_short=0)
strategy.risk.max_position_size(2)
if bar_index == 0
    strategy.entry("L",strategy.long,qty=5)
result = strategy.position_size`)
	assertStrategyNumber(t, rt, "result", 2)
}

func TestStrategyProcessOrdersOnCloseIsPostEvaluation(t *testing.T) {
	e := strategyEngine(brokerCandle{100, 100, 100, 100})
	rt := strategyRun(t, e, `strategy("Close fills",margin_long=0,margin_short=0,process_orders_on_close=true)
strategy.entry("L",strategy.long)
during = strategy.position_size`)
	assertStrategyNumber(t, rt, "during", 0)
	if position, ok := rt.strategyIdentifier("strategy.position_size"); !ok || position != float64(1) {
		t.Fatalf("close processing did not fill: %v", position)
	}
}
