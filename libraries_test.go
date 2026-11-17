// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"strings"
	"testing"
)

func TestLibraryLinkingAndFrozenBytecode(t *testing.T) {
	sources := LibrarySources{
		"me/Base/1": `library("Base")
const float scale = 2
hidden(x) => x * scale
export twice(float x) => hidden(x)`,
		"me/Tools/1": `library("Tools")
import me/Base/1 as base
export add(float x, float y = 1) => base.twice(x) + y
export enum Side
    up = "Up"
    down = "Down"
export type Point
    float x
    float y = 2
export point(float x) => Point.new(x)`,
	}
	e := NewEngine()
	e.SetLibraryResolver(sources)
	code, err := e.Compile(`import me/Tools/1 as tools
import me/Base/1 as other
p = tools.point(5)
side = tools.Side.up
result = tools.add(y = 3, x = close) + other.twice(p.x) + p.y
title = str.tostring(side)`)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve only while compiling; execution uses the linked bytecode.
	sources["me/Base/1"] = `library("Broken")`
	e.SetLibraryResolver(nil)
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	rt, _, err := e.ExecuteWithRuntime(code)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	if got, _ := rt.Value("result"); got != float64(21) {
		t.Fatalf("got %v,want 21", got)
	}
	if got, _ := rt.Value("title"); got != "Up" {
		t.Fatal(got)
	}
	// Decoding in an independent engine must preserve linked types/functions.
	other := NewEngine()
	other.RegisterMarketDataProvider(providerWithClose("TEST", 1, 2, 3))
	if got, err := other.Execute(code); err != nil || got != float64(21) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestImportedMethods(t *testing.T) {
	e := NewEngine()
	e.SetLibraryResolver(LibrarySources{"me/Points/1": `library("Points")
export type Point
    float x
export method twice(Point self) => self.x * 2`})
	code, err := e.Compile(`import me/Points/1 as p
point = p.Point.new(3)
result = point.twice()`)
	if err != nil {
		t.Fatal(err)
	}
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
	if got, err := e.Execute(code); err != nil || got != float64(6) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestImportedMethodWinsOverUnqualifiedFunction(t *testing.T) {
	e := NewEngine()
	e.SetLibraryResolver(LibrarySources{"me/Counters/1": `library("Counters")
export type Counter
    float x
export method value(Counter self) => self.x * 2`})
	code, err := e.Compile(`import me/Counters/1 as lib
value(x) => 99
obj = lib.Counter.new(3)
result = obj.value()`)
	if err != nil {
		t.Fatal(err)
	}
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
	if got, err := e.Execute(code); err != nil || got != float64(6) {
		t.Fatalf("got %v, %v; want 6", got, err)
	}
}

func TestLibraryDeclarationsRequireTopLevel(t *testing.T) {
	for _, script := range []string{
		"if true\n    import me/A/1 as a",
		"unused() =>\n    import me/A/1 as a",
		"if true\n    export f(x) => x",
		"if true\n    export type Point\n        float x",
	} {
		_, err := NewEngine().Compile(script)
		if err == nil || !strings.Contains(err.Error(), "must appear at top level") {
			t.Fatalf("%s: got %v, want compile-time scope error", script, err)
		}
	}
}

func TestLibraryErrorsAndPrivateMembers(t *testing.T) {
	for _, tc := range []struct {
		source    string
		libraries LibrarySources
		want      string
	}{
		{`import me/A/1 as a`, nil, "LibraryResolver"},
		{`import me/A/0 as a`, nil, "positive integer"},
		{`import me/A/1 as ta`, LibrarySources{"me/A/1": `library("A")`}, "built-in namespace"},
		{`import me/A/1 as a
import me/A/1 as a`, LibrarySources{"me/A/1": `library("A")`}, "duplicate library alias"},
		{`import me/A/1 as a`, LibrarySources{"me/A/1": `import me/B/1 as b
library("A")`, "me/B/1": `import me/A/1 as a
library("B")`}, "cyclic"},
		{`import me/A/1 as a`, LibrarySources{"me/A/1": `export f(x) => x`}, "no library declaration"},
		{`import me/A/1 as a`, LibrarySources{"me/A/1": `library("A")
var x = close
export f() => x`}, "must be constant"},
		{`export f(x) => x`, nil, "requires a library"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			e := NewEngine()
			if tc.libraries != nil {
				e.SetLibraryResolver(tc.libraries)
			}
			_, err := e.Compile(tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v,want %s", err, tc.want)
			}
		})
	}
	e := NewEngine()
	e.SetLibraryResolver(LibrarySources{"me/A/1": `library("A")
hidden(x) => x
export f(x) => hidden(x)`})
	code, err := e.Compile(`import me/A/1 as a
a.hidden(1)`)
	if err != nil {
		t.Fatal(err)
	}
	e.RegisterMarketDataProvider(providerWithClose("TEST", 1))
	if _, err = e.Execute(code); err == nil {
		t.Fatal("private function exposed")
	}
}
