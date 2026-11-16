// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

type securityTestSource struct {
	contexts map[string]SecurityData
	calls    []string
}

func (s *securityTestSource) GetSecurityData(symbol, tf string) (SecurityData, error) {
	key := symbol + "/" + tf
	s.calls = append(s.calls, key)
	data, ok := s.contexts[key]
	if !ok {
		return SecurityData{}, fmt.Errorf("unknown requested context %s", key)
	}
	return data, nil
}
func securityFixture(tf string, step time.Duration, values ...float64) SecurityData {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := make([]BarTime, len(values))
	for i := range values {
		bars[i] = BarTime{Open: start.Add(time.Duration(i) * step), Close: start.Add(time.Duration(i+1) * step)}
	}
	return SecurityData{Series: map[string]SeriesExtended{"close": mkSeries(values...), "open": mkSeries(values...)}, Bars: bars, Info: SymbolInfo{"timezone": "UTC"}}
}
func requestTestEngine(values ...float64) (*Engine, *securityTestSource) {
	e := NewEngine()
	p := providerWithClose("TEST", values...)
	p.timeframe = "60"
	e.RegisterMarketDataProvider(p)
	e.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	source := &securityTestSource{contexts: map[string]SecurityData{
		"OTHER/120": securityFixture("120", 2*time.Hour, 100, 200, 300),
		"OTHER/30":  securityFixture("30", 30*time.Minute, 1, 2, 3, 4, 5, 6, 7, 8),
	}}
	e.SetSecurityDataProvider(source)
	return e, source
}
func TestSecurityAlignmentIsolationAndDependencies(t *testing.T) {
	e, source := requestTestEngine(10, 20, 30, 40)
	var observed [][]interface{}
	e.RegisterFunction("capture", func(a ...interface{}) (interface{}, error) {
		observed = append(observed, append([]interface{}(nil), a...))
		return nil, nil
	})
	sideEffects := 0
	e.RegisterFunction("effect", func(...interface{}) (interface{}, error) { sideEffects++; return nil, nil })
	code, err := e.Compile(`input_length = input.int(2)
average = ta.sma(close,input_length)
effect()
confirmed = request.security("OTHER","120",average)
ahead = request.security("OTHER","120",close,lookahead = barmerge.lookahead_on)
gapped = request.security("OTHER","120",close,gaps = barmerge.gaps_on)
capture(confirmed,ahead,gapped)
result = confirmed`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if sideEffects != 4 {
		t.Fatalf("unrelated statements replayed in security: %d", sideEffects)
	}
	if len(source.calls) != 1 {
		t.Fatalf("requested context fetched %d times", len(source.calls))
	}
	if !isNA(observed[0][0]) || !isNA(observed[1][0]) || !isNA(observed[2][0]) || observed[3][0] != float64(150) {
		t.Fatalf("confirmed SMA alignment: %v", observed)
	}
	for i, want := range []float64{100, 100, 200, 200} {
		if observed[i][1] != want {
			t.Fatalf("lookahead: %v", observed)
		}
	}
	if !isNA(observed[0][2]) || observed[1][2] != float64(100) || !isNA(observed[2][2]) || observed[3][2] != float64(200) {
		t.Fatalf("gaps: %v", observed)
	}
	if got, _ := rt.Value("average"); got != float64(35) {
		t.Fatalf("secondary context changed chart state: %v", got)
	}
}
func TestSecurityLowerTFAndTupleArrays(t *testing.T) {
	e, _ := requestTestEngine(10, 20)
	code, err := e.Compile(`[opens,closes] = request.security_lower_tf("OTHER","30",[open,close])
first = request.security("OTHER","30",close,lookahead = barmerge.lookahead_on)
last = request.security("OTHER","30",close)
result = array.sum(opens)+array.sum(closes)+first+last`)
	if err != nil {
		t.Fatal(err)
	}
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if got, _ := rt.Value("result"); got != float64(21) {
		t.Fatalf("got %v,want 21", got)
	}
	if got, _ := rt.Value("first"); got != float64(3) {
		t.Fatal(got)
	}
	if got, _ := rt.Value("last"); got != float64(4) {
		t.Fatal(got)
	}
}
func TestSecurityFunctionParametersLimitsAndHooks(t *testing.T) {
	e, _ := requestTestEngine(10, 20, 30, 40)
	code, err := e.Compile(`fetch(src) => request.security("OTHER","120",src)
result = fetch(close + 5)`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.Execute(code); err != nil || got != float64(205) {
		t.Fatalf("%v %v", got, err)
	}
	for _, tc := range []struct{ script, want string }{
		{`request.security("MISSING","120",close)`, "unknown requested context"},
		{`request.security_lower_tf("OTHER","120",close)`, "must not exceed"},
		{`request.security("OTHER","120",close,currency = "USD")`, "conversion"},
		{`request.security("OTHER","BAD",close)`, "invalid"},
	} {
		code, err = e.Compile(tc.script)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Execute(code); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("got %v,want %s", err, tc.want)
		}
	}
	code, _ = e.Compile(`result = request.security("MISSING","120",close,ignore_invalid_symbol = true)`)
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if got, _ := rt.Value("result"); !isNA(got) {
		t.Fatal(got)
	}
	e.RegisterFunction("request.security", func(...interface{}) (interface{}, error) { return 7.0, nil })
	code, _ = e.Compile(`request.security("MISSING","120",close)`)
	if got, err := e.Execute(code); err != nil || got != 7.0 {
		t.Fatalf("hook lost precedence: %v %v", got, err)
	}
}
func TestSameContextSecurityAndEmptyIntrabars(t *testing.T) {
	e := NewEngine()
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2))
	code, _ := e.Compile(`result = request.security("","",close)`)
	if got, err := e.Execute(code); err != nil || got != float64(2) {
		t.Fatalf("%v %v", got, err)
	}
	e, source := requestTestEngine(1, 2)
	data := source.contexts["OTHER/30"]
	data.Bars = data.Bars[:2]
	data.Series = map[string]SeriesExtended{"close": mkSeries(1, 2), "open": mkSeries(1, 2)}
	source.contexts["OTHER/30"] = data
	code, _ = e.Compile(`[a,b] = request.security_lower_tf("OTHER","30",[open,close])
result = array.size(a)+array.size(b)`)
	if got, err := e.Execute(code); err != nil || got != float64(0) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestSecurityShadowedParametersAndSnapshots(t *testing.T) {
	e, _ := requestTestEngine(1, 2, 3, 4)
	code, err := e.Compile(`twice(x) => x * 2
x = request.security("OTHER","120",twice(close))
result = x`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.Execute(code); err != nil || got != float64(400) {
		t.Fatalf("%v %v", got, err)
	}
	array := &pineArray{}
	array.items = []interface{}{array}
	cloned, err := cloneRequestValue(array)
	if err != nil {
		t.Fatal(err)
	}
	copy := cloned.(*pineArray)
	if copy == array || copy.items[0] != copy {
		t.Fatal("cyclic request snapshot was not isolated")
	}
	matrix, _ := newMatrix(1, 1, 2)
	cloned, err = cloneRequestValue(matrix)
	if err != nil {
		t.Fatal(err)
	}
	cloned.(*Matrix).Data[0][0] = 9
	if matrix.Data[0][0] != 2 {
		t.Fatal("matrix snapshot aliased its source")
	}
	slice := []interface{}{1.0, 2.0}
	cloned, err = cloneRequestValue([]interface{}{slice[:1], slice[:2], matrix, matrix})
	if err != nil {
		t.Fatal(err)
	}
	items := cloned.([]interface{})
	if len(items[0].([]interface{})) != 1 || len(items[1].([]interface{})) != 2 || items[2] != items[3] {
		t.Fatal("request graph identity/shape was lost")
	}
}

func TestSecurityReplaysOnlyNeededCollectionMutations(t *testing.T) {
	e, _ := requestTestEngine(1, 2, 3, 4)
	code, err := e.Compile(`a = array.from(close)
array.push(a,5)
result = request.security("OTHER","120",array.sum(a))`)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.Execute(code); err != nil || got != float64(205) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestEmptyIntrabarTupleShapeIsBounded(t *testing.T) {
	program, err := parseProgram(`recursive() => recursive()
pair() =>
    [open,close]`)
	if err != nil {
		t.Fatal(err)
	}
	if got := requestTupleSize(&Expr{Kind: "call", Left: &Expr{Kind: "ident", Name: "recursive"}}, program); got != 0 {
		t.Fatal(got)
	}
	result := emptyIntrabars(&Expr{Kind: "call", Left: &Expr{Kind: "ident", Name: "pair"}}, program)
	tuple, ok := result.([]interface{})
	if !ok || len(tuple) != 2 {
		t.Fatalf("block function lost tuple shape: %T %v", result, result)
	}
}
