// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"strings"

	series "github.com/woodstock-tokyo/pinescription/series"

	pinego "github.com/woodstock-tokyo/pinescription"
)

type demoProvider struct {
	timeframe string
	session   string
}

func (d *demoProvider) GetSeries(seriesKey string) (pinego.SeriesExtended, error) {
	symbol, valueType, ok := splitSeriesKey(seriesKey)
	if !ok || symbol != "DEMO" {
		return nil, errors.New("unknown series key")
	}
	q := series.NewQueue(64)
	values := []float64{100, 101, 102, 103, 104, 106, 107}
	if valueType == "volume" {
		values = []float64{900, 920, 940, 970, 980, 1000, 1010}
	}
	for _, v := range values {
		q.Update(v)
	}
	return q, nil
}

func (d *demoProvider) GetSymbols() ([]string, error) {
	return []string{"DEMO"}, nil
}

func (d *demoProvider) GetValuesTypes() ([]string, error) {
	return []string{"close", "volume"}, nil
}

func (d *demoProvider) SetTimeframe(timeframe string) error {
	d.timeframe = timeframe
	return nil
}

func (d *demoProvider) GetTimeframe() string {
	if strings.TrimSpace(d.timeframe) == "" {
		d.timeframe = "1D"
	}
	return d.timeframe
}

func (d *demoProvider) SetSession(session string) error {
	d.session = session
	return nil
}

func (d *demoProvider) GetSession() string {
	if strings.TrimSpace(d.session) == "" {
		d.session = "regular"
	}
	return d.session
}

func splitSeriesKey(seriesKey string) (string, string, bool) {
	parts := strings.SplitN(seriesKey, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func main() {
	engine := pinego.NewEngine()
	engine.RegisterMarketDataProvider(&demoProvider{timeframe: "1D", session: "regular"})
	engine.SetDefaultSymbol("DEMO")

	script := `
sum2(a, b) => a + b
var ma = sma(close, 3)
var ex = ema(close, 3)
sum2(ma, ex)
`

	bytecode, err := engine.Compile(script)
	if err != nil {
		panic(err)
	}
	result, err := engine.Execute(bytecode)
	if err != nil {
		panic(err)
	}

	fmt.Printf("result=%v\n", result)
}
