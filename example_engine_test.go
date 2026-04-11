// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription_test

import (
	"errors"
	"fmt"
	"strings"

	pinescription "github.com/woodstock-tokyo/pinescription"
	"github.com/woodstock-tokyo/pinescription/series"
)

type exampleProvider struct {
	timeframe string
	session   string
	series    map[string][]float64
}

func (p *exampleProvider) GetSeries(seriesKey string) (pinescription.SeriesExtended, error) {
	values, ok := p.series[seriesKey]
	if !ok {
		return nil, errors.New("unknown series key")
	}
	q := series.NewQueue(len(values) + 1)
	for _, value := range values {
		q.Update(value)
	}
	return q, nil
}

func (p *exampleProvider) GetSymbols() ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(p.series))
	for key := range p.series {
		symbol, _, ok := strings.Cut(key, "|")
		if !ok || symbol == "" || seen[symbol] {
			continue
		}
		seen[symbol] = true
		out = append(out, symbol)
	}
	return out, nil
}

func (p *exampleProvider) GetValuesTypes() ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(p.series))
	for key := range p.series {
		_, valueType, ok := strings.Cut(key, "|")
		if !ok || valueType == "" || seen[valueType] {
			continue
		}
		seen[valueType] = true
		out = append(out, valueType)
	}
	return out, nil
}

func (p *exampleProvider) SetTimeframe(timeframe string) error {
	p.timeframe = timeframe
	return nil
}

func (p *exampleProvider) GetTimeframe() string {
	if p.timeframe == "" {
		return "1D"
	}
	return p.timeframe
}

func (p *exampleProvider) SetSession(session string) error {
	p.session = session
	return nil
}

func (p *exampleProvider) GetSession() string {
	if p.session == "" {
		return "regular"
	}
	return p.session
}

func ExampleNewEngine() {
	engine := pinescription.NewEngine()
	engine.RegisterMarketDataProvider(&exampleProvider{
		series: map[string][]float64{
			"DEMO|close": {40, 41, 42},
		},
	})
	engine.SetDefaultSymbol("DEMO")

	bytecode, err := engine.Compile("close + 1")
	if err != nil {
		panic(err)
	}

	result, err := engine.Execute(bytecode)
	if err != nil {
		panic(err)
	}

	fmt.Printf("result=%.0f\n", result.(float64))
	// Output:
	// result=43
}

func ExampleEngine_ExecuteWithRuntime() {
	engine := pinescription.NewEngine()
	engine.RegisterMarketDataProvider(&exampleProvider{
		series: map[string][]float64{
			"DEMO|close": {10, 11, 12},
		},
	})
	engine.SetDefaultSymbol("DEMO")

	bytecode, err := engine.Compile("scaled = close * 2\nscaled")
	if err != nil {
		panic(err)
	}

	runtime, result, err := engine.ExecuteWithRuntime(bytecode)
	if err != nil {
		panic(err)
	}

	snapshot := runtime.Snapshot()
	fmt.Printf("result=%.0f\n", result.(float64))
	fmt.Printf("bar_index=%d\n", snapshot.BarIndex)
	fmt.Printf("symbol=%s\n", snapshot.ActiveSymbol)
	// Output:
	// result=24
	// bar_index=2
	// symbol=DEMO
}
