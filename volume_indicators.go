// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"math"
)

func isVolumeIndicator(name string) bool {
	switch name {
	case "ta.accdist", "ta.iii", "ta.nvi", "ta.obv", "ta.pvi", "ta.pvt", "ta.wad", "ta.wvad":
		return true
	}
	return false
}

// Unlike the legacy price fallback, a volume-based API must not substitute
// close prices when a provider does not supply volume.
func (r *Runtime) exactSeriesValue(valueType string, bar int) float64 {
	if bar < 0 || !r.valueTypesBySymbol[r.activeSymbol][valueType] {
		return math.NaN()
	}
	ser, err := r.getExactSeries(r.activeSymbol, valueType)
	if err != nil || bar >= ser.Length() {
		return math.NaN()
	}
	return ser.Last(ser.Length() - 1 - bar)
}

type volumeIndicatorState struct {
	sums   []float64
	values []float64
}

func (r *Runtime) volumeIndicator(name string) float64 {
	idx := r.effectiveBarIndex()
	if idx < 0 {
		return math.NaN()
	}
	key := name + ":" + r.activeSymbol
	st, ok := r.indicatorState[key].(*volumeIndicatorState)
	if !ok {
		st = &volumeIndicatorState{}
		r.indicatorState[key] = st
	}
	// Recompute the current bar from its preceding committed value on updates.
	start := len(st.values) - 1
	if start < 0 {
		start = 0
	}
	if idx < start {
		return st.values[idx]
	}
	for bar := start; bar <= idx; bar++ {
		c, h, l, o, v := r.exactSeriesValue("close", bar), r.exactSeriesValue("high", bar), r.exactSeriesValue("low", bar), r.exactSeriesValue("open", bar), r.exactSeriesValue("volume", bar)
		pc, pv := r.exactSeriesValue("close", bar-1), r.exactSeriesValue("volume", bar-1)
		sum := 0.0
		if name == "ta.nvi" || name == "ta.pvi" {
			sum = 1
		}
		if bar > 0 {
			sum = st.sums[bar-1]
		}
		out := sum
		valid := !math.IsNaN(c) && !math.IsNaN(h) && !math.IsNaN(l) && !math.IsNaN(v)
		switch name {
		case "ta.accdist", "ta.iii":
			term := 0.0
			if valid && h != l {
				term = (2*c - h - l) / (h - l) * v
			}
			if name == "ta.iii" {
				out = term
				if !valid {
					out = math.NaN()
				}
			} else {
				sum += term
				out = sum
			}
		case "ta.wvad":
			out = math.NaN()
			if valid && !math.IsNaN(o) {
				out = 0
				if h != l {
					out = (c - o) / (h - l) * v
				}
			}
		case "ta.obv":
			if math.IsNaN(pc) {
				sum, out = 0, math.NaN()
			} else {
				if c > pc {
					sum += v
				} else if c < pc {
					sum -= v
				}
				out = sum
			}
		case "ta.pvt":
			if !math.IsNaN(c) && !math.IsNaN(pc) && !math.IsNaN(v) && pc != 0 {
				sum += (c - pc) / pc * v
			}
			out = sum
			if bar == 0 {
				out = math.NaN()
			}
		case "ta.wad":
			if !math.IsNaN(c) && !math.IsNaN(pc) && !math.IsNaN(h) && !math.IsNaN(l) {
				if c > pc {
					sum += c - math.Min(l, pc)
				} else if c < pc {
					sum += c - math.Max(h, pc)
				}
			}
			out = sum
		case "ta.nvi", "ta.pvi":
			if !math.IsNaN(c) && !math.IsNaN(pc) && c != 0 && pc != 0 {
				if math.IsNaN(v) {
					v = 0
				}
				if math.IsNaN(pv) {
					pv = 0
				}
				if (name == "ta.nvi" && v < pv) || (name == "ta.pvi" && v > pv) {
					sum *= c / pc
				}
			}
			out = sum
		}
		if bar < len(st.values) {
			st.values[bar], st.sums[bar] = out, sum
		} else {
			st.values = append(st.values, out)
			st.sums = append(st.sums, sum)
		}
	}
	return st.values[idx]
}
