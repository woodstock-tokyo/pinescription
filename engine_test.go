// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	series "github.com/woodstock-tokyo/pinescription/series"
)

type testProvider struct {
	data       map[string]SeriesExtended
	valueTypes []string
	timeframe  string
	session    string
	setCalls   []string
}

func (p *testProvider) GetSeries(seriesKey string) (SeriesExtended, error) {
	return p.data[seriesKey], nil
}

func (p *testProvider) GetSymbols() ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(p.data))
	for key := range p.data {
		symbol, _, ok := splitSeriesKey(key)
		if !ok {
			symbol = key
		}
		if symbol == "" || seen[symbol] {
			continue
		}
		seen[symbol] = true
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out, nil
}

func (p *testProvider) GetValuesTypes() ([]string, error) {
	if len(p.valueTypes) > 0 {
		out := append([]string{}, p.valueTypes...)
		sort.Strings(out)
		return out, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for key := range p.data {
		_, valueType, ok := splitSeriesKey(key)
		if !ok {
			valueType = "close"
		}
		if valueType == "" || seen[valueType] {
			continue
		}
		seen[valueType] = true
		out = append(out, valueType)
	}
	if len(out) == 0 {
		out = append(out, "close")
	}
	sort.Strings(out)
	return out, nil
}

func (p *testProvider) SetTimeframe(timeframe string) error {
	p.timeframe = timeframe
	p.setCalls = append(p.setCalls, timeframe)
	return nil
}

func (p *testProvider) GetTimeframe() string {
	if strings.TrimSpace(p.timeframe) == "" {
		return "1D"
	}
	return p.timeframe
}

func (p *testProvider) SetSession(session string) error {
	p.session = session
	return nil
}

func (p *testProvider) GetSession() string {
	if strings.TrimSpace(p.session) == "" {
		return "regular"
	}
	return p.session
}

func providerWithValueType(symbol, valueType string, values ...float64) *testProvider {
	return &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey(symbol, valueType): mkSeries(values...),
		},
		valueTypes: []string{valueType},
		timeframe:  "1D",
		session:    "regular",
	}
}

func providerWithClose(symbol string, values ...float64) *testProvider {
	return providerWithValueType(symbol, "close", values...)
}

func mkSeries(values ...float64) SeriesExtended {
	q := series.NewQueue(len(values) + 8)
	for _, v := range values {
		q.Update(v)
	}
	return q
}

func compileExec(t *testing.T, script string, values ...float64) interface{} {
	t.Helper()
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", values...))
	e.SetDefaultSymbol("TEST")
	b, err := e.Compile(script)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	return v
}

func TestNamedArgsScriptFunctionOutOfOrder(t *testing.T) {
	v := compileExec(t, "foo(b = 2, a = 5)\nfoo(a, b) => a * 10 + b", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 52 {
		t.Fatalf("expected 52, got %v", got)
	}
}

func TestNamedArgsTypeConstructorUsesDefaultsForSparseArgs(t *testing.T) {
	v := compileExec(t, "type Pair\n    float left = 3\n    float right = 7\np = Pair.new(right = 11)\np.left + p.right", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 14 {
		t.Fatalf("expected 14, got %v", got)
	}
}

func TestNamedArgsBuiltinBindingForInputsAndBox(t *testing.T) {
	v := compileExec(t, "x = input.int(20, 'len', group = 'grp', minval = 1)\nb = box.new(1, 2, 3, 4, bgcolor = color.new(color.red, 25))\nx + box.get_bottom(b)", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 24 {
		t.Fatalf("expected 24, got %v", got)
	}
}

func TestNamedArgsInputDefaultWhenDefvalOmitted(t *testing.T) {
	v := compileExec(t, "input.int(title = 'x', minval = 1)", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 0 {
		t.Fatalf("expected default 0, got %v", got)
	}
}

func TestNamedArgsBuiltinRejectsMissingRequiredPrefixParam(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")

	for _, tc := range []struct {
		script string
		want   string
	}{
		{script: "color.new(transp = 25)", want: `missing required argument "color" for color.new`},
		{script: "box.new(bottom = 4)", want: `missing required argument "left" for box.new`},
	} {
		b, err := e.Compile(tc.script)
		if err != nil {
			t.Fatalf("compile failed for %q: %v", tc.script, err)
		}
		_, err = e.Execute(b)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected runtime error containing %q for %q, got %v", tc.want, tc.script, err)
		}
	}
}

func TestNamedArgsBuiltinBindingForBarcolor(t *testing.T) {
	v := compileExec(t, "barcolor(color.green, title = 'Volume Weighted Colored Bars', editable = false)\n1", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestNamedArgsBuiltinBindingForIndicatorAndTableCell(t *testing.T) {
	v := compileExec(t, "indicator('Named Args', 'NA', true, max_bars_back = 5000, max_boxes_count = 500)\nt = table.new(position.bottom_right, 1, 1)\ntable.cell(t, 0, 0, 'x', text_size = size.normal, text_color = color.teal, tooltip = 'ok')\n1", 1, 2, 3)
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("expected float64 result, got %T (%v)", v, v)
	}
	if got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestArithmeticAndVariables(t *testing.T) {
	script := `
var x = 1
x = x + 2 * 3
x
`
	v := compileExec(t, script, 10, 11, 12)
	f, ok := v.(float64)
	if !ok || f != 7 {
		t.Fatalf("expected 7, got %#v", v)
	}
}

func TestIfElse(t *testing.T) {
	script := `
var out = 0
if close > 10
    out = 5
else
    out = 2
out
`
	v := compileExec(t, script, 9, 10, 11)
	f := v.(float64)
	if f != 5 {
		t.Fatalf("expected 5, got %v", f)
	}
}

func TestForAndWhile(t *testing.T) {
	script := `
var s = 0
for i = 1 to 4
    s = s + i
var k = 0
while k < 2
    s = s + 1
    k = k + 1
s
`
	v := compileExec(t, script, 1, 2)
	f := v.(float64)
	if f != 12 {
		t.Fatalf("expected 12, got %v", f)
	}
}

func TestSwitchStatementWithValue(t *testing.T) {
	script := `
var out = 0
switch close
    1 => out = 10
    2 => out = 20
    => out = 99
out
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if f != 99 {
		t.Fatalf("expected 99, got %v", f)
	}
}

func TestSwitchStatementWithConditions(t *testing.T) {
	script := `
var out = 0
switch
    close > 10 => out = 1
    close > 5 => out = 2
    => out = 3
out
`
	v := compileExec(t, script, 2, 4, 7)
	f := v.(float64)
	if f != 2 {
		t.Fatalf("expected 2, got %v", f)
	}
}

func TestFunctionsArraysTuplesAndCasts(t *testing.T) {
	script := `
sum2(a, b) => a + b
var arr = array.new_int(3, 1)
arr = array.set(arr, 1, 9)
var x = array.get(arr, 1)
var n = int(2.8)
var tup = (x, n)
sum2(float(x), float(n))
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if f != 11 {
		t.Fatalf("expected 11, got %v", f)
	}
}

func TestTypedDeclarationsIncludeMapPlotAndHline(t *testing.T) {
	script := `
int i = 2
float f = 3.5
bool b = true
string s = "x"
array<int> arr = array.new_int(2, 1)
arr = array.set(arr, 1, 5)
matrix<float> m = matrix.new(1, 1, 4)
map<string, float> mp = map.new()
mp = map.put(mp, "k", 7)
plot p = na
hline h = na
var got = map.get(mp, "k")
var has = map.contains(mp, "k") ? 1 : 0
var sz = map.size(mp)
i + f + (b ? 1 : 0) + str.length(s) + array.get(arr, 1) + matrix.get(m, 0, 0) + got + has + sz
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if f != 25.5 {
		t.Fatalf("expected 25.5, got %v", f)
	}
}

func TestBoolCannotBeNA(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")
	_, err := e.Compile("bool b = na")
	if err != nil {
	}
	b, err2 := e.Compile("bool b = na\nb")
	if err2 != nil {
		t.Fatalf("compile failed: %v", err2)
	}
	_, execErr := e.Execute(b)
	if execErr == nil {
		t.Fatalf("expected bool value cannot be na error")
	}
}

func TestNaNzFixnanRejectBoolArguments(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")

	for _, script := range []string{
		"na(true)",
		"nz(true)",
		"nz(true, 1)",
		"nz(1, true)",
		"fixnan(true)",
	} {
		b, err := e.Compile(script)
		if err != nil {
			continue
		}
		_, execErr := e.Execute(b)
		if execErr == nil {
			t.Fatalf("expected runtime error for %q", script)
		}
	}
}

func TestBoolHistoryIndexOutOfRangeIsFalse(t *testing.T) {
	script := `
bool b = close > 0
int x = b[1] ? 1 : 0
x
`
	v := compileExec(t, script, 10)
	if v.(float64) != 0 {
		t.Fatalf("expected 0, got %v", v)
	}
}

func TestCompileRejectsNumericToBoolAutoConversion(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")

	for _, script := range []string{
		"bool b = 1\nb",
		"bool b = close\nb",
		"var x = 1\nbool b = x\nb",
		"bool b = true\nb := float(1)\nb",
		"if 1\n    1\n1",
		"var x = 1 ? 2 : 3\nx",
		"var x = 1 and true\nx ? 1 : 0",
	} {
		_, err := e.Compile(script)
		if err == nil {
			t.Fatalf("expected compile error for %q", script)
		}
		if !strings.Contains(err.Error(), "int/float") {
			t.Fatalf("expected int/float error for %q, got %v", script, err)
		}
	}
}

func TestExplicitBoolCastWorksForBoolAssignmentsAndConditions(t *testing.T) {
	script := `
var as_bool = bool(1)
bool b = as_bool
if bool(0)
    b := false
b ? 1 : 0
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 1 {
		t.Fatalf("expected 1, got %v", v)
	}
}

func TestMapBuiltinsKeysValuesCopyAndClear(t *testing.T) {
	script := `
map<string, float> mp = map.new()
mp = map.put(mp, "b", 2)
mp = map.put(mp, "a", 1)
var keys = map.keys(mp)
var vals = map.values(mp)
var total = array.get(vals, 0) + array.get(vals, 1)
var hasA = map.contains(mp, "a") ? 1 : 0
var cp = map.copy(mp)
cp = map.remove(cp, "a")
var cpSz = map.size(cp)
mp = map.clear(mp)
var sz = map.size(mp)
var firstKeyIsA = array.get(keys, 0) == "a" ? 1 : 0
total + hasA + cpSz + sz + firstKeyIsA
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if f != 6 {
		t.Fatalf("expected 6, got %v", f)
	}
}

func TestOperatorAssignmentVariantsAndUnaryPlus(t *testing.T) {
	script := `
var x = 10
x := x + 1
x += 2
x -= 3
x *= 4
x /= 2
x %= 5
var y = +x
var z = not false and true or false ? 1 : 0
x + y + z
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 1 {
		t.Fatalf("expected 1, got %v", v)
	}
}

func TestKeywordQualifiersVaripSimpleSeriesInput(t *testing.T) {
	script := `
varip a = 1
simple float b = 2
series int c = 3
input int d = 4
a + b + c + d
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 10 {
		t.Fatalf("expected 10, got %v", v)
	}
}

func TestRecognizedButUnsupportedKeywords(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.SetDefaultSymbol("AAA")

	_, err := e.Compile("import foo")
	if err == nil || !strings.Contains(err.Error(), "recognized but not supported") {
		t.Fatalf("expected unsupported keyword error for import, got %v", err)
	}

	_, err = e.Compile("do")
	if err == nil || !strings.Contains(err.Error(), "recognized but not supported") {
		t.Fatalf("expected unsupported keyword error for do, got %v", err)
	}
}

func TestMethodDeclarationsAndAttachmentCalls(t *testing.T) {
	script := `
method add(float self, float delta) =>
    self + delta

method second(array<int> arr) =>
    array.get(arr, 1)

var x = 2
var arr = array.new_int(2, 1)
arr = array.set(arr, 1, 9)
x.add(3) + arr.second()
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 14 {
		t.Fatalf("expected 14, got %v", v)
	}
}

func TestCustomTypeDefinitionConstructorAndFields(t *testing.T) {
	script := `
type Candle
    float o
    float c = 5

method spread(Candle self) =>
    self.c - self.o

Candle x = Candle.new(2)
x.c := 8
x.spread()
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 6 {
		t.Fatalf("expected 6, got %v", v)
	}
}

func TestCustomTypeDefaultFieldValues(t *testing.T) {
	script := `
type Pair
    float a = 1
    float b = 2

var p = Pair.new()
p.a + p.b
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 3 {
		t.Fatalf("expected 3, got %v", v)
	}
}

func TestBooleanNotFalseAndTrue(t *testing.T) {
	v := compileExec(t, `not false and true ? 1 : 0`, 1, 2, 3)
	if v.(float64) != 1 {
		t.Fatalf("expected 1, got %v", v)
	}
}

func TestTupleDestructureDiscardAndTypedFunctionParams(t *testing.T) {
	script := `
sumDiff(float val1, float val2) =>
    [val1 + val2, val1 - val2]

[_, diff] = sumDiff(5, 2)
diff
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if f != 3 {
		t.Fatalf("expected 3, got %v", f)
	}
}

func TestTupleDestructureWithMathRandom(t *testing.T) {
	script := `
sumDiff(float val1, float val2) =>
    [val1 + val2, val1 - val2]

[_, diff] = sumDiff(math.random(), math.random())
diff
`
	v := compileExec(t, script, 1, 2, 3)
	f := v.(float64)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		t.Fatalf("expected finite random diff, got %v", f)
	}
	if f < -1 || f > 1 {
		t.Fatalf("expected random diff in [-1,1], got %v", f)
	}
}

func TestBuiltInIndicators(t *testing.T) {
	script := `
var a = sma(close, 3)
var b = ema(close, 3)
var c = rsi(close, 3)
a + b + c
`
	v := compileExec(t, script, 10, 11, 12, 13, 14, 15)
	f := v.(float64)
	if math.IsNaN(f) {
		t.Fatalf("expected indicator sum numeric, got NaN")
	}
}

func TestRegisterUserFunction(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")
	e.RegisterFunction("double", func(args ...interface{}) (interface{}, error) {
		f, _ := toFloat(args[0])
		return f * 2, nil
	})
	b, err := e.Compile("double(21)")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 42 {
		t.Fatalf("expected 42, got %v", v)
	}
}

func TestRegisterFunctionCallableFromInputPineScript(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 10, 20, 30))
	e.SetDefaultSymbol("TEST")

	calls := 0
	var lastArgs []float64
	e.RegisterFunction("strategy.order", func(args ...any) (any, error) {
		calls++
		if len(args) != 2 {
			return nil, fmt.Errorf("strategy.order expected 2 args, got %d", len(args))
		}
		base, ok := toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("strategy.order first arg is not numeric: %T", args[0])
		}
		offset, ok := toFloat(args[1])
		if !ok {
			return nil, fmt.Errorf("strategy.order second arg is not numeric: %T", args[1])
		}
		lastArgs = []float64{base, offset}
		return base + offset + 5, nil
	})

	b, err := e.Compile(`
base = close + 2
strategy.order(base, 7)
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if calls == 0 {
		t.Fatalf("expected registered function to be called from PineScript")
	}
	if len(lastArgs) != 2 || lastArgs[0] != 32 || lastArgs[1] != 7 {
		t.Fatalf("expected final PineScript args [32 7], got %v", lastArgs)
	}
	if v.(float64) != 44 {
		t.Fatalf("expected 44, got %v", v)
	}
}

func TestRegisterDottedFunctionCallableWithNamedArgs(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 10, 20, 30))
	e.SetDefaultSymbol("TEST")

	calls := 0
	var lastArgs []float64
	e.RegisterFunctionWithParamNames("request.security", []string{"source", "offset"}, func(args ...any) (any, error) {
		calls++
		if len(args) != 2 {
			return nil, fmt.Errorf("request.security expected 2 args, got %d", len(args))
		}
		seriesValue, ok := toFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("request.security first arg is not numeric: %T", args[0])
		}
		offset, ok := toFloat(args[1])
		if !ok {
			return nil, fmt.Errorf("request.security second arg is not numeric: %T", args[1])
		}
		lastArgs = []float64{seriesValue, offset}
		return seriesValue + offset, nil
	})

	b, err := e.Compile(`
base = close + 2
request.security(offset = 7, source = base)
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if calls == 0 {
		t.Fatalf("expected registered dotted function to be called from PineScript")
	}
	if len(lastArgs) != 2 || lastArgs[0] != 32 || lastArgs[1] != 7 {
		t.Fatalf("expected final PineScript args [32 7], got %v", lastArgs)
	}
	if v.(float64) != 39 {
		t.Fatalf("expected 39, got %v", v)
	}
}

func TestRegisteredFunctionNamedArgsRequireParamNames(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 10, 20, 30))
	e.SetDefaultSymbol("TEST")
	e.RegisterFunction("request.security", func(args ...any) (any, error) {
		return nil, fmt.Errorf("registered hook should not receive silently misbound named args: %v", args)
	})

	b, err := e.Compile(`
base = close + 2
request.security(offset = 7, source = base)
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err == nil || !strings.Contains(err.Error(), "named arguments are not supported for registered function request.security without parameter metadata") {
		t.Fatalf("expected missing parameter metadata error, got %v", err)
	}
}

func TestUnsupportedFeatures(t *testing.T) {
	tests := []string{
		"strategy.entry(\"L\", strategy.long)",
		"plot(close)",
		"request.security(\"TEST\", \"D\", close)",
	}
	for _, script := range tests {
		e := NewEngine()
		e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
		e.SetDefaultSymbol("TEST")
		b, err := e.Compile(script)
		if err != nil {
			t.Fatalf("compile failed for %q: %v", script, err)
		}
		_, err = e.Execute(b)
		if err == nil || !strings.Contains(err.Error(), "unsupported feature") {
			t.Fatalf("expected unsupported feature error for %q, got %v", script, err)
		}
	}
}

func TestUnknownMethodErrorNotUnsupportedFeature(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")

	b, err := e.Compile(`
var myArr = array.new_int(0)
myArr.noSuchMethod()
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err == nil {
		t.Fatalf("expected unknown method error")
	}
	if strings.Contains(err.Error(), "unsupported feature") {
		t.Fatalf("expected unknown method error, got unsupported feature: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown method") {
		t.Fatalf("expected unknown method error, got %v", err)
	}
}

func TestAlertSink(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	e.SetDefaultSymbol("TEST")

	var events []AlertEvent
	e.SetAlertSink(func(ev AlertEvent) {
		events = append(events, ev)
	})

	b, err := e.Compile("if bar_index == 0\n    alert(\"hello\")")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(events))
	}
	if events[0].Message != "hello" {
		t.Fatalf("unexpected alert message: %q", events[0].Message)
	}
	if events[0].BarIndex != 0 {
		t.Fatalf("unexpected alert bar index: %d", events[0].BarIndex)
	}
}

func TestSymbolsFromRegisteredProviders(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.RegisterMarketDataProvider(providerWithClose("BBB", 10, 20, 30))

	syms, err := e.Symbols()
	if err != nil {
		t.Fatalf("symbols failed: %v", err)
	}
	sort.Strings(syms)
	if len(syms) != 2 || syms[0] != "AAA" || syms[1] != "BBB" {
		t.Fatalf("unexpected symbols: %v", syms)
	}
}

func TestValueTypesFromRegisteredProviders(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(&testProvider{
		data: map[string]SeriesExtended{
			"AAA|close":  mkSeries(1, 2, 3),
			"AAA|volume": mkSeries(10, 20, 30),
		},
		valueTypes: []string{"close", "volume"},
	})
	e.RegisterMarketDataProvider(&testProvider{
		data: map[string]SeriesExtended{
			"BBB|event": mkSeries(100, 200, 300),
		},
		valueTypes: []string{"event"},
	})

	vts, err := e.ValueTypes()
	if err != nil {
		t.Fatalf("value types failed: %v", err)
	}
	sort.Strings(vts)
	if len(vts) != 3 || vts[0] != "close" || vts[1] != "event" || vts[2] != "volume" {
		t.Fatalf("unexpected value types: %v", vts)
	}

	empty := NewEngine()
	if _, err := empty.ValueTypes(); err == nil {
		t.Fatalf("expected ValueTypes() to fail without providers")
	}
}

func TestDefaultValueTypeValidation(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.SetDefaultSymbol("AAA")
	e.SetDefaultValueType("volume")

	b, err := e.Compile("close")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err == nil || !strings.Contains(err.Error(), "default value_type") {
		t.Fatalf("expected default value_type error, got: %v", err)
	}
}

func TestBytecodeReferencedSymbolsAcrossProviders(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.RegisterMarketDataProvider(providerWithClose("BBB", 10, 20, 30))
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`close_of("AAA") + close_of("BBB")`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	program, err := decodeProgram(b)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	sort.Strings(program.Symbols)
	if len(program.Symbols) != 2 || program.Symbols[0] != "AAA" || program.Symbols[1] != "BBB" {
		t.Fatalf("unexpected bytecode symbols: %v", program.Symbols)
	}
	sort.Strings(program.ValueTypes)
	if len(program.ValueTypes) != 1 || program.ValueTypes[0] != "close" {
		t.Fatalf("unexpected bytecode value_types: %v", program.ValueTypes)
	}
	sort.Strings(program.SeriesKeys)
	if len(program.SeriesKeys) != 2 || program.SeriesKeys[0] != "AAA|close" || program.SeriesKeys[1] != "BBB|close" {
		t.Fatalf("unexpected bytecode series_keys: %v", program.SeriesKeys)
	}

	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 33 {
		t.Fatalf("expected 33, got %v", v)
	}
}

func TestCompiledBytecodeReusableAcrossEnginesWithDifferentProviders(t *testing.T) {
	compiler := NewEngine()
	b, err := compiler.Compile(`close_of("AAA") + close_of("BBB")`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	e1 := NewEngine()
	e1.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e1.RegisterMarketDataProvider(providerWithClose("BBB", 10, 20, 30))
	e1.SetDefaultSymbol("AAA")
	v1, err := e1.Execute(b)
	if err != nil {
		t.Fatalf("first execute failed: %v", err)
	}
	if v1.(float64) != 33 {
		t.Fatalf("expected first execute 33, got %v", v1)
	}

	e2 := NewEngine()
	e2.RegisterMarketDataProvider(providerWithClose("AAA", 4, 5, 6))
	e2.RegisterMarketDataProvider(providerWithClose("BBB", 40, 50, 60))
	e2.SetDefaultSymbol("AAA")
	v2, err := e2.Execute(b)
	if err != nil {
		t.Fatalf("second execute failed: %v", err)
	}
	if v2.(float64) != 66 {
		t.Fatalf("expected second execute 66, got %v", v2)
	}
}

func TestCompiledBytecodeReusableAfterProviderMutation(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1, 2, 3)
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	e.SetTimeframe("1D")

	b, err := e.Compile(`close + (timeframe.period == "1D" ? 100 : 0)`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	v1, err := e.Execute(b)
	if err != nil {
		t.Fatalf("first execute failed: %v", err)
	}
	if v1.(float64) != 103 {
		t.Fatalf("expected first execute 103, got %v", v1)
	}

	p.data[makeSeriesKey("AAA", "close")] = mkSeries(5, 6, 7)
	e.SetTimeframe("60")
	v2, err := e.Execute(b)
	if err != nil {
		t.Fatalf("second execute failed: %v", err)
	}
	if v2.(float64) != 7 {
		t.Fatalf("expected second execute 7, got %v", v2)
	}
	if p.timeframe != "60" {
		t.Fatalf("expected provider timeframe updated to 60, got %q", p.timeframe)
	}
}

func TestRuntimeStatePeekAPI(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 5, 6, 7))
	e.SetDefaultSymbol("AAA")

	rt, v, err := e.ExecuteWithRuntime([]byte{})
	if err == nil || rt != nil || v != nil {
		t.Fatalf("expected decode error for empty bytecode")
	}

	b, err := e.Compile(`
var x = close
var y = x + 2
y
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	rt, v, err = e.ExecuteWithRuntime(b)
	if err != nil {
		t.Fatalf("execute with runtime failed: %v", err)
	}
	if v.(float64) != 9 {
		t.Fatalf("expected 9, got %v", v)
	}
	snap := rt.Snapshot()
	if snap.ActiveSymbol != "AAA" {
		t.Fatalf("unexpected active symbol: %s", snap.ActiveSymbol)
	}
	if _, ok := snap.Variables["x"]; !ok {
		t.Fatalf("snapshot missing x variable")
	}
	if got, ok := rt.Value("y"); !ok || got.(float64) != 9 {
		t.Fatalf("unexpected runtime y value: %#v", got)
	}
	if gotRt := e.Runtime(); gotRt == nil {
		t.Fatalf("engine runtime should be stored after execute")
	}
}

func TestSeriesHistoryIndexing(t *testing.T) {
	v := compileExec(t, "close[1]", 10, 20, 30)
	if v.(float64) != 20 {
		t.Fatalf("expected 20, got %v", v)
	}
}

func TestVariableHistoryIndexing(t *testing.T) {
	script := `
var x = close
var y = x[1]
nz(y)
`
	v := compileExec(t, script, 10, 20, 30)
	if v.(float64) != 20 {
		t.Fatalf("expected 20, got %v", v)
	}
}

func TestCrossSymbolHistoryIndexing(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.RegisterMarketDataProvider(providerWithClose("BBB", 10, 20, 30))
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`close_of("BBB")[1] + close_of("AAA")`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 23 {
		t.Fatalf("expected 23, got %v", v)
	}
}

func TestNaNzAndBarIndex(t *testing.T) {
	script := `
var prev = close[10]
var p = nz(prev, 7)
var is_missing = na(prev)
var bonus = 0
if is_missing
    bonus = 1
p + bonus + bar_index
`
	v := compileExec(t, script, 10, 20, 30)
	if v.(float64) != 10 {
		t.Fatalf("expected 10, got %v", v)
	}
}

func TestMathAndTaNamespaceBuiltins(t *testing.T) {
	script := `
var a = ta.sma(close, 2)
var b = math.abs(-3)
var c = math.pow(2, 3)
var d = math.max(4, math.min(6, 5))
var e = math.round(2.4)
a + b + c + d + e
`
	v := compileExec(t, script, 1, 2, 3)
	if v.(float64) != 20.5 {
		t.Fatalf("expected 20.5, got %v", v)
	}
}

func TestTernaryOperator(t *testing.T) {
	script := `
var x = close > 15 ? 10 : 1
x
`
	v := compileExec(t, script, 10, 20, 30)
	if v.(float64) != 10 {
		t.Fatalf("expected 10, got %v", v)
	}
}

func TestGenericExpressionHistoryIndexing(t *testing.T) {
	v := compileExec(t, "(close + 1)[1]", 10, 20, 30)
	if v.(float64) != 21 {
		t.Fatalf("expected 21, got %v", v)
	}

	v2 := compileExec(t, "ta.sma(close, 2)[1]", 1, 2, 3)
	if v2.(float64) != 1.5 {
		t.Fatalf("expected 1.5, got %v", v2)
	}
}

func TestTACrossoverAndCrossunder(t *testing.T) {
	upScript := `
var up = ta.crossover(close, 1.5)
up[1] ? 1 : 0
`
	up := compileExec(t, upScript, 1, 2, 3)
	if up.(float64) != 1 {
		t.Fatalf("expected 1, got %v", up)
	}

	downScript := `
var down = ta.crossunder(close, 2.5)
down[1] ? 1 : 0
`
	down := compileExec(t, downScript, 3, 2, 1)
	if down.(float64) != 1 {
		t.Fatalf("expected 1, got %v", down)
	}
}

func TestTAATR(t *testing.T) {
	v := compileExec(t, "ta.atr(3)", 10, 13, 12, 18)
	f := v.(float64)
	if math.Abs(f-2.8888888889) > 0.000001 {
		t.Fatalf("expected ~2.8888888889, got %v", f)
	}
}

func TestProviderValueTypeSeriesKeyAccess(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(&testProvider{
		data: map[string]SeriesExtended{
			"AAA|close":  mkSeries(10, 20, 30),
			"AAA|volume": mkSeries(100, 200, 300),
		},
		valueTypes: []string{"close", "volume"},
	})
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`value_of("AAA", "volume") + value_of("AAA", "close")`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	program, err := decodeProgram(b)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	sort.Strings(program.SeriesKeys)
	if len(program.SeriesKeys) != 2 || program.SeriesKeys[0] != "AAA|close" || program.SeriesKeys[1] != "AAA|volume" {
		t.Fatalf("unexpected bytecode series_keys: %v", program.SeriesKeys)
	}

	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 330 {
		t.Fatalf("expected 330, got %v", v)
	}
}

func TestSingleValueTypeProviderFallbackForClose(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithValueType("EVT", "event", 1, 2, 3))
	e.SetDefaultSymbol("EVT")

	v, err := e.Execute([]byte{})
	if err == nil || v != nil {
		t.Fatalf("expected decode error for empty bytecode")
	}

	b, err := e.Compile(`close + close[1]`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err = e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 5 {
		t.Fatalf("expected 5, got %v", v)
	}
}

func TestRuntimeExposedFunctionsAndFailoverBehavior(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(&testProvider{
		data: map[string]SeriesExtended{
			"EVT|event": mkSeries(1, 2, 3),
		},
		valueTypes: []string{"close", "event"},
	})
	e.SetDefaultSymbol("EVT")
	e.SetDefaultValueType("event")

	b, err := e.Compile(`close + close[1] + value_of("EVT", "missing")`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	rt, v, err := e.ExecuteWithRuntime(b)
	if err != nil {
		t.Fatalf("execute with runtime failed: %v", err)
	}
	if v.(float64) != 8 {
		t.Fatalf("expected 8, got %v", v)
	}

	if rt == nil {
		t.Fatalf("runtime should not be nil")
	}
	vts := rt.ValueTypes("EVT")
	if len(vts) != 2 || vts[0] != "close" || vts[1] != "event" {
		t.Fatalf("unexpected runtime value types: %v", vts)
	}

	keys := rt.SeriesKeys()
	sort.Strings(keys)
	if len(keys) < 2 || keys[0] != "EVT|close" || keys[1] != "EVT|event" {
		t.Fatalf("expected runtime keys to include close and event, got %v", keys)
	}

	if _, ok := rt.Series("EVT|close"); !ok {
		t.Fatalf("expected runtime series EVT|close to be available")
	}
	if _, ok := rt.Series("EVT|missing"); !ok {
		t.Fatalf("expected runtime series failover for EVT|missing")
	}
	if _, ok := rt.Series("bad-key"); ok {
		t.Fatalf("expected invalid series key to fail")
	}
}

func TestEngineClearRuntimeAndRuntimeRelease(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("AAA", 1, 2, 3))
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`
var x = close
var y = x + close[1]
y
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	rt, v, err := e.ExecuteWithRuntime(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 5 {
		t.Fatalf("expected 5, got %v", v)
	}
	if _, ok := rt.Value("y"); !ok {
		t.Fatalf("runtime should expose y before release")
	}

	rt.Release()
	if len(rt.SeriesKeys()) != 0 {
		t.Fatalf("expected released runtime to have no series keys")
	}
	if _, ok := rt.Value("y"); ok {
		t.Fatalf("expected released runtime to drop variable history")
	}

	e.ClearRuntime()
	if e.Runtime() != nil {
		t.Fatalf("expected engine runtime cleared")
	}
}

func TestMatrixCoreCoverage(t *testing.T) {
	script := `
var m = matrix.new_float(2, 2, 0)
m = matrix.set(m, 0, 0, 1)
m = matrix.set(m, 0, 1, 2)
m = matrix.set(m, 1, 0, 3)
m = matrix.set(m, 1, 1, 4)
var s = matrix.sum(m)
var a = matrix.avg(m)
var n = matrix.min(m)
var x = matrix.max(m)
var med = matrix.median(m)
var mode = matrix.mode(m)
var det = matrix.det(m)
var tr = matrix.trace(m)
var t = matrix.transpose(m)
var t01 = matrix.get(t, 0, 1)
var p = matrix.pow(m, 2)
var p00 = matrix.get(p, 0, 0)
var c = matrix.elements_count(m)
var sq = matrix.is_square(m) ? 1 : 0
s + a + n + x + med + mode + det + tr + t01 + p00 + c + sq
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-39.0) > 0.000001 {
		t.Fatalf("expected 39, got %v", v)
	}
}

func TestMatrixModificationCoverage(t *testing.T) {
	script := `
var m = matrix.new_float(2, 2, 1)
m = matrix.add_row(m, [2, 2])
m = matrix.add_col(m, [3, 3, 3])
m = matrix.swap_rows(m, 0, 2)
m = matrix.swap_columns(m, 0, 2)
m = matrix.remove_row(m, 1)
m = matrix.remove_col(m, 1)
m = matrix.fill(m, 5)
m = matrix.reverse(m)
m = matrix.sort(m)
m = matrix.reshape(m, 1, 4)
var sub = matrix.submatrix(m, 0, 1, 1, 3)
matrix.get(sub, 0, 0) + matrix.columns(sub) + matrix.rows(m) + matrix.elements_count(m)
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-12.0) > 0.000001 {
		t.Fatalf("expected 12, got %v", v)
	}
}

func TestMatrixLinearAlgebraAndProperties(t *testing.T) {
	script := `
var m = matrix.new_float(2, 2, 0)
m = matrix.set(m, 0, 0, 2)
m = matrix.set(m, 1, 1, 3)
var vals = matrix.eigenvalues(m)
var vecs = matrix.eigenvectors(m)
var pin = matrix.pinv(m)
var p00 = matrix.get(pin, 0, 0)
var is_diag = matrix.is_diagonal(m) ? 1 : 0
var is_stoch = matrix.is_stochastic(m) ? 1 : 0
var r = matrix.rank(m)
array.get(vals, 0) + array.get(vals, 1) + p00 + is_diag + is_stoch + r + matrix.get(vecs, 0, 0) * 0
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-8.5) > 0.000001 {
		t.Fatalf("expected 8.5, got %v", v)
	}
}

func TestMatrixEigenHigherDimension(t *testing.T) {
	script := `
var m = matrix.new_float(3, 3, 0)
m = matrix.set(m, 0, 0, 1)
m = matrix.set(m, 1, 1, 2)
m = matrix.set(m, 2, 2, 4)
var vals = matrix.eigenvalues(m)
var vecs = matrix.eigenvectors(m)
array.get(vals, 0) + array.get(vals, 1) + array.get(vals, 2) + matrix.rows(vecs) + matrix.columns(vecs)
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-13.0) > 0.000001 {
		t.Fatalf("expected 13, got %v", v)
	}
}

func TestMatrixFillRange(t *testing.T) {
	script := `
var m = matrix.new_float(3, 3, 0)
m = matrix.fill(m, 7, 1, 3, 0, 2)
matrix.get(m, 0, 0) + matrix.get(m, 1, 0) + matrix.get(m, 2, 1) + matrix.get(m, 2, 2)
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-14.0) > 0.000001 {
		t.Fatalf("expected 14, got %v", v)
	}
}

func TestArrayExtendedBuiltins(t *testing.T) {
	script := `
var a = array.new_int(0)
a = array.push(a, 2)
a = array.push(a, 4)
a = array.unshift(a, 1)
var p = array.pop(a)
var s = array.shift(a)
a = array.push(a, 9)
var b = array.new_int(1, 7)
var c = array.concat(a, b)
var sl = array.slice(c, 1, 3)
var inc = array.includes(c, 7) ? 1 : 0
var idx = array.indexof(c, 9)
var z = array.size(array.clear(c))
array.get(sl, 0) + p + s + inc + idx + z
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-16.0) > 0.000001 {
		t.Fatalf("expected 16, got %v", v)
	}
}

func TestArrayAdvancedBuiltinsCoverage(t *testing.T) {
	script := `
var a = array.from(-1.0, 2.0, -3.0, 2.0, 5.0)
var ab = array.abs(a)
var s = array.sum(ab)
var av = array.avg(ab)
var c = array.copy(a)
c = array.insert(c, 1, 9.0)
var f = array.first(a)
var l = array.last(a)
var li = array.lastindexof(a, 2.0)
var mx = array.max(a)
var mx2 = array.max(a, 1)
var mn = array.min(a)
var mn2 = array.min(a, 1)
var med = array.median(a)
var mode = array.mode(a)
var rg = array.range(a)
var pl = array.percentile_linear_interpolation(a, 50)
var pn = array.percentile_nearest_rank(a, 50)
var pnt = array.percentile_neareast_rank(a, 50)
var pr = array.percentrank(a, 3)
var sorted = array.from(1.0, 2.0, 2.0, 3.0, 4.0)
var bl = array.binary_search_leftmost(sorted, 2.0)
var br = array.binary_search_rightmost(sorted, 2.0)
var x = array.from(1.0, 2.0, 3.0)
var y = array.from(2.0, 4.0, 6.0)
var cov = array.covariance(x, y)
var e1 = array.every(array.from(1.0, 2.0, 3.0)) ? 1 : 0
var e2 = array.every(array.from(2.0, 2.0, 2.0), 2.0) ? 1 : 0
var joined = array.join(array.from("a", "b", "c"), "-")
var g = array.new<float>(2, 1.5)
s + av + f + l + li + mx + mx2 + mn + mn2 + med + mode + rg + pl + pn + pnt + pr + bl + br + cov + e1 + e2 + array.get(c, 1) + array.get(g, 1) + str.length(joined)
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-140.4333333333) > 0.000001 {
		t.Fatalf("expected ~140.4333333333, got %v", v)
	}
}

func TestArrayInsertMutatesPineArrayWithoutReassignment(t *testing.T) {
	v := compileExec(t, `
var a = array.new_int(0)
a = array.push(a, 1)
a = array.push(a, 3)
array.insert(a, 1, 2)
array.get(a, 1) * 10 + array.size(a)
`, 1, 2, 3)
	if math.Abs(v.(float64)-23.0) > 0.000001 {
		t.Fatalf("expected 23, got %v", v)
	}
}

func TestArrayDerivedArraysStayMutableWithoutReassignment(t *testing.T) {
	v := compileExec(t, `
var a = array.from(-1.0, -2.0)
var b = array.copy(a)
var c = array.abs(a)
array.push(a, 3.0)
array.push(b, 4.0)
array.push(c, 5.0)
array.size(a) + array.size(b) * 10 + array.size(c) * 100
`, 1, 2, 3)
	if math.Abs(v.(float64)-333.0) > 0.000001 {
		t.Fatalf("expected 333, got %v", v)
	}
}

func TestStringBuiltins(t *testing.T) {
	script := `
var s = "pine-go"
var up = str.upper(s)
var lo = str.lower(up)
var c = str.contains(lo, "go") ? 1 : 0
var st = str.startswith(lo, "pin") ? 1 : 0
var en = str.endswith(lo, "go") ? 1 : 0
var r = str.replace(lo, "go", "ts")
var sub = str.substring(r, 5, 7)
var parts = str.split(r, "-")
str.length(array.get(parts, 0)) + str.length(sub) + c + st + en
`
	v := compileExec(t, script, 1, 2, 3)
	if math.Abs(v.(float64)-9.0) > 0.000001 {
		t.Fatalf("expected 9, got %v", v)
	}
}

func TestTAChangeHighestLowestAndMathExtended(t *testing.T) {
	script := `
var ch = ta.change(close, 2)
var hi = ta.highest(close, 3)
var lo = ta.lowest(close, 3)
var m = math.log(math.exp(1)) + math.sin(0) + math.cos(0) + math.tan(0)
ch + hi + lo + m
`
	v := compileExec(t, script, 10, 12, 11, 15)
	if math.Abs(v.(float64)-31.0) > 0.000001 {
		t.Fatalf("expected 31, got %v", v)
	}
}

func TestTAStdevAndCorrelation(t *testing.T) {
	script := `
var s1 = ta.stdev(close, 4)
var s2 = ta.stdev(close, 4, false)
var c1 = ta.correlation(close, close * 2, 4)
var c2 = ta.correlation(close, -close, 4)
s1 + s2 + c1 + c2
`
	v := compileExec(t, script, 1, 2, 3, 4)
	if math.Abs(v.(float64)-2.4090284375) > 0.000001 {
		t.Fatalf("expected ~2.4090284375, got %v", v)
	}
}

func TestTAAdditionalFunctionsCoverage(t *testing.T) {
	script := `
var rma = ta.rma(close, 5)
var wma = ta.wma(close, 5)
var swma = ta.swma(close)
var hma = ta.hma(close, 5)
var alma = ta.alma(close, 5, 0.85, 6)
var almaf = ta.alma(close, 5, 0.85, 6, true)
var lr = ta.linreg(close, 5, 0)
var cci = ta.cci(close, 5)
var cmo = ta.cmo(close, 5)
var cog = ta.cog(close, 5)
[macdLine, signalLine, histLine] = ta.macd(close, 3, 6, 2)
var ok = (na(rma) ? 0 : 1) + (na(wma) ? 0 : 2) + (na(swma) ? 0 : 4) + (na(hma) ? 0 : 8) + (na(alma) ? 0 : 16) + (na(almaf) ? 0 : 32) + (na(lr) ? 0 : 64) + (na(cci) ? 0 : 128) + (na(cmo) ? 0 : 256) + (na(cog) ? 0 : 512) + (na(macdLine) or na(signalLine) or na(histLine) ? 0 : 1024)
var macdRel = math.abs((macdLine - signalLine) - histLine) < 0.000001 ? 2048 : 0
ok + macdRel
`
	v := compileExec(t, script, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	if v.(float64) != 4095 {
		t.Fatalf("expected 4095 (all indicators finite and macd relation true), got %v", v)
	}
}

func TestTAVWMAWithVolumeSeries(t *testing.T) {
	e := NewEngine()
	p := &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey("AAA", "close"):  mkSeries(1, 2, 3, 4),
			makeSeriesKey("AAA", "volume"): mkSeries(10, 10, 20, 20),
		},
		valueTypes: []string{"close", "volume"},
		timeframe:  "1D",
		session:    "regular",
	}
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	b, err := e.Compile(`ta.vwma(close, 3)`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if math.Abs(v.(float64)-3.2) > 0.000001 {
		t.Fatalf("expected 3.2, got %v", v)
	}
}

func TestTAPercentileFunctions(t *testing.T) {
	v := compileExec(t, `
var p1 = ta.percentile_linear_interpolation(close, 5, 50)
var p2 = ta.percentile_nearest_rank(close, 5, 50)
var p3 = ta.percentrank(close, 5)
var ok = (math.abs(p1 - 8) < 0.000001 ? 1 : 0) + (math.abs(p2 - 8) < 0.000001 ? 1 : 0) + (math.abs(p3 - 100) < 0.000001 ? 1 : 0)
ok
`, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	if v.(float64) != 3 {
		t.Fatalf("expected 3, got %v", v)
	}
}

func TestTAPivotPointLevels(t *testing.T) {
	e := NewEngine()
	p := &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey("AAA", "high"):  mkSeries(10, 12),
			makeSeriesKey("AAA", "low"):   mkSeries(8, 9),
			makeSeriesKey("AAA", "close"): mkSeries(9, 11),
		},
		valueTypes: []string{"high", "low", "close"},
		timeframe:  "1D",
		session:    "regular",
	}
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	b, err := e.Compile(`
var levels = ta.pivot_point_levels("Classic", true)
var pp = array.get(levels, 0)
var r1 = array.get(levels, 1)
var s1 = array.get(levels, 4)
(math.abs(pp - 9) < 0.000001 and math.abs(r1 - 10) < 0.000001 and math.abs(s1 - 8) < 0.000001) ? 1 : 0
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 1 {
		t.Fatalf("expected 1, got %v", v)
	}
}

func TestTANextBatchIndicatorsFiniteAndRanges(t *testing.T) {
	e := NewEngine()
	p := &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey("AAA", "high"):   mkSeries(2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13),
			makeSeriesKey("AAA", "low"):    mkSeries(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11),
			makeSeriesKey("AAA", "close"):  mkSeries(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12),
			makeSeriesKey("AAA", "volume"): mkSeries(100, 120, 140, 160, 180, 200, 220, 240, 260, 280, 300, 320),
		},
		valueTypes: []string{"high", "low", "close", "volume"},
		timeframe:  "1D",
		session:    "regular",
	}
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`
[bbMid, bbUp, bbLow] = ta.bb(close, 5, 2)
var bbw = ta.bbw(close, 5, 2)
[kcMid, kcUp, kcLow] = ta.kc(close, 5, 1.5)
var kcw = ta.kcw(close, 5, 1.5)
var stoch = ta.stoch(close, high, low, 5)
var mfi = ta.mfi(close, 5)
var tsi = ta.tsi(close, 5, 8)
var wpr = ta.wpr(5)
[plusDI, minusDI, adx] = ta.dmi(5, 5)
var sar = ta.sar(0.02, 0.02, 0.2)
[st, dir] = ta.supertrend(3, 5)

var ok = 0
ok := ok + (na(bbMid) or na(bbUp) or na(bbLow) ? 0 : 1)
ok := ok + (bbUp > bbMid and bbMid > bbLow ? 1 : 0)
ok := ok + (na(bbw) ? 0 : 1)
ok := ok + (na(kcMid) or na(kcUp) or na(kcLow) ? 0 : 1)
ok := ok + (kcUp > kcMid and kcMid > kcLow ? 1 : 0)
ok := ok + (na(kcw) ? 0 : 1)
ok := ok + (stoch >= 0 and stoch <= 100 ? 1 : 0)
ok := ok + (mfi >= 0 and mfi <= 100 ? 1 : 0)
ok := ok + (na(tsi) ? 0 : 1)
ok := ok + (wpr <= 0 and wpr >= -100 ? 1 : 0)
ok := ok + (na(plusDI) or na(minusDI) or na(adx) ? 0 : 1)
ok := ok + (plusDI >= 0 and minusDI >= 0 and adx >= 0 ? 1 : 0)
ok := ok + (na(sar) ? 0 : 1)
ok := ok + (na(st) ? 0 : 1)
ok := ok + (dir == 1 or dir == -1 ? 1 : 0)
ok
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 15 {
		t.Fatalf("expected 15, got %v", v)
	}
}

func TestTABBAndBBWReferenceFormula(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`
[bbMid, bbUp, bbLow] = ta.bb(close, 5, 2)
var bbw = ta.bbw(close, 5, 2)
var basis = ta.sma(close, 5)
var dev = ta.stdev(close, 5)

var ok = 0
ok := ok + (math.abs(bbMid - basis) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(bbUp - (basis + 2 * dev)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(bbLow - (basis - 2 * dev)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(bbw - ((bbUp - bbLow) / bbMid)) < 0.0000000001 ? 1 : 0)
ok
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 4 {
		t.Fatalf("expected 4, got %v", v)
	}
}

func TestDerivedPriceIdentifiersOHLC4Variants(t *testing.T) {
	e := NewEngine()
	p := &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey("AAA", "open"):  mkSeries(2, 6, 8),
			makeSeriesKey("AAA", "high"):  mkSeries(6, 18, 20),
			makeSeriesKey("AAA", "low"):   mkSeries(1, 3, 4),
			makeSeriesKey("AAA", "close"): mkSeries(4, 10, 14),
		},
		valueTypes: []string{"open", "high", "low", "close"},
		timeframe:  "1D",
		session:    "regular",
	}
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`
var ok = 0
ok := ok + (math.abs(hl2 - ((high + low) / 2)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(hlc3 - ((high + low + close) / 3)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(hlcc4 - ((high + low + close + close) / 4)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(ohlc4 - ((open + high + low + close) / 4)) < 0.0000000001 ? 1 : 0)

ok := ok + (math.abs(hl2[1] - ((high[1] + low[1]) / 2)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(hlc3[1] - ((high[1] + low[1] + close[1]) / 3)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(hlcc4[1] - ((high[1] + low[1] + close[1] + close[1]) / 4)) < 0.0000000001 ? 1 : 0)
ok := ok + (math.abs(ohlc4[1] - ((open[1] + high[1] + low[1] + close[1]) / 4)) < 0.0000000001 ? 1 : 0)
ok
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 8 {
		t.Fatalf("expected 8, got %v", v)
	}
}

func TestTANextBatchUtilityFunctionsAndPivots(t *testing.T) {
	e := NewEngine()
	p := &testProvider{
		data: map[string]SeriesExtended{
			makeSeriesKey("AAA", "high"):   mkSeries(1, 2, 5, 2, 1),
			makeSeriesKey("AAA", "low"):    mkSeries(3, 2, 0, 2, 3),
			makeSeriesKey("AAA", "close"):  mkSeries(1, 2, 3, 4, 5),
			makeSeriesKey("AAA", "volume"): mkSeries(10, 10, 10, 10, 10),
		},
		valueTypes: []string{"high", "low", "close", "volume"},
		timeframe:  "1D",
		session:    "regular",
	}
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")

	b, err := e.Compile(`
var bars = ta.barssince(close > 0)
var cv = ta.cum(close)
var vw = ta.valuewhen(close > 2, close, 0)
var hb = ta.highestbars(high, 5)
var lb = ta.lowestbars(low, 5)
var mx = ta.max(close)
var mn = ta.min(close)
var med = ta.median(close, 5)
var mode = ta.mode(close, 5)
var rng = ta.range(close, 5)
var vr = ta.variance(close, 5)
var dv = ta.dev(close, 5)
var rise = ta.rising(close, 5) ? 1 : 0
var fall = ta.falling(close, 5) ? 1 : 0
var trv = ta.tr()
var x = ta.cross(close, ta.sma(close, 2)) ? 1 : 0
var ph = ta.pivothigh(high, 2, 2)
var pl = ta.pivotlow(low, 2, 2)

var ok = 0
ok := ok + (bars == 0 ? 1 : 0)
ok := ok + (cv == 15 ? 1 : 0)
ok := ok + (vw == 5 ? 1 : 0)
ok := ok + (hb == 2 ? 1 : 0)
ok := ok + (lb == 2 ? 1 : 0)
ok := ok + (mx == 5 ? 1 : 0)
ok := ok + (mn == 1 ? 1 : 0)
ok := ok + (med == 3 ? 1 : 0)
ok := ok + (mode == 1 ? 1 : 0)
ok := ok + (rng == 4 ? 1 : 0)
ok := ok + (math.abs(vr - 2) < 0.000001 ? 1 : 0)
ok := ok + (math.abs(dv - 1.2) < 0.000001 ? 1 : 0)
ok := ok + (rise == 1 and fall == 0 ? 1 : 0)
ok := ok + (trv == 3 ? 1 : 0)
ok := ok + (x == 0 ? 1 : 0)
ok := ok + (ph == 5 ? 1 : 0)
ok := ok + (pl == 0 ? 1 : 0)
ok
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 17 {
		t.Fatalf("expected 17, got %v", v)
	}
}

func TestStringFormatBuiltin(t *testing.T) {
	v := compileExec(t, `str.length(str.format("A={0}, B={1}, {{x}}", 42, "ok"))`, 1, 2, 3)
	if v.(float64) != 15 {
		t.Fatalf("unexpected str.format length result: %v", v)
	}

	v2 := compileExec(t, `str.length(str.format("{2}-{0}", "first"))`, 1, 2, 3)
	if v2.(float64) != 9 {
		t.Fatalf("unexpected out-of-range placeholder behavior length: %v", v2)
	}
}

func TestEngineTimeframeInitializationAndApplyToProviders(t *testing.T) {
	p := providerWithClose("AAA", 1, 2, 3)
	p.timeframe = "15"

	e := NewEngine()
	e.RegisterMarketDataProvider(p)
	if got := e.Timeframe(); got != "15" {
		t.Fatalf("expected engine timeframe initialized from provider, got %q", got)
	}

	e.SetTimeframe("1D")
	b, err := e.Compile(`close`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if p.timeframe != "1D" {
		t.Fatalf("expected provider timeframe 1D after execution, got %q", p.timeframe)
	}
	if len(p.setCalls) == 0 || p.setCalls[len(p.setCalls)-1] != "1D" {
		t.Fatalf("expected provider SetTimeframe call with 1D, got %v", p.setCalls)
	}
}

func TestTimeframeBuiltinsCoverage(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1, 2, 3, 4)
	p.timeframe = "60"
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	e.SetTimeframe("60")

	b, err := e.Compile(`
var period_match = timeframe.period == "60" ? 1 : 0
var main_match = timeframe.main_period == "60" ? 1 : 0
var mul = timeframe.multiplier
var intraday = timeframe.isintraday ? 1 : 0
var minutes = timeframe.isminutes ? 1 : 0
var dwm = timeframe.isdwm ? 1 : 0
var sec = timeframe.in_seconds()
var from = timeframe.from_seconds(3600)
var from_match = from == "60" ? 1 : 0
var ch = timeframe.change("120") ? 1 : 0
period_match + main_match + mul + intraday + minutes + dwm + sec + from_match + ch
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 3666 {
		t.Fatalf("expected 3666, got %v", v)
	}
}

func TestEngineSessionInitializationAndApplyToProviders(t *testing.T) {
	p := providerWithClose("AAA", 1, 2, 3)
	p.session = "regular"

	e := NewEngine()
	e.RegisterMarketDataProvider(p)
	if got := e.Session(); got != "regular" {
		t.Fatalf("expected engine session initialized from provider, got %q", got)
	}

	e.SetSession("extended")
	b, err := e.Compile(`close`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if p.session != "extended" {
		t.Fatalf("expected provider session extended after execution, got %q", p.session)
	}
}

func TestMathSessionAndTimeBuiltinsFromEngineClock(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1, 2, 3)
	p.timeframe = "60"
	p.session = "regular"
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	e.SetTimeframe("60")
	e.SetSession("extended")

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 1, 1, 3, 30, 0, 0, time.UTC)
	e.SetStartTime(start)
	e.SetCurrentTime(now)

	b, err := e.Compile(`
var m = math.e + math.pi + math.phi + math.rphi
var s = (session.extended == "extended" and session.regular == "regular") ? 1 : 0
var tf = timeframe.isdaily ? 1 : 0
var ticks = timeframe.isticks ? 1 : 0
var t = time
var tFn = time()
var tc = time_close
var tcFn = time_close()
var tn = timenow
var tnFn = timenow()
var td = time_tradingday
var tdFn = time_tradingday()
var ts = timestamp(2025, 1, 1, 2, 0, 0)
var tsz = timestamp("UTC+0", 2025, 1, 1, 3, 0, 0)
var ok = (t == ts ? 1 : 0) + (tc == tsz ? 1 : 0) + (t == tFn ? 1 : 0) + (tc == tcFn ? 1 : 0) + (tn == tnFn ? 1 : 0) + (td == tdFn ? 1 : 0)
m + s + tf + ticks + ok
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	expected := math.E + math.Pi + ((1 + math.Sqrt(5)) / 2) + ((math.Sqrt(5) - 1) / 2) + 1 + 0 + 0 + 6
	if math.Abs(v.(float64)-expected) > 0.000001 {
		t.Fatalf("expected %v, got %v", expected, v)
	}
}

func TestRequestedMathFunctionsCoverage(t *testing.T) {
	script := `
var a = math.log10(1000)
var b = abs(-5)
var c = avg(2, 4, 6)
var d = cos(0)
var e = exp(1)
var f = max(1, 3, 2)
var g = min(1, 3, 2)
var h = pow(2, 3)
var i = sin(0)
var j = sum(1, 2, 3)
var k = tan(0)
var l = acos(1)
var m = asin(0)
var n = atan(1)
var o = ceil(1.2)
var p = sign(-3)
var q = sqrt(9)
var r = floor(1.8)
var s = round(1.6)
var td = todegrees(math.pi)
var tr = toradians(180)
a+b+c+d+e+f+g+h+i+j+k+l+m+n+o+p+q+r+s+td+tr
`
	v := compileExec(t, script, 1, 2, 3)
	expected := 3.0 + 5.0 + 4.0 + 1.0 + math.E + 3.0 + 1.0 + 8.0 + 0.0 + 6.0 + 0.0 + 0.0 + 0.0 + math.Atan(1.0) + 2.0 - 1.0 + 3.0 + 1.0 + 2.0 + 180.0 + math.Pi
	if math.Abs(v.(float64)-expected) > 0.000001 {
		t.Fatalf("expected %v, got %v", expected, v)
	}
}

func TestLogBuiltinsWriteEngineBuffer(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1)
	p.timeframe = "60"
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	e.SetTimeframe("60")
	start := time.Date(2025, 2, 2, 12, 0, 0, 0, time.UTC)
	e.SetStartTime(start)
	e.SetCurrentTime(start)
	e.ClearLogs()

	b, err := e.Compile(`
log.info("close={0}", close)
log.warning("warn")
log.error("err {0}", 7)
close
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	v, err := e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if v.(float64) != 1 {
		t.Fatalf("expected close result 1, got %v", v)
	}

	logs := e.Logs()
	if len(logs) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(logs))
	}
	levels := []string{"info", "warning", "error"}
	messages := []string{"close=1", "warn", "err 7"}
	for i := range logs {
		if logs[i].Level != levels[i] {
			t.Fatalf("expected log level %q at %d, got %q", levels[i], i, logs[i].Level)
		}
		if logs[i].Message != messages[i] {
			t.Fatalf("expected log message %q at %d, got %q", messages[i], i, logs[i].Message)
		}
		if !logs[i].Timestamp.Equal(start) {
			t.Fatalf("expected log timestamp %v at %d, got %v", start, i, logs[i].Timestamp)
		}
	}
}

func TestLogWarningSupportsNumberFormatPlaceholders(t *testing.T) {
	e := NewEngine()
	p := providerWithClose("AAA", 1)
	p.timeframe = "60"
	e.RegisterMarketDataProvider(p)
	e.SetDefaultSymbol("AAA")
	e.SetTimeframe("60")
	start := time.Date(2025, 2, 2, 12, 0, 0, 0, time.UTC)
	e.SetStartTime(start)
	e.SetCurrentTime(start)
	e.ClearLogs()

	b, err := e.Compile(`
var numerator = 10.123456789
var denominator = 3.0
var ratio = numerator / denominator
var average = (numerator + denominator + ratio) / 3
log.warning(
    "Values (unconfirmed):\nnumerator: {0,number,#.########}\ndenominator: {1,number,#.########}"
    + "\nratio: {2,number,#.########}\naverage: {3,number,#.########}",
    numerator, denominator, ratio, average
)
close
`)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	_, err = e.Execute(b)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	logs := e.Logs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	expected := "Values (unconfirmed):\n" +
		"numerator: 10.12345679\n" +
		"denominator: 3\n" +
		"ratio: 3.3744856\n" +
		"average: 5.49931413"
	if logs[0].Level != "warning" {
		t.Fatalf("expected warning level, got %q", logs[0].Level)
	}
	if logs[0].Message != expected {
		t.Fatalf("unexpected formatted message:\nexpected:\n%s\nactual:\n%s", expected, logs[0].Message)
	}
}
