<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

# API Reference

This document describes the public Go API for the Pinescription Pine Script v6 compiler and runtime. Pinescription is distributed as the Go package `github.com/woodstock-tokyo/pinescription`, typically imported as `pinego`.

The recommended integration style is:

1. Create an isolated `Engine` with `NewEngine()`
2. Register one or more `Provider` implementations
3. Compile Pine Script source with `Engine.Compile`
4. Execute bytecode with `Engine.Execute` or `Engine.ExecuteWithRuntime`

For runnable consumer-oriented examples, see the pkg.go.dev example tests in:

- `example_engine_test.go` (`ExampleNewEngine`, `ExampleEngine_ExecuteWithRuntime`)
- `series/examples_test.go` (`ExampleNewQueue`, `ExampleSeriesExtend_Mean`, `ExampleCrossOver`)

## Package-Level Functions (Default Engine)

The package provides a default engine instance for quick usage without explicitly creating an engine. These functions operate on shared global state and are suitable for simple applications or testing.

For reusable applications, services, or tests that should not share state, prefer `NewEngine()` instead of the package-level helpers.

### Compile

```go
func Compile(pinescript string) ([]byte, error)
```

Compiles Pine Script source code into bytecode. Returns the compiled bytecode as a byte slice, or an error if compilation fails. The compilation process includes parsing, validation, lowering, and encoding stages.

**Parameters:**
- `pinescript`: The Pine Script source code as a string.

**Returns:**
- `[]byte`: The compiled bytecode.
- `error`: Any compilation error, or nil on success.

### Execute

```go
func Execute(bytecode []byte) (interface{}, error)
```

Executes pre-compiled bytecode and returns the final value. This is a convenience function that wraps `Engine.Execute`. The execution processes all bars in the loaded series and returns the last expression value.

**Parameters:**
- `bytecode`: Compiled bytecode from `Compile`.

**Returns:**
- `interface{}`: The final computed value (typically a float64 for numeric results).
- `error`: Any execution error, or nil on success.

### RegisterFunction

```go
func RegisterFunction(name string, function func(args ...interface{}) (interface{}, error))
```

Registers a custom callable function that can be invoked from within Pine Script. The function receives arguments as `interface{}` and must return an `interface{}` and error. Namespaced Pine calls use exact names, so registering `strategy.order` makes `strategy.order(...)` callable even though unregistered strategy APIs remain unsupported.

Use `RegisterFunctionWithParamNames` instead when Pine code may call the registered function with named arguments, including unsupported hook points such as `plot(close, title = "Close")`.

**Parameters:**
- `name`: The name by which the function is callable in Pine Script.
- `function`: A function matching the signature `func(args ...interface{}) (interface{}, error)`.

### RegisterFunctionWithParamNames

```go
func RegisterFunctionWithParamNames(name string, paramNames []string, function func(args ...interface{}) (interface{}, error)) error
```

Registers a custom callable function on the package-level default engine and records parameter names for Pine Script named-argument binding. Positional calls are still passed through in source order; named calls are reordered according to `paramNames` before the Go function is invoked.

This is required for registered functions that may be called with named arguments. Without parameter metadata, a named-argument call to a registered function fails at runtime instead of silently forwarding values in source order.

The function name may be an ordinary custom function name such as `my_signal` or an exact unsupported feature hook point such as `plot`, `request.security`, or `strategy.order`. Registration returns an error when `name` is empty, parser-reserved, a Pine type keyword such as `int`, `float`, `color`, or `table`, or an already implemented built-in function such as `ta.rsi`. Each entry in `paramNames` must be non-empty and unique.

**Parameters:**
- `name`: The Pine Script function name to register, including namespace when hooking unsupported APIs (for example, `my_signal`, `plot`, or `request.security`).
- `paramNames`: Parameter names in the order expected by the Go function. Names must be non-empty and unique.
- `function`: A function matching the signature `func(args ...interface{}) (interface{}, error)`.

**Returns:**
- `error`: Validation error for invalid, reserved, type-keyword, or built-in function names, or invalid parameter names, or nil on success.

### RegisterMarketDataProvider

```go
func RegisterMarketDataProvider(provider Provider)
```

Registers a market data provider with the default engine. Multiple providers can be registered; symbols and value types are aggregated across all providers.

**Parameters:**
- `provider`: An implementation of the `Provider` interface.

### SetTimeframe

```go
func SetTimeframe(timeframe string)
```

Sets the timeframe for execution. Common values include "1D", "1H", "15", etc.

**Parameters:**
- `timeframe`: The timeframe string (e.g., "1D", "1H", "5").

### SetSession

```go
func SetSession(session string)
```

Sets the trading session type for execution (e.g., "24x7", "session").

**Parameters:**
- `session`: The session string.

### SetCurrentTime

```go
func SetCurrentTime(now time.Time)
```

Sets the current time context for execution. This is used for time-dependent calculations and time-related built-in variables.

**Parameters:**
- `now`: A `time.Time` value representing the current time.

### SetStartTime

```go
func SetStartTime(start time.Time)
```

Sets the start time for the execution window. This determines the beginning of the data range to process.

**Parameters:**
- `start`: A `time.Time` value representing the start time.

## Recommended Engine Workflow

The instance-based `Engine` API is the primary integration surface:

```go
engine := pinego.NewEngine()
engine.RegisterMarketDataProvider(&myProvider{})
engine.SetDefaultSymbol("AAPL")

bytecode, err := engine.Compile("close + 1")
if err != nil {
    // handle compile error
}

result, err := engine.Execute(bytecode)
if err != nil {
    // handle execution error
}
```

If you need post-execution inspection, use `Engine.ExecuteWithRuntime()` instead and read the returned `Runtime`.

## Engine Type

The `Engine` type is the recommended way to use Pinescription. It provides isolated instances with full control over configuration, making it suitable for production use with multiple independent script executions.

### NewEngine

```go
func NewEngine() *Engine
```

Creates a new, isolated engine instance. Each engine maintains its own state, providers, and configuration.

**Returns:**
- `*Engine`: A new engine instance.

### Engine.Compile

```go
func (e *Engine) Compile(pinescript string) ([]byte, error)
```

Compiles Pine Script source into bytecode using this engine instance.

**Parameters:**
- `pinescript`: The Pine Script source code.

**Returns:**
- `[]byte`: The compiled bytecode.
- `error`: Any compilation error.

### Engine.Execute

```go
func (e *Engine) Execute(bytecode []byte) (interface{}, error)
```

Executes bytecode and returns the final value. This is a convenience method that does not return the runtime directly, but it still updates the engine's latest runtime state. The runtime remains available via `Engine.Runtime()` until `ClearRuntime()` is called.

**Parameters:**
- `bytecode`: Compiled bytecode.

**Returns:**
- `interface{}`: The final computed value.
- `error`: Any execution error.

### Engine.ExecuteWithRuntime

```go
func (e *Engine) ExecuteWithRuntime(bytecode []byte) (*Runtime, interface{}, error)
```

Executes bytecode and returns the runtime for post-execution inspection. This method provides access to the full runtime state, including variable values, series data, and execution metadata.

**Parameters:**
- `bytecode`: Compiled bytecode.

**Returns:**
- `*Runtime`: The runtime state holder.
- `interface{}`: The final computed value.
- `error`: Any execution error.

### Engine.RegisterFunction

```go
func (e *Engine) RegisterFunction(name string, fn UserFunction)
```

Registers a custom callable function with this engine. The function must implement `UserFunction`, which is defined as `func(args ...interface{}) (interface{}, error)`. Namespaced Pine calls use exact names, so registering `strategy.order` makes `strategy.order(...)` callable even though unregistered strategy APIs remain unsupported.

Use `Engine.RegisterFunctionWithParamNames` instead when Pine code may call the registered function with named arguments, including unsupported hook points such as `plot(close, title = "Close")`.

**Parameters:**
- `name`: The function name callable from Pine Script.
- `fn`: The function implementation.

### Engine.RegisterFunctionWithParamNames

```go
func (e *Engine) RegisterFunctionWithParamNames(name string, paramNames []string, fn UserFunction) error
```

Registers a custom callable function with this engine and records parameter names for named-argument binding. Positional calls are passed through in source order; named calls are reordered according to `paramNames` before invoking `fn`.

Use this for ordinary custom functions when scripts may call them with named arguments, and for exact external hook points such as `plot` or `request.security`. The runtime treats unregistered `request.*`, `strategy.*`, and plotting calls as unsupported features; an exact-name hook makes only that registered call executable.

Registration validates `name` and returns an error when it is empty, parser-reserved, an ordinary Pine type keyword, or an implemented built-in function. Valid names include ordinary custom names such as `my_signal` and exact unsupported hook targets such as `plot` or `request.security`. Rejected-name examples include `ta.rsi`, `rsi`, `if`, `for`, `int`, `float`, `color`, and `table`. Each entry in `paramNames` must also be non-empty and unique so named-argument binding is unambiguous.

Calling `Engine.RegisterFunction` later with the same `name` replaces the function and clears the parameter metadata.

**Parameters:**
- `name`: The Pine Script function name to register, including namespace when hooking unsupported APIs.
- `paramNames`: Parameter names in the order expected by `fn`. Names must be non-empty and unique.
- `fn`: The function implementation.

**Returns:**
- `error`: Validation error for invalid, reserved, type-keyword, or built-in function names, or invalid parameter names, or nil on success.

### Engine.RegisterMarketDataProvider

```go
func (e *Engine) RegisterMarketDataProvider(provider Provider)
```

Registers a market data provider with this engine. Multiple providers can be registered to aggregate symbols and value types.

**Parameters:**
- `provider`: A `Provider` implementation.

### Engine.SetDefaultSymbol

```go
func (e *Engine) SetDefaultSymbol(symbol string)
```

Sets the default symbol for resolution when accessing price data without an explicit symbol qualifier.

**Parameters:**
- `symbol`: The default symbol (e.g., "AAPL", "GOOGL").

### Engine.SetDefaultValueType

```go
func (e *Engine) SetDefaultValueType(valueType string)
```

Sets the default value type for resolution when accessing series data without an explicit value type.

**Parameters:**
- `valueType`: The default value type (e.g., "close", "open", "high", "low", "volume").

### Engine.SetTimeframe

```go
func (e *Engine) SetTimeframe(timeframe string)
```

Sets the execution timeframe. This propagates to all registered providers.

**Parameters:**
- `timeframe`: The timeframe string.

### Engine.SetSession

```go
func (e *Engine) SetSession(session string)
```

Sets the trading session. This propagates to all registered providers.

**Parameters:**
- `session`: The session string.

### Engine.Timeframe

```go
func (e *Engine) Timeframe() string
```

Returns the currently configured timeframe string for this engine.

### Engine.Session

```go
func (e *Engine) Session() string
```

Returns the currently configured session string for this engine.

### Engine.SetCurrentTime

```go
func (e *Engine) SetCurrentTime(now time.Time)
```

Sets the current wall-clock time used during execution. This is most useful for deterministic tests or replaying historical execution with a fixed "now".

### Engine.CurrentTime

```go
func (e *Engine) CurrentTime() time.Time
```

Returns the currently configured execution time.

### Engine.SetStartTime

```go
func (e *Engine) SetStartTime(start time.Time)
```

Sets the timestamp of the first bar in the execution window. The runtime combines this with timeframe information to derive per-bar timestamps.

### Engine.StartTime

```go
func (e *Engine) StartTime() time.Time
```

Returns the configured start time.

### Engine.Logs

```go
func (e *Engine) Logs() []EngineLogEntry
```

Returns a copy of execution log entries collected by the engine during the last run.

### Engine.ClearLogs

```go
func (e *Engine) ClearLogs()
```

Clears any retained engine log entries.

### Engine.SetAlertSink

```go
func (e *Engine) SetAlertSink(func(AlertEvent))
```

Registers a callback function to receive alerts triggered by `alert()` and `alertcondition()` in Pine Script.

**Parameters:**
- A function that receives `AlertEvent` objects.

### Engine.ClearRuntime

```go
func (e *Engine) ClearRuntime()
```

Releases retained runtime state from the previous execution. Call this to free memory when done inspecting runtime state.

### Engine.Runtime

```go
func (e *Engine) Runtime() *Runtime
```

Returns the runtime from the most recent execution, or nil if no execution has occurred.

**Returns:**
- `*Runtime`: The latest runtime, or nil.

### Engine.Symbols

```go
func (e *Engine) Symbols() ([]string, error)
```

Returns all symbols known across all registered providers, sorted alphabetically.

**Returns:**
- `[]string`: Sorted list of symbol names.
- `error`: Error if no providers are registered.

### Engine.ValueTypes

```go
func (e *Engine) ValueTypes() ([]string, error)
```

Returns all value types supported by registered providers, sorted alphabetically.

**Returns:**
- `[]string`: Sorted list of value type names.
- `error`: Error if no providers are registered.

## Provider Interface

The `Provider` interface must be implemented to supply market data to the runtime:

```go
type Provider interface {
    GetSeries(seriesKey string) (SeriesExtended, error)
    GetSymbols() ([]string, error)
    GetValuesTypes() ([]string, error)
    SetTimeframe(timeframe string) error
    GetTimeframe() string
    SetSession(session string) error
    GetSession() string
}
```

### GetSeries

```go
GetSeries(seriesKey string) (SeriesExtended, error)
```

Retrieves a time series for the given key. The `seriesKey` format is `symbol + "|" + value_type`, for example `"AAPL|close"`, `"GOOGL|volume"`, or `"MSFT|high"`.

**Parameters:**
- `seriesKey`: A string in the format `"symbol|value_type"`.

**Returns:**
- `SeriesExtended`: The time series data.
- `error`: Any error loading the series.

### GetSymbols

```go
GetSymbols() ([]string, error)
```

Returns all symbols available from this provider.

**Returns:**
- `[]string`: List of symbol names.
- `error`: Any error.

### GetValuesTypes

```go
GetValuesTypes() ([]string, error)
```

Returns all value types available from this provider for the set of symbols it provides.

**Returns:**
- `[]string`: List of value type names (e.g., "close", "open", "high", "low", "volume").
- `error`: Any error.

### SetTimeframe

```go
SetTimeframe(timeframe string) error
```

Sets the timeframe on the provider. Called by the engine before execution.

**Parameters:**
- `timeframe`: The timeframe string.

**Returns:**
- `error`: Any error setting the timeframe.

### GetTimeframe

```go
GetTimeframe() string
```

Returns the current timeframe from the provider.

**Returns:**
- `string`: The current timeframe.

### SetSession

```go
SetSession(session string) error
```

Sets the trading session on the provider.

**Parameters:**
- `session`: The session string.

**Returns:**
- `error`: Any error setting the session.

### GetSession

```go
GetSession() string
```

Returns the current session from the provider.

**Returns:**
- `string`: The current session.

## Provider Resolution

The engine supports registering multiple providers simultaneously. When multiple providers are registered:

1. **Symbol Aggregation**: All symbols across providers are combined and deduplicated. `Engine.Symbols()` returns the union of all provider symbols.

2. **Value Type Aggregation**: All value types across providers are combined and deduplicated. `Engine.ValueTypes()` returns the union of all provider value types.

3. **Bytecode Dependency Resolution**: During compilation, the engine analyzes the script to determine required symbols and value types. At execution time, the engine resolves each required series key against the registered providers.

4. **Error on Missing References**: If the bytecode references a symbol or value type that is not available in any registered provider, execution fails with an appropriate error message.

5. **Engine-Level Settings Propagation**: When `Engine.SetTimeframe()` or `Engine.SetSession()` is called, the values propagate to all registered providers that implement these setter methods.

6. **Default Selection**: If `SetDefaultSymbol()` or `SetDefaultValueType()` is not explicitly called, the engine uses the first available symbol or value type from the provider catalog as the default.

## Runtime State Holder API

The `Runtime` type holds the state from a single script execution. It is returned by `Engine.ExecuteWithRuntime()` and is also accessible via `Engine.Runtime()` after execution.

### Runtime.SetBarIndex

```go
func (r *Runtime) SetBarIndex(i int)
```

Advances the runtime to a specific bar index and refreshes cached bar-state values. This is called automatically by the engine during execution, but it is also exported for step-through inspection and replay-style tooling.

### Runtime.Snapshot

```go
func (r *Runtime) Snapshot() RuntimeSnapshot
```

Returns a snapshot of the runtime state at the final bar. The snapshot contains:

- `BarIndex`: The current bar index (0-based).
- `LastValue`: The final computed value from the script.
- `ActiveSymbol`: The symbol used as the default for the execution.
- `ActiveValueType`: The value type used as the default.
- `Symbols`: List of symbols known to the runtime.
- `SeriesKeys`: List of series keys loaded during execution.
- `Variables`: A map of variable names to their final values.

**Returns:**
- `RuntimeSnapshot`: A copy of the runtime state.

### Runtime.Release

```go
func (r *Runtime) Release()
```

Clears retained references and internal maps to free memory. Call this when the runtime is no longer needed.

### Runtime.Symbols

```go
func (r *Runtime) Symbols() []string
```

Returns all symbols known in this runtime, sorted alphabetically.

**Returns:**
- `[]string`: Sorted list of symbol names.

### Runtime.SeriesKeys

```go
func (r *Runtime) SeriesKeys() []string
```

Returns all series keys loaded or known in this runtime, sorted alphabetically.

**Returns:**
- `[]string`: Sorted list of series keys (e.g., "AAPL|close", "GOOGL|volume").

### Runtime.ValueTypes

```go
func (r *Runtime) ValueTypes(symbol string) []string
```

Returns the known value types for a given symbol within this runtime.

**Parameters:**
- `symbol`: The symbol to query.

**Returns:**
- `[]string`: Sorted list of value types for the symbol, or nil if the symbol is unknown.

### Runtime.Series

```go
func (r *Runtime) Series(seriesKey string) (SeriesExtended, bool)
```

Returns the series for a given key, if available.

**Parameters:**
- `seriesKey`: The series key in `"symbol|value_type"` format.

**Returns:**
- `SeriesExtended`: The series data.
- `bool`: True if the series was found, false otherwise.

### Runtime.Value

```go
func (r *Runtime) Value(name string) (interface{}, bool)
```

Returns the latest value of a variable by name, including both scalar variables and historical series.

**Parameters:**
- `name`: The variable name.

**Returns:**
- `interface{}`: The latest value.
- `bool`: True if the variable was found, false otherwise.

## Alerts

Pinescription supports the `alert()` and `alertcondition()` Pine Script built-in functions. When these functions are called during execution, they emit events to a registered alert sink rather than delivering alerts in the TradingView style.

### AlertEvent

```go
type AlertEvent struct {
    Message   string
    Frequency string    // Optional: "all", "once_per_bar", "once_per_bar_close"
    BarIndex  int
    Time      time.Time // UTC time of the bar
    Symbol    string
}
```

The `AlertEvent` struct contains the details of a triggered alert.

## Engine Logs

In addition to returning values and runtime state, an `Engine` also retains structured log entries from the most recent execution. Use `Engine.Logs()` to retrieve them and `Engine.ClearLogs()` to clear them before or after a run.

```go
type EngineLogEntry struct {
    Timestamp time.Time
    Level     string
    Message   string
}
```

`Level` is one of `"info"`, `"warning"`, or `"error"`.

## Runnable Examples

The following runnable examples are part of the test suite and are rendered by pkg.go.dev:

- `ExampleNewEngine` — create an engine, register a provider, compile, and execute
- `ExampleEngine_ExecuteWithRuntime` — inspect the returned runtime snapshot
- `ExampleNewQueue` — show queue indexing and fixed-size buffer behavior
- `ExampleSeriesExtend_Mean` — compute means from `series.SeriesExtend`
- `ExampleCrossOver` — demonstrate crossover detection in the `series` package

### Setting Up Alert Handling

To receive alerts, register a callback with the engine:

```go
engine.SetAlertSink(func(event pinego.AlertEvent) {
    fmt.Printf("Alert on %s: %s (bar %d)\n", event.Symbol, event.Message, event.BarIndex)
})
```

## Usage Example

The following example demonstrates creating an engine, registering a provider, compiling a script, and executing it:

```go
package main

import (
    "fmt"
    "log"
    pinego "github.com/woodstock-tokyo/pinescription"
)

func main() {
    // Create a new engine
    engine := pinego.NewEngine()

    // Register your market data provider
    engine.RegisterMarketDataProvider(&myProvider{})

    // Set default symbol and value type
    engine.SetDefaultSymbol("AAPL")
    engine.SetDefaultValueType("close")

    // Optionally set timeframe and session
    engine.SetTimeframe("1D")
    engine.SetSession("24x7")

    // Compile a Pine Script
    script := `
var ma = sma(close, 20)
var ex = ema(close, 20)
ma + ex
`
    bytecode, err := engine.Compile(script)
    if err != nil {
        log.Fatal(err)
    }

    // Execute with runtime access
    rt, result, err := engine.ExecuteWithRuntime(bytecode)
    if err != nil {
        log.Fatal(err)
    }

    // Inspect results
    fmt.Println("Result:", result)
    fmt.Println("Bar index:", rt.Snapshot().BarIndex)
    fmt.Println("Active symbol:", rt.Snapshot().ActiveSymbol)

    // Access runtime state
    symbols := rt.Symbols()
    fmt.Println("Known symbols:", symbols)

    series, found := rt.Series("AAPL|close")
    if found {
        fmt.Println("Close series length:", series.Length())
    }
}
```

This example shows the typical workflow: create an engine, configure it with providers and settings, compile your script, execute it with runtime inspection, and then access the computed results and runtime state. For shorter, verified examples that run under `go test`, see the pkg.go.dev example tests listed above.
