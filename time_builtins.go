// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

func checkedTimezone(name string) (*time.Location, error) {
	if name == "" || name == "UTC" || name == "GMT" {
		return time.UTC, nil
	}
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "UTC") || strings.HasPrefix(upper, "GMT") {
		s := name[3:]
		if len(s) < 2 || (s[0] != '+' && s[0] != '-') {
			return nil, fmt.Errorf("invalid timezone %q", name)
		}
		parts := strings.Split(s[1:], ":")
		h, err := strconv.Atoi(parts[0])
		m := 0
		if len(parts) == 2 {
			var e error
			m, e = strconv.Atoi(parts[1])
			if e != nil {
				return nil, fmt.Errorf("invalid timezone %q", name)
			}
		}
		if err != nil || len(parts) > 2 || h < 0 || h > 14 || m < 0 || m > 59 || (h == 14 && m != 0) {
			return nil, fmt.Errorf("invalid timezone %q", name)
		}
		offset := (h*60 + m) * 60
		if s[0] == '-' {
			offset = -offset
		}
		return time.FixedZone(name, offset), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", name, err)
	}
	return loc, nil
}

func formatPineTime(t time.Time, pattern string) (string, error) {
	if len(pattern) > maxFormatSize {
		return "", fmt.Errorf("time format is too long")
	}
	var b strings.Builder
	for i := 0; i < len(pattern); {
		if pattern[i] == '\'' {
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			i++
			closed := false
			for i < len(pattern) {
				if pattern[i] == '\'' {
					if i+1 < len(pattern) && pattern[i+1] == '\'' {
						b.WriteByte('\'')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				b.WriteByte(pattern[i])
				i++
			}
			if !closed {
				return "", fmt.Errorf("unterminated time format quote")
			}
			continue
		}
		j := i + 1
		for j < len(pattern) && pattern[j] == pattern[i] {
			j++
		}
		width := j - i
		var value string
		number := func(v int) string {
			if width == 1 {
				return strconv.Itoa(v)
			}
			return fmt.Sprintf("%0*d", width, v)
		}
		switch pattern[i] {
		case 'y':
			if width == 2 {
				value = fmt.Sprintf("%02d", t.Year()%100)
			} else {
				value = number(t.Year())
			}
		case 'M':
			switch {
			case width >= 4:
				value = t.Month().String()
			case width == 3:
				value = t.Month().String()[:3]
			default:
				value = number(int(t.Month()))
			}
		case 'd':
			value = number(t.Day())
		case 'H':
			value = number(t.Hour())
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			value = number(h)
		case 'm':
			value = number(t.Minute())
		case 's':
			value = number(t.Second())
		case 'S':
			value = (fmt.Sprintf("%09d", t.Nanosecond()) + strings.Repeat("0", width))[:width]
		case 'a':
			value = "AM"
			if t.Hour() >= 12 {
				value = "PM"
			}
		case 'E':
			value = t.Weekday().String()
			if width < 4 {
				value = value[:3]
			}
		case 'w':
			_, week := t.ISOWeek()
			value = number(week)
		case 'D':
			value = number(t.YearDay())
		case 'Z':
			value = t.Format("-0700")
		case 'X':
			if _, offset := t.Zone(); offset == 0 {
				value = "Z"
			} else if width >= 3 {
				value = t.Format("-07:00")
			} else {
				value = t.Format("-0700")
			}
		case 'z':
			value, _ = t.Zone()
		default:
			value = pattern[i:j]
		}
		b.WriteString(value)
		i = j
	}
	return b.String(), nil
}

type sessionWindow struct{ open, close time.Time }

// Day digits use Pine's Sunday=1 convention. Overnight sessions are labelled
// by their trading (ending) day rather than the calendar day of their opening.
func sessionAt(t time.Time, spec string) (sessionWindow, bool, error) {
	if spec == "" || spec == "24x7" {
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return sessionWindow{start, start.AddDate(0, 0, 1)}, true, nil
	}
	parts := strings.Split(spec, ":")
	if len(parts) > 2 {
		return sessionWindow{}, false, fmt.Errorf("invalid session %q", spec)
	}
	days := "1234567"
	if len(parts) == 2 {
		days = parts[1]
	}
	if days == "" || strings.Trim(days, "1234567") != "" {
		return sessionWindow{}, false, fmt.Errorf("invalid session days %q", days)
	}
	parseClock := func(s string) (int, error) {
		if len(s) != 4 {
			return 0, fmt.Errorf("invalid session time %q", s)
		}
		h, e1 := strconv.Atoi(s[:2])
		m, e2 := strconv.Atoi(s[2:])
		if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return 0, fmt.Errorf("invalid session time %q", s)
		}
		return h*60 + m, nil
	}
	var match sessionWindow
	found := false
	for _, period := range strings.Split(parts[0], ",") {
		clocks := strings.Split(period, "-")
		if len(clocks) != 2 {
			return match, false, fmt.Errorf("invalid session %q", spec)
		}
		startMin, err := parseClock(clocks[0])
		if err != nil {
			return match, false, err
		}
		endMin, err := parseClock(clocks[1])
		if err != nil {
			return match, false, err
		}
		for _, day := range []int{-1, 0} {
			date := t.AddDate(0, 0, day)
			open := time.Date(date.Year(), date.Month(), date.Day(), startMin/60, startMin%60, 0, 0, t.Location())
			close := time.Date(date.Year(), date.Month(), date.Day(), endMin/60, endMin%60, 0, 0, t.Location())
			trading := date
			if endMin <= startMin {
				close = close.AddDate(0, 0, 1)
				if startMin != 0 {
					trading = trading.AddDate(0, 0, 1)
				}
			}
			if strings.ContainsRune(days, rune('1'+trading.Weekday())) && !t.Before(open) && t.Before(close) {
				match, found = sessionWindow{open, close}, true
			}
		}
	}
	return match, found, nil
}

func timeBucket(t time.Time, tf string, anchor time.Time) (time.Time, time.Time, error) {
	base, mult, ok := parseTimeframe(tf)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timeframe %q", tf)
	}
	loc := t.Location()
	switch base {
	case "T":
		return time.Time{}, time.Time{}, fmt.Errorf("tick timeframe has no predictable time boundary")
	case "M":
		month := (int(t.Month())-1)/mult*mult + 1
		open := time.Date(t.Year(), time.Month(month), 1, 0, 0, 0, 0, loc)
		close := open.AddDate(0, mult, 0)
		yearEnd := time.Date(t.Year()+1, 1, 1, 0, 0, 0, 0, loc)
		if close.After(yearEnd) {
			close = yearEnd
		}
		return open, close, nil
	case "W":
		monday := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(int(t.Weekday())+6)%7)
		firstMonday := func(year int) time.Time {
			start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
			return start.AddDate(0, 0, (8-int(start.Weekday()))%7)
		}
		start := firstMonday(monday.Year())
		weeks := (monday.YearDay() - start.YearDay()) / 7
		open := start.AddDate(0, 0, weeks/mult*mult*7)
		close := open.AddDate(0, 0, 7*mult)
		if yearEnd := firstMonday(monday.Year() + 1); close.After(yearEnd) {
			close = yearEnd
		}
		return open, close, nil
	case "D":
		date := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		if mult > 1 {
			date = date.AddDate(0, 0, -(t.YearDay()-1)%mult)
		}
		if !anchor.IsZero() {
			date = time.Date(date.Year(), date.Month(), date.Day(), anchor.Hour(), anchor.Minute(), 0, 0, loc)
		}
		close := date.AddDate(0, 0, mult)
		yearEnd := time.Date(t.Year()+1, 1, 1, date.Hour(), date.Minute(), 0, 0, loc)
		if close.After(yearEnd) {
			close = yearEnd
		}
		return date, close, nil
	default:
		seconds, _ := timeframeInSeconds(tf)
		if seconds <= 0 || int64(seconds) > int64(math.MaxInt64)/int64(time.Second) {
			return time.Time{}, time.Time{}, fmt.Errorf("timeframe is too large")
		}
		dayAligned := anchor.IsZero()
		if dayAligned {
			anchor = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
		step := time.Duration(seconds) * time.Second
		open := anchor.Add(time.Duration(math.Floor(float64(t.Sub(anchor))/float64(step))) * step)
		close := open.Add(step)
		dayEnd := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
		if dayAligned && close.After(dayEnd) {
			close = dayEnd
		}
		return open, close, nil
	}
}

func (r *Runtime) timeFunction(args []interface{}, closing bool) (interface{}, error) {
	if len(args) > 5 {
		return nil, fmt.Errorf("time/time_close expects up to 5 args")
	}
	tf, session, zone := r.timeframe, "", r.exchangeTimezone().String()
	if len(args) > 0 && args[0] != nil {
		s, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("timeframe must be a string")
		}
		if s != "" {
			tf = s
		}
	}
	barsBack, tfBack := 0, 0
	for i := 1; i < len(args); i++ {
		if args[i] == nil {
			continue
		}
		if i == 1 {
			if s, ok := args[i].(string); ok {
				session = s
				continue
			}
		}
		if i == 2 {
			s, ok := args[i].(string)
			if !ok {
				return nil, fmt.Errorf("timezone must be a string")
			}
			zone = s
			continue
		}
		n, ok := toFloat(args[i])
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < -500 || n > 1000000 {
			return nil, fmt.Errorf("bars_back must be an integer from -500 to 1000000")
		}
		if i == 4 {
			tfBack = int(n)
		} else {
			barsBack = int(n)
		}
	}
	loc, err := checkedTimezone(zone)
	if err != nil {
		return nil, err
	}
	idx := r.effectiveBarIndex() - barsBack
	if idx < 0 {
		return math.NaN(), nil
	}
	if len(args) == 0 {
		if closing {
			return float64(r.barCloseTime().UnixMilli()), nil
		}
		return float64(r.barOpenTime().UnixMilli()), nil
	}
	if normalizeTimeframe(tf) == r.timeframePeriod && session == "" && tfBack == 0 {
		open, close, _ := r.timesAtBar(idx)
		if closing {
			return float64(close.UnixMilli()), nil
		}
		return float64(open.UnixMilli()), nil
	}
	t, _, _ := r.timesAtBar(idx)
	t = t.In(loc)
	var anchor time.Time
	var window sessionWindow
	if session != "" {
		var present bool
		window, present, err = sessionAt(t, session)
		if err != nil {
			return nil, err
		}
		if !present {
			return math.NaN(), nil
		}
		anchor = window.open
	}
	open, close, err := timeBucket(t, tf, anchor)
	if err != nil {
		return nil, err
	}
	if tfBack != 0 {
		base, mult, _ := parseTimeframe(tf)
		switch base {
		case "M":
			open, close = open.AddDate(0, -tfBack*mult, 0), close.AddDate(0, -tfBack*mult, 0)
		case "W":
			open, close = open.AddDate(0, 0, -7*tfBack*mult), close.AddDate(0, 0, -7*tfBack*mult)
		case "D":
			open, close = open.AddDate(0, 0, -tfBack*mult), close.AddDate(0, 0, -tfBack*mult)
		default:
			step := close.Sub(open) * time.Duration(tfBack)
			open, close = open.Add(-step), close.Add(-step)
		}
	}
	if session != "" && tfBack == 0 && close.After(window.close) {
		close = window.close
	}
	if closing {
		return float64(close.UnixMilli()), nil
	}
	return float64(open.UnixMilli()), nil
}

func shiftTimeframe(t time.Time, steps int, tf string) time.Time {
	base, mult, ok := parseTimeframe(tf)
	if !ok {
		return t.Add(time.Duration(steps) * time.Minute)
	}
	switch base {
	case "M":
		return t.AddDate(0, steps*mult, 0)
	case "W":
		return t.AddDate(0, 0, steps*mult*7)
	case "D":
		return t.AddDate(0, 0, steps*mult)
	default:
		seconds, ok := timeframeInSeconds(tf)
		if !ok {
			seconds = 60
		}
		total := int64(steps) * int64(seconds)
		// Split long offsets so time.Duration cannot overflow at ~290 years.
		utc := t.UTC().AddDate(0, 0, int(total/86400))
		return utc.Add(time.Duration(total%86400) * time.Second)
	}
}
