// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import "testing"

func TestAlternativeTickerDescriptors(t *testing.T) {
	for _, tc := range []struct {
		expr, chart, key string
		want             interface{}
	}{
		{`ticker.renko("NYSE:TEST","ATR",14,true,"OHLC")`, "renko", "param", float64(14)},
		{`ticker.kagi("NYSE:TEST",2.5)`, "kagi", "reversal", 2.5},
		{`ticker.linebreak("NYSE:TEST",3)`, "linebreak", "number_of_lines", float64(3)},
		{`ticker.pointfigure("NYSE:TEST","hl","Traditional",1,3)`, "pointfigure", "source", "hl"},
	} {
		t.Run(tc.chart, func(t *testing.T) {
			id := completionValue(t, "result = "+tc.expr, 1).(string)
			if id == "NYSE:TEST" {
				t.Fatal("alternative ticker silently became ordinary candles")
			}
			d, err := ParseTickerID(id)
			if err != nil {
				t.Fatal(err)
			}
			if d.Symbol != "NYSE:TEST" || d.Chart != tc.chart || d.Parameters[tc.key] != tc.want {
				t.Fatalf("%+v", d)
			}
			plain, err := (&Runtime{}).builtinTicker("ticker.standard", []interface{}{id})
			if err != nil || plain != "NYSE:TEST" {
				t.Fatalf("%v %v", plain, err)
			}
		})
	}
}
