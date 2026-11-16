<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

<p align="center"><img src="./logo.png" alt="Pinescription" width="600"></p>

<h1 align="center">Pinescription</h1>

<p align="center">A high-performance Pine Script v6 compiler and runtime for Go.</p>

<p align="center">Pinescription compiles Pine Script code into optimized bytecode and executes it against market data providers, enabling you to run TradingView indicators and strategies in production Go applications without webhooks or external dependencies.</p>

---

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go" alt="Go version"></a>
  <a href=".github/workflows/pr-main.yml"><img src="https://img.shields.io/github/actions/workflow/status/woodstock-tokyo/pinescription/pr-main.yml?style=flat-square" alt="Build"></a>
  <a href="#license"><img src="https://img.shields.io/badge/License-AGPL--3.0%20%2F%20Commercial-blue?style=flat-square" alt="License"></a>
  <a href="https://pkg.go.dev/github.com/woodstock-tokyo/pinescription"><img src="https://img.shields.io/badge/GoDoc-reference-5272B4?style=flat-square&logo=go" alt="GoDoc"></a>
</p>

---

<p align="center">
  <img src="./assets/demo.gif" alt="Pinescription live demo — BTC/USDT chart with RSI, SMA, and a custom styled indicator compiled from Pine Script" width="720">
</p>

---

## Why Pinescription

- **No webhooks, no lock-in.** Run Pine Script indicators directly in your Go backend. Connect to any broker API through a simple provider interface.
- **Native Go performance.** The compiler generates optimized bytecode with streaming evaluation. Benchmarks show 164x speedup over full recomputation.
- **Pine Script v6 compatible.** Supports language core, arrays, matrices, tuples, 30+ indicators, and the `ta.*` namespace.
- **Single dependency.** Built on gonum for linear algebra. No other external dependencies.

## Quick Start

**Step 1: Install**

```bash
go get github.com/woodstock-tokyo/pinescription
```

**Step 2: Write your first indicator**

```go
package main

import (
    "fmt"
    pinego "github.com/woodstock-tokyo/pinescription"
)

func main() {
    engine := pinego.NewEngine()
    engine.RegisterMarketDataProvider(&yourProvider{})
    engine.SetDefaultSymbol("AAPL")

    script := `
var ma = sma(close, 20)
var ex = ema(close, 20)
ma + ex
`

    bytecode, err := engine.Compile(script)
    if err != nil {
        panic(err)
    }
    result, err := engine.Execute(bytecode)
    if err != nil {
        panic(err)
    }
    fmt.Println("Result:", result)
}
```

**Step 3: Explore more examples**

See `examples/basic/main.go` for a working demo with sample market data, or `examples/volume_profile_pivot_anchored/` for a complex Pine Script v6 execution.

---

## Performance

Benchmarked on Apple M3:

| Benchmark | Result | Notes |
|-----------|--------|-------|
| Bollinger Bands (streaming) | 634 ns/op | 164x faster than full recompute |
| 5000-bar indicators | ~5.8 ms/op | Optimized sliding window |
| Fisher Transform (5000 bars) | ~10.9 ms/op | Complex multi-step indicator |

The streaming execution model updates only the active bar state, avoiding full recalculation on each iteration.

---

## Features

- **Language Core**: `var`/`const` declarations, `if`/`else`/`for`/`while`/`switch`, functions, arrays, tuples, matrices
- **30+ Built-in Indicators**: SMA, EMA, RSI, ATR, Bollinger Bands, crossover, crossunder, stdev, correlation...
- **TA and Volume APIs**: Moving averages, oscillators, RCI, anchored VWAP/bands,
  OBV, accumulation/distribution, and other volume series
- **`math.*` Namespace**: `math.abs`, `math.pow`, `math.sqrt`, `math.log`, `math.sin`...
- **Array Operations**: 25+ methods including `push`, `pop`, `sort`, `sum`, `avg`...
- **Matrix Operations**: Linear algebra helpers, eigenvalues, determinant, transpose...
- **String Operations**: `str.format`, `str.split`, `str.replace`, `str.contains`...
- **Market Data**: Multi-symbol queries (`close_of`, `sma_of`...), history indexing (`close[1]`)
- **Alerts**: `alert()` and `alertcondition()` with custom sinks
- **Pre-Execution Inputs**: Discover parameters and resolve price, text,
  timeframe, enum, and source choices before runtime creation
- **Optional Market Context**: Provider-supplied symbol metadata, actual bar
  timestamps, and explicit session schedules
- **Versioned Libraries**: Explicit source resolution and linked imports/exports
- **Historical Requests and Order Flow**: Isolated security calculations,
  lower-timeframe arrays, and provider-backed footprint rows
- **Historical Strategy Broker**: Basic order execution and trade accounting,
  with [explicit limitations](docs/features.md#native-historical-strategies--limited-broker-model)

See [docs/features.md](docs/features.md) for the complete feature reference.

---

## Documentation

| Document | Description |
|----------|-------------|
| [API Reference](docs/api.md) | Complete API surface: Engine, Runtime, Provider, Alerts |
| [Architecture and Design](docs/architecture.md) | Compilation pipeline, execution flow, streaming evaluation model |
| [Feature Reference](docs/features.md) | Exhaustive list of supported Pine Script v6 features |
| [PineTS API Work Order](docs/api-parity.md) | Completed additions, explicit exclusions, and remaining API work |

---

## Architecture

Pinescription compiles Pine Script source to intermediate representation, then encodes it as bytecode. At runtime, the VM evaluates the bytecode bar-by-bar against market data from your provider. See [docs/architecture.md](docs/architecture.md) for detailed flow diagrams.

---

## Provider Interface

Your market data provider implements the `Provider` interface:

```go
type Provider interface {
    GetSeries(seriesKey string) (SeriesExtended, error)  // seriesKey = "symbol|value_type"
    GetSymbols() ([]string, error)
    GetValuesTypes() ([]string, error)
    SetTimeframe(timeframe string) error
    GetTimeframe() string
    SetSession(session string) error
    GetSession() string
}
```

Common series keys include `AAPL|close`, `AAPL|volume`, etc.

---

## Remaining Gaps and Custom Function Hooks

- Advanced strategy behavior: margin liquidation, trailing stops, recalculation,
  cross-currency accounting, and remaining risk rules
- Financial/economic/corporate-action request data (`request.financial`...)
- Plot APIs (`plot`, `plotshape`...)

Unsupported behavior returns explicit errors. Native historical strategies,
`request.security`, `request.security_lower_tf`, and `request.footprint` are now
available for the documented subset; this is not full TradingView parity.
Exact-name strategy/request hooks retain precedence, including
`RegisterFunction("strategy.order", fn)` for positional hooks and
`RegisterFunctionWithParamNames(...)` for host-defined named arguments. Plot
hooks continue to work as before. Ordinary non-hookable built-ins and reserved
names remain protected.

Host applications may also register selected drawing-object hook points such as `polyline.new`, `box.new`, `label.new`, `chart.point.from_index`, `table.cell`, and `table.clear`. Pinescription provides compatibility stubs for those APIs, but a host hook can capture the calls and render the resulting objects outside the runtime.

---

## License

Pinescription is dual-licensed under **AGPL-3.0-only** and a commercial license.

- AGPL-3.0-only: See `LICENSES/AGPL-3.0-only.txt`
- Commercial: See `LICENSES/LICENSE-COMMERCIAL.md`

---

## Security

See `SECURITY.md` for vulnerability reporting.

---

## Built by

Built by [Woodstock K.K.](https://woodstock.co)

> _Pine Script™ is a trademark of TradingView. Pinescription is an independent project and is not affiliated with, endorsed by, or associated with TradingView._

<a href="https://x.com/woodstockapp"><img src="https://img.shields.io/twitter/url?url=https%3A%2F%2Fx.com%2Fwoodstockapp&style=flat-square&logo=X&label=Follow" alt="Follow on X"></a>
