// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	pinego "github.com/woodstock-tokyo/pinescription"
	series "github.com/woodstock-tokyo/pinescription/series"
)

//go:embed script.pine
var scriptFS embed.FS

type demoOHLCVProvider struct {
	timeframe string
	session   string
	bars      int
}

func (d *demoOHLCVProvider) GetSeries(seriesKey string) (pinego.SeriesExtended, error) {
	symbol, valueType, ok := splitSeriesKey(seriesKey)
	if !ok || symbol != "DEMO" {
		return nil, errors.New("unknown series key")
	}

	values := make([]float64, 0, d.bars)
	for i := 0; i < d.bars; i++ {
		base := 100.0 + float64(i)*0.1
		op := base
		cl := base
		if i%2 == 0 {
			cl = base + 0.05
		} else {
			cl = base - 0.02
		}
		hi := op
		if cl > hi {
			hi = cl
		}
		lo := op
		if cl < lo {
			lo = cl
		}
		hi += 0.1
		lo -= 0.1
		vol := 1000.0
		if i == d.bars-1-50 {
			vol = 5000.0
		}

		switch valueType {
		case "open":
			values = append(values, op)
		case "high":
			values = append(values, hi)
		case "low":
			values = append(values, lo)
		case "close":
			values = append(values, cl)
		case "volume":
			values = append(values, vol)
		default:
			return nil, errors.New("unknown value type")
		}
	}

	q := series.NewQueue(d.bars + 8)
	for _, v := range values {
		q.Update(v)
	}
	return q, nil
}

func (d *demoOHLCVProvider) GetSymbols() ([]string, error) {
	return []string{"DEMO"}, nil
}

func (d *demoOHLCVProvider) GetValuesTypes() ([]string, error) {
	return []string{"open", "high", "low", "close", "volume"}, nil
}

func (d *demoOHLCVProvider) SetTimeframe(timeframe string) error {
	d.timeframe = timeframe
	return nil
}

func (d *demoOHLCVProvider) GetTimeframe() string {
	if strings.TrimSpace(d.timeframe) == "" {
		d.timeframe = "1D"
	}
	return d.timeframe
}

func (d *demoOHLCVProvider) SetSession(session string) error {
	d.session = session
	return nil
}

func (d *demoOHLCVProvider) GetSession() string {
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
	scriptBytes, err := scriptFS.ReadFile("script.pine")
	if err != nil {
		panic(err)
	}

	engine := pinego.NewEngine()
	engine.RegisterMarketDataProvider(&demoOHLCVProvider{timeframe: "1D", session: "regular", bars: 200})
	engine.SetDefaultSymbol("DEMO")

	var alerts []pinego.AlertEvent
	engine.SetAlertSink(func(ev pinego.AlertEvent) {
		alerts = append(alerts, ev)
	})

	bytecode, err := engine.Compile(string(scriptBytes))
	if err != nil {
		panic(err)
	}
	result, err := engine.Execute(bytecode)
	if err != nil {
		panic(err)
	}

	fmt.Printf("result=%v alerts=%d\n", result, len(alerts))
	for i := 0; i < len(alerts) && i < 10; i++ {
		fmt.Printf("[%d] bar=%d symbol=%s freq=%s msg=%q\n", i, alerts[i].BarIndex, alerts[i].Symbol, alerts[i].Frequency, alerts[i].Message)
	}
}
