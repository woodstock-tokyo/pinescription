<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

# Supported Features

Reference of supported Pine Script v6 features and their execution limits.
Pine Script v6 defines the intended behavior, not every PineTS fallback or
extension. This is a supported subset, not a claim of complete TradingView
compatibility. Provider requirements, compatibility extensions, and
[unsupported features](#unsupported-features) are documented below.

## Language Core

Pinescription supports the fundamental Pine Script language constructs for variable management, operators, control flow, and composite values.

### Variable Declaration and Assignment

- `var` - Declare a variable that preserves its value across iterations
- `const` - Declare a compile-time constant

### Scalar Types

- `int` - Integer values
- `float` - Floating-point values
- `bool` - Boolean values (`true`, `false`)
- `string` - Text values
- `na` - Not available (null/missing value)

### Operators

- Arithmetic operators: `+`, `-`, `*`, `/`, `%` (modulo), `^` (power)
- Logical operators: `and`, `or`, `not`
- Comparison operators: `==`, `!=`, `<`, `>`, `<=`, `>=`
- Ternary conditional operator: `cond ? a : b`

### Control Flow

- `if` / `else` - Conditional branching
- `for` - Iterative loops with counter
- `while` - Conditional loops
- `switch` - Multi-way branch selection
- `break` - Exit loop early
- `continue` - Skip to next iteration
- `return` - Exit function early

### Functions

- Arrow functions: `x => x * 2`
- Block functions: `function foo(x) { return x * 2 }`

### Composite Values

- Arrays - Ordered collections of values
- Tuples - Immutable heterogeneous collections
- Matrices - Two-dimensional collections

## Series and Market Data

Pinescription provides access to OHLCV data and related market information through built-in variables and series history indexing.

### Built-in Variables

- `open` - Opening price of the current bar
- `high` - Highest price of the current bar
- `low` - Lowest price of the current bar
- `close` - Closing price of the current bar
- `volume` - Trading volume of the current bar
- `hl2` - Average of high and low: `(high + low) / 2`
- `hlc3` - Average of high, low, and close: `(high + low + close) / 3`
- `hlcc4` - Average of high, low, and twice the close: `(high + low + close * 2) / 4`
- `ohlc4` - Average of open, high, low, and close: `(open + high + low + close) / 4`
- `bar_index` - Sequential index of the bar (0-based)
- `last_bar_index`, `last_bar_time` - Index and actual/configured opening time of
  the final loaded bar
- `bid`, `ask` - Provider-supplied values on `1T` only; otherwise `na` (no fallback
  to close prices)

### Market Context and Historical State

Optional `SymbolInfoProvider`, `BarTimeProvider`, and `MarketSessionsProvider`
interfaces extend the existing Provider without changing it. Hosts can instead
use `Engine.SetSymbolInfo`, `SetBarTimes`, and `SetMarketSessions`.

- `syminfo` exposes symbol identity, pricing, currency, timezone, session, and
  supplied metadata fields. Missing numeric metadata is `na`, missing text is
  empty, and the default clock timezone is explicitly UTC. Tick size is **not**
  invented. `syminfo.prefix(symbol)` and `syminfo.ticker(symbol)` extract parts
  of prefixed symbols, including encoded ticker IDs.
- All seven `barstate` flags are available, including history indexing. Engine
  execution is historical: `ishistory`, `isnew`, and `isconfirmed` are true;
  `isrealtime` is false. The loaded endpoints determine `isfirst`, `islast`, and
  `islastconfirmedhistory`. This is not a live-tick rollback engine.
- `session.ismarket`, `ispremarket`, `ispostmarket`, `isfirstbar`,
  `islastbar`, `isfirstbar_regular`, and `islastbar_regular` use explicit weekly
  schedules in the exchange timezone. Querying these without a regular
  `MarketSessions` schedule is an error. Adjacent pre/regular/post windows form
  an extended session. No exchange holidays or special closes are inferred.
- Calendar variables and `year(timestamp[, timezone])`, `month`, `weekofyear`,
  `dayofmonth`, `dayofweek`, `hour`, `minute`, and `second` use the configured
  exchange timezone unless an explicit timezone is passed.

### Series History Indexing

Access past values of series-like variables using bracket notation with an integer offset.

- `close[1]` - Previous bar's close price
- `x[2]` - Value of variable `x` from 2 bars ago
- `close_of("AAPL")[1]` - Previous bar's close for a specific symbol

### Cross-Symbol Built-Ins

Access data from other symbols using these functions:

- `value_of(symbol, expression)` - Evaluate an expression for a specific symbol
- `close_of(symbol)` - Close price of the current bar for a symbol
- `open_of(symbol)` - Open price of the current bar for a symbol
- `high_of(symbol)` - High price of the current bar for a symbol
- `low_of(symbol)` - Low price of the current bar for a symbol
- `sma_of(symbol, length)` - Simple moving average for a symbol
- `ema_of(symbol, length)` - Exponential moving average for a symbol
- `rsi_of(symbol, length)` - Relative Strength Index for a symbol

## Indicators and Numeric Helpers

Pinescription implements popular technical indicators and utility functions for numeric operations.

### Built-in Indicators

- `sma(source, length)` - Simple Moving Average
- `ema(source, length)` - Exponential Moving Average
- `rsi(source, length)` - Relative Strength Index
- `atr(length)` - Average True Range
- `bb(source, length, mult)` - Bollinger Bands (returns upper, middle, lower)
- `bbw(source, length, mult)` - Bollinger Bands Width
- `crossover(source1, source2)` - Returns true when source1 crosses above source2
- `crossunder(source1, source2)` - Returns true when source1 crosses below source2
- `change(source, length)` - Difference between current and past values
- `highest(source, length)` - Highest value over a period
- `lowest(source, length)` - Lowest value over a period
- `stdev(source, length)` - Standard deviation
- `correlation(source1, source2, length)` - Pearson correlation coefficient

### ta Namespace Aliases

All built-in indicators are also available under the `ta` namespace:

- `ta.sma`, `ta.ema`, `ta.rsi`, `ta.atr`
- `ta.bb`, `ta.bbw`
- `ta.crossover`, `ta.crossunder`
- `ta.change`, `ta.highest`, `ta.lowest`
- `ta.stdev`, `ta.correlation`

### Additional TA APIs

- Volume variables: `ta.accdist`, `ta.iii`, `ta.nvi`, `ta.obv`, `ta.pvi`,
  `ta.pvt`, `ta.wad`, and `ta.wvad`. These are bare series, not function calls.
  Cumulative calculations support history access and do not double-count a
  repeated evaluation of the current bar. NVI/PVI begin at 1; OBV/PVT have an
  `na` first value because no preceding close exists.
- `ta.rci(source, length)` computes Pearson correlation of chronological ranks
  with average ranks for tied prices, scaled to -100..100. Like the scanned
  PineTS implementation, it warms up through `bar_index < length` and returns
  `na` for constant or `na`-containing windows.
- `ta.vwap` uses `hlc3`. `ta.vwap(source)` resets at each exchange-calendar date.
  `ta.vwap(source, anchor)` starts at the first true anchor and resets whenever
  the anchor is true. `ta.vwap(source, anchor, stdev_mult)` returns
  `[vwap, upper, lower]`, using volume-weighted population deviation.
  Zero volume or unavailable source/volume yields `na` for that bar without
  contaminating the accumulated sums used by later valid bars. Calls, including
  zero-argument `ta.vwap()` calls, have independent
  state; current-bar re-evaluation does not accumulate twice. Exchange-calendar
  daily resets are not an inferred futures/overnight trading-day calendar.

### math Namespace

A subset of mathematical functions from the `math` namespace:

- `math.abs(x)` - Absolute value
- `math.max(a, b)` - Maximum of two values
- `math.min(a, b)` - Minimum of two values
- `math.round(x)` - Round to nearest integer
- `math.floor(x)` - Round down to integer
- `math.ceil(x)` - Round up to integer
- `math.pow(base, exponent)` - Power operation
- `math.sqrt(x)` - Square root
- `math.log(x)` - Natural logarithm
- `math.exp(x)` - Exponential function
- `math.sin(x)` - Sine (in radians)
- `math.cos(x)` - Cosine (in radians)
- `math.tan(x)` - Tangent (in radians)
- `math.round_to_mintick(x)` - Round to configured `syminfo.mintick`, with ties
  toward the higher tick. Missing tick metadata is a configuration error.

### NA Helpers

- `na(value)` - Check if a value is not available (null)
- `nz(value, replacement)` - Return value or replacement if value is NA
- `fixnan(source)` - Carry forward the latest non-`na` call-site value

### Type Helpers

- `int(value)` - Convert to integer
- `float(value)` - Convert to floating-point
- `bool(value)` - Convert to boolean
- `string(value)` - Convert to string

## Array Built-Ins

Arrays in Pinescription are mutable and passed by reference, enabling in-place modifications without assignment.

### Constructors

- `array.new_int(size, initial_value)` - Create integer array
- `array.new_float(size, initial_value)` - Create float array
- `array.new_bool(size, initial_value)` - Create boolean array
- `array.new_string(size, initial_value)` - Create string array
- `array.new_line(size)` - Create line array
- `array.new_label(size)` - Create label array
- `array.new_color`, `array.new_linefill`, `array.new_table` - Additional typed
  constructors; all also support the corresponding `array.new<type>` spelling.
  Arrays of drawing values do not add drawing/rendering behavior.
- `array.new<type>(size, initial_value)` - Generic array constructor
- `array.from(values)` - Create array from a list of values
- `array.copy(array)` - Create a shallow copy of an array

### Access and Mutation

- `array.size(arr)` - Get number of elements
- `array.get(arr, index)` - Get element at index
- `array.set(arr, index, value)` - Set element at index
- `array.push(arr, value)` - Add element to end
- `array.pop(arr)` - Remove and return last element
- `array.unshift(arr, value)` - Add element to beginning
- `array.shift(arr)` - Remove and return first element
- `array.insert(arr, index, value)` - Insert element at index
- `array.clear(arr)` - Remove all elements
- `array.concat(arr1, arr2)` - Concatenate two arrays
- `array.slice(arr, from, to)` - Extract a portion
- `array.first(arr)` - Get first element
- `array.last(arr)` - Get last element
- `array.fill(arr, value[, index_from[, index_to]])` - Fill a half-open range
- `array.reverse(arr)` - Reverse in place
- `array.sort(arr[, order])` - Stable in-place numeric or string sort
- `array.sort_indices(arr[, order])` - New array of indices, leaving input intact

Sort order is `order.ascending` (default) or `order.descending`. `na` elements
sort last. Mixed string/numeric arrays are rejected before mutation.

### Search

- `array.includes(arr, value)` - Check if value exists
- `array.indexof(arr, value)` - Find index of first occurrence
- `array.lastindexof(arr, value)` - Find index of last occurrence
- `array.binary_search_leftmost(arr, value)` - Binary search for leftmost insertion point
- `array.binary_search_rightmost(arr, value)` - Binary search for rightmost insertion point
- `array.binary_search(arr, value)` - Search ascending numeric data; returns -1
  when the value is absent. Trailing `na` elements sort after valid numbers and
  do not hide valid matches.

### Statistics

- `array.abs(arr)` - Absolute value of all elements
- `array.sum(arr)` - Sum of all elements
- `array.avg(arr)` - Average of all elements
- `array.min(arr)` - Minimum value
- `array.max(arr)` - Maximum value
- `array.range(arr)` - Difference between max and min
- `array.median(arr)` - Median value
- `array.mode(arr)` - Most frequent value
- `array.covariance(arr1, arr2)` - Covariance between two arrays
- `array.percentrank(arr, value)` - Percentile rank of a value
- `array.percentile_linear_interpolation(arr, percent)` - Percentile with linear interpolation
- `array.percentile_nearest_rank(arr, percent)` - Percentile using nearest rank method
- `array.variance(arr[, biased])`, `array.stdev(arr[, biased])` - Ignore `na`
  elements; population divisor by default, sample divisor when `biased=false`
- `array.standardize(arr)` - New float array of population-standardized values;
  `na` elements remain `na`. Constant valid values become 1, matching the
  scanned PineTS compatibility behavior.

### Utilities

- `array.join(arr, separator)` - Join array elements into a string
- `array.every(arr, condition)` - Check if all elements satisfy a condition
- `array.some(arr)` - True when any non-`na` element is truthy

## Maps

`map.new`, `clear`, `copy`, `size`, `put`, `get`, `contains`, `remove`, `keys`,
and `values` are available. `map.put_all(destination, source)` mutates the
destination, replacing matching keys; self-merges are safe. Array/map receiver
syntax is supported, for example `a.sort()` and `destination.put_all(source)`.

## Inputs: Pre-Execution Parameters

Supported input kinds are generic `input`, `int`, `float`, `bool`, `string`,
`color`, `time`, `session`, `symbol`, `source`, `price`, `text_area`, `timeframe`,
and `enum`.

Hosts inspect `Engine.InputParameters(bytecode)` without fetching market data
or executing Pine code, then call `SetInputParameters` or configure an
`InputResolver`. Values, type/range/option constraints, and source/enum selections
are resolved once **before provider calls and before runtime creation**.
Runtime calls only read the resulting bindings; source selections follow the
chosen series on each bar. Invalid choices fail before execution.

Each input requires a constant `defval`. IDs use the declaration/assignment
name, or `input#N` for anonymous/repeated names. Pine enum declarations with
optional display titles are supported for enum parameters. Host enum choices
may use a member name, its qualified name, or `EnumValue`. Source selections use
`InputSource("close")` or another supported OHLC/derived source name.
Metadata uses the constants visible at the input's declaration; later constants,
sibling blocks, and function-local constants cannot overwrite those defaults.
Function parameters and nonconstant locals shadow outer constants.
Inputs in function parameter defaults are also discovered and validated before
execution, including defaults in unused functions. Their prepared bindings honor
host overrides just like body-level inputs; for example,
`pick(x = input.int(2, minval=1)) => x` exposes that input before `pick()` runs.
Empty child scopes do not copy the constant environment.

```pine
const int n = 2
length = input.int(n) // Default remains 2, regardless of unused() below.
unused() =>
    const int n = 9
    input.int(n) // This separate input defaults to 9.
```

This deliberately replaces the old permissive behavior where an omitted
`defval` silently produced zero. See [api.md](api.md) for host interfaces.

### Compatibility Aliases

- `array.percentile_neareast_rank` - Alias for `array.percentile_nearest_rank` (typo-preserved)

## Matrix Built-Ins

Matrices provide two-dimensional data structures with comprehensive linear algebra support.

### Creation, Copy, and Access

- `matrix.new_int(rows, columns, initial_value)` - Create integer matrix
- `matrix.new_float(rows, columns, initial_value)` - Create float matrix
- `matrix.new_bool(rows, columns, initial_value)` - Create boolean matrix
- `matrix.new_string(rows, columns, initial_value)` - Create string matrix
- `matrix.new<type>(rows, columns, initial_value)` - Generic matrix constructor
- `matrix.copy(mtx)` - Create a shallow copy
- `matrix.get(mtx, row, column)` - Get element at position
- `matrix.set(mtx, row, column, value)` - Set element at position
- `matrix.row(mtx, row_index)` - Get a row as an array
- `matrix.col(mtx, column_index)` - Get a column as an array

### Shape and Mutation

- `matrix.rows(mtx)` - Number of rows
- `matrix.columns(mtx)` - Number of columns
- `matrix.elements_count(mtx)` - Total number of elements
- `matrix.reshape(mtx, rows, columns)` - Change dimensions
- `matrix.submatrix(mtx, row_from, row_to, col_from, col_to)` - Extract a submatrix
- `matrix.add_row(mtx, row_index, values)` - Insert a row
- `matrix.add_col(mtx, col_index, values)` - Insert a column
- `matrix.remove_row(mtx, row_index)` - Remove a row
- `matrix.remove_col(mtx, col_index)` - Remove a column
- `matrix.swap_rows(mtx, row1, row2)` - Swap two rows
- `matrix.swap_columns(mtx, col1, col2)` - Swap two columns
- `matrix.reverse(mtx)` - Reverse all elements
- `matrix.sort(mtx, column_index, order)` - Sort by column
- `matrix.fill(mtx, value)` - Fill entire matrix with value
- `matrix.fill(mtx, row_from, row_to, col_from, col_to, value)` - Fill a range with value

### Statistics

- `matrix.sum(mtx)` - Sum of all elements
- `matrix.avg(mtx)` - Average of all elements
- `matrix.min(mtx)` - Minimum value
- `matrix.max(mtx)` - Maximum value
- `matrix.median(mtx)` - Median value
- `matrix.mode(mtx)` - Most frequent value

### Operations

- `matrix.concat(mtx1, mtx2)` - Concatenate matrices vertically
- `matrix.diff(mtx, axis)` - Difference between elements
- `matrix.mult(mtx1, mtx2)` - Matrix multiplication
- `matrix.kron(mtx1, mtx2)` - Kronecker product
- `matrix.pow(mtx, exponent)` - Matrix power

### Linear Algebra

- `matrix.det(mtx)` - Determinant (square matrices only)
- `matrix.rank(mtx)` - Matrix rank
- `matrix.trace(mtx)` - Sum of diagonal elements
- `matrix.transpose(mtx)` - Transpose matrix
- `matrix.inv(mtx)` - Matrix inverse (square matrices only)
- `matrix.pinv(mtx)` - Moore-Penrose pseudoinverse
- `matrix.eigenvalues(mtx)` - Eigenvalues (square matrices only)
- `matrix.eigenvectors(mtx)` - Eigenvectors (square matrices only)

### Properties

- `matrix.is_square(mtx)` - Check if matrix is square
- `matrix.is_symmetric(mtx)` - Check if matrix is symmetric
- `matrix.is_diagonal(mtx)` - Check if matrix is diagonal
- `matrix.is_identity(mtx)` - Check if matrix is identity
- `matrix.is_zero(mtx)` - Check if all elements are zero
- `matrix.is_triangular(mtx)` - Check if matrix is triangular
- `matrix.is_binary(mtx)` - Check if matrix contains only 0s and 1s
- `matrix.is_antidiagonal(mtx)` - Check if matrix is antidiagonal
- `matrix.is_antisymmetric(mtx)` - Check if matrix is antisymmetric
- `matrix.is_stochastic(mtx)` - Check if matrix is stochastic

## String and Placeholder APIs

Pinescription provides string manipulation functions and placeholder types for common Pine Script patterns.

### String Helpers

- `str.tostring(value, format)` - Convert values to text; the format is optional.
  Numeric patterns support `0`, `#`, decimal places, comma grouping, percent
  scaling, literal prefixes/suffixes, and a semicolon-separated negative pattern.
  Named formats include `format.percent` (two decimals, no multiplication by
  100), `format.volume` (K/M/B/T suffixes), and `format.mintick` (requires symbol
  tick metadata). The default numeric format uses up to ten decimals.
- `str.length(str)` - Get string length
- `str.upper(str)` - Convert to uppercase
- `str.lower(str)` - Convert to lowercase
- `str.contains(str, substring)` - Check if substring exists
- `str.startswith(str, prefix)` - Check if string starts with prefix
- `str.endswith(str, suffix)` - Check if string ends with suffix
- `str.replace(str, old, new, occurrence)` - Replace one occurrence (default:
  the first, index zero). Occurrences are non-overlapping. An absent occurrence leaves
  the source unchanged.
- `str.substring(str, from, to)` - Extract substring
- `str.split(str, delimiter)` - Split into array
- `str.tonumber(str)` - Parse a signed decimal string; invalid/exponential forms
  return `na`
- `str.trim(str)` - Remove surrounding whitespace
- `str.repeat(source, count[, separator])` - Bounded repetition (0 is empty)
- `str.replace_all(source, target, replacement)` - Literal replacement of all
  non-overlapping occurrences
- `str.pos(source, target)` - Character position of the first match, or `na`
- `str.match(source, pattern)` - First regex match, or an empty string. Patterns
  use Go's RE2-compatible syntax: Java lookarounds/backreferences are **not**
  emulated and unsupported expressions produce an error.
- `str.format_time(timestamp[, format[, timezone]])` - Calendar patterns,
  default `yyyy-MM-dd'T'HH:mm:ssZ`, exchange timezone unless overridden.
  The active `Z` emits the actual offset, e.g. `2025-01-01T20:00:00-0500`
  for `2025-01-02T01:00:00Z` in New York; an explicitly quoted `'Z'` stays literal.
  Supported tokens include `y`, `M`, `d`, `E`, `D`, `w`, `H`, `h`, `m`, `s`,
  `S`, `a`, `Z`, `X`, and `z`, with quoted literals.
- `str.format(format, args...)` - Indexed placeholders, including
  `{0,number,integer}`, `{0,number,percent}`, `{0,number,currency}`, custom number
  patterns, and `{0,date,yyyy-MM-dd}` / `{0,time,HH:mm:ss}`. Single quotes escape
  literal text; doubled quotes produce a quote. The existing `{{` / `}}` brace
  extension and preservation of out-of-range placeholders remain supported.
  Message number formatting rounds ties to even; `str.tostring` rounds decimal
  ties away from zero. Patterns are limited to 4096 bytes and results to 1 MiB.

### Completed Series and Time Helpers

- `fixnan(source)` carries the most recent non-`na` value from an earlier
  evaluation of the same call site. It returns `na` until such a value exists.
  Separate calls, including separate calls through a script function, have
  independent state. Re-evaluating a bar does not commit a tentative value.
- Bare `ta.tr` is true range with an `na` result when the preceding close is
  unavailable. `ta.tr(true)` uses `high - low` in that case; `ta.tr(false)` does
  not. The legacy no-argument `tr()` / `ta.tr()` extension retains its fallback.
- `time()` and `time_close()` accept timeframe, session, timezone, `bars_back`,
  and `timeframe_bars_back`. The two-argument `(timeframe, bars_back)` form is
  also accepted. Session strings support comma-separated `HHmm-HHmm` periods,
  optional Sunday=1 day digits, and overnight sessions. Out-of-session and
  pre-history bars return `na`; session-end closes are clipped to that session.
  Overnight daily buckets use the containing session's opening date.
  Calendar `timeframe_bars_back` offsets follow actual adjacent buckets, including
  shortened year-end buckets: on December 31, 2025,
  `time_close("5M", "", "UTC", 0, 1)` is November 1, not August 1.
  Large calendar offsets skip complete 400-year Gregorian cycles rather than
  visiting each intervening bar. Intraday offsets likewise jump ordinary days
  between timezone transitions, while preserving shortened final buckets: at
  23:59 UTC, `time("7", "", "UTC", 0, 1)` opens at 23:48, not 23:50.
- Timezones accept IANA names and fixed `UTC`/`GMT` offsets. Invalid zones are
  errors, not silent UTC fallbacks. `timestamp` accepts numeric calendar
  components (optionally prefixed by timezone) or a date string in ISO/RFC3339
  and common English day-month-year forms.
- `timeframe.change` compares current and previous time buckets, rather than
  counting chart bars. The first available bar returns false.
- Seconds and tick resolutions are classified as intraday; tick resolutions
  have no predictable time boundary. `timeframe.from_seconds` rounds up to a
  representable resolution, capped at `12M`. Month conversion in
  `timeframe.in_seconds` uses 2,628,003 seconds.

The base Provider interface still supplies value series. Without a
`BarTimeProvider` / `SetBarTimes`, bar times use the engine's start time and
timeframe. Without symbol timezone metadata, time calculations use UTC.
Timestamp arrays must match the base-series length and have strictly increasing
opens and closes after opens. Helpers do not infer holidays, missing bars, or
exchange schedules. Future projected times use the configured chart step.
Monthly projections clamp invalid days and preserve month-end anchors; inferred
bars remain anchored to the configured final time (March 31 follows February's
last day, not a normalized March 3). Non-month-end inferred dates retain their
original day whenever that month permits it.
Session first/last flags join at most one occurrence of each supplied
pre/regular/post schedule. A regular-only `24x7` session retains daily boundaries.

### Runtime Errors and Ticker IDs

`runtime.error(message)` aborts execution with the bar index, active symbol,
and message.

`ticker.new`, `ticker.modify`, `ticker.standard`, `ticker.inherit`, and
`ticker.heikinashi` construct, modify, and strip ticker identifiers. Session,
adjustment, backadjustment, and settlement modifiers use deterministic encoded
JSON IDs. Heikin Ashi uses PineTS-compatible `;heikinashi` routing; it does not
transform bars. Providers must supply the data for modified ticker IDs.
Alternative-chart constructors `ticker.renko`, `kagi`, `linebreak`, and
`pointfigure` preserve their validated parameters in engine-local opaque IDs.
`ParseTickerID` exposes those descriptors to providers. These IDs are not a
promise of TradingView server wire-format compatibility: the provider must
serve the requested transformed bars, never ordinary candles under another name.

### Libraries

`library()`, versioned `import author/name/version as alias`, and exported
functions, methods, types, and enums are supported. Configure a `LibraryResolver`
before compilation; linked bytecode requires no resolver at execution time.
Imports are namespaced, private helpers and dependencies stay out of the public
namespace, and scalar constant library globals are isolated.
`import` and `export` are top-level-only; nested declarations fail at compilation.
Receiver-qualified library methods take precedence over unqualified functions
with the same name.

Libraries with executable/data-dependent globals are rejected. Cycles, alias
collisions, excessive nesting/module count, and oversized sources are errors.
No filesystem or network loading is implicit.

### Historical Requests

`request.security` evaluates an expression against an isolated requested
symbol/timeframe. `SecurityDataProvider` supplies value series and actual bar
timestamps. Chart providers are not switched back and forth between timeframes.
Same-context requests can use the chart data without an additional provider.

- Default `barmerge.lookahead_off` exposes only confirmed requested bars.
  `lookahead_on` deliberately exposes the selected historical period early.
- `barmerge.gaps_off` carries the last available value; `gaps_on` emits on newly
  aligned requested boundaries.
- Lower-timeframe `request.security` selects the first intrabar with
  lookahead on, the last with it off.
- `request.security_lower_tf` returns closed intrabars in chronological arrays;
  tuple expressions return a tuple of arrays. An equal timeframe returns the
  current bar as one intrabar.
- `ignore_invalid_symbol`, `ignore_invalid_timeframe` (lower-TF), and
  `calc_bars_count` are supported. Ignored invalid symbols return `na`.
- Global expression dependencies, functions, parameters, and supported global
  collection mutations are evaluated in the requested context. Unrelated
  plots/alerts/callbacks are not replayed. Mutable results are snapshotted.
  Mutations are retained when their return value is assigned but unused, and
  through reference aliases such as `b = a; b.push(5)`.
  Script calls that mutate a needed collection parameter are retained too,
  including nested calls and mutations inside switch expressions. Scalar copies
  are not collection aliases and do not create extra request dependencies.
  Series-parameter substitution includes switch selectors, matches, and bodies.
- Limits: 40 data contexts, 128 expression streams, nested depth 16, bounded
  collection snapshots. Arbitrary mutable function-local captures and
  cross-currency conversion are not implemented; unsupported use is an error.
  Currency is validated on every evaluation, including cached stream reuse.

```pine
a = array.from(close, 5)
b = a
removed = array.pop(b)
requested = request.security("OTHER", "120", array.sum(a))
// requested uses the requested close alone, not requested close + 5.
```

```pine
appendFive(values) => array.push(values, 5)
values = array.from(close)
appendFive(values)
requested = request.security("OTHER", "120", array.sum(values))
// The script call is retained: requested equals requested close + 5.
```

### Footprints

`request.footprint(ticks_per_row[, va_percent[, imbalance_percent]])` uses
genuine executed buy/sell levels supplied by `FootprintProvider`. No provider
or no data returns `na`; usable data requires explicit `syminfo.mintick`.

Rows share a tick grid anchored at zero and cover provided levels plus the
candle range. POC ties favor the center, then the lower row. The value area
grows from POC toward the larger adjacent row, stopping before exceeding the
requested share (with a 0.01 volume tolerance). Imbalances are diagonal between
neighboring rows. More than 2,000 rows returns `na`; invalid source levels error.

`footprint.buy_volume`, `sell_volume`, `total_volume`, `delta`, `poc`, `vah`,
`val`, `rows`, and `get_row_by_price` are available. `volume_row` exposes
`up_price`, `down_price`, `buy_volume`, `sell_volume`, `total_volume`, `delta`,
`has_buy_imbalance`, and `has_sell_imbalance`. Namespace and receiver syntax work.
Rows are immutable snapshots; `rows()` returns a fresh array. Inside a security
expression, footprints describe the requested context's own bars.

### Native Historical Strategies — Limited Broker Model

`strategy()` enables a native historical broker. It requires provider OHLC,
`syminfo.mintick`, `syminfo.pointvalue`, and explicitly disabled margins.
Exact OHLC/volume lookups never substitute the default close series on a failed
or empty load. Strategies reject invalid OHLC; other exact-data consumers receive
`na` and retain their documented missing-data policies.
Preloading caches only genuinely loaded series under their own value-type keys;
legacy default-series fallback is not registered as an exact OHLC cache entry.

For example:

```pine
strategy("Example", initial_capital = 10000, margin_long = 0, margin_short = 0)
if bar_index == 0
    strategy.entry("L", strategy.long, qty = 1)
if bar_index == 5
    strategy.close("L")
net = strategy.netprofit
```

Implemented behavior:

- Market orders fill on the next bar's open by default. Pending limit, stop,
  and stop-limit orders use a deterministic OHLC path; gaps fill at the opening
  price when marketable. Stop-limit activation is distinct from its limit fill.
- `entry` supports pyramiding and reversal; `order` nets its quantity against
  existing positions. `exit` supports absolute or tick-relative take-profit /
  stop-loss brackets with first-hit OCO behavior. `close`, `close_all`,
  `cancel`, and `cancel_all` are supported.
  Named exits capture eligible open trades and pending entry identities when
  called; an unmatched named exit cannot capture an entry created on a later bar.
  Named exits with no eligible entries are discarded, and orphaned exits are
  removed when cancellation leaves neither eligible pending nor open trades.
- `close(..., immediately=true)` closes during evaluation;
  `process_orders_on_close` processes eligible orders after evaluation, without
  a second script calculation.
  Immediate market closes still apply direction-dependent slippage.
  Slippage changes execution prices, not the chart mark used for equity or later
  orders. Two immediate partial closes on a bar with close 110 and one tick of
  sell slippage both fill at 109; the remaining position stays marked at 110.
- Fixed/cash/equity-percent sizing, commission modes, slippage, and partial
  closes are supported. Cash/percent sizing requires `syminfo.mincontract`;
  default order quantity is calculated at the fill price.
- Explicit-quantity OCA cancellation/reduction groups, `risk.allow_entry_in`,
  and `risk.max_position_size` are supported.
  OCA groups are identified by both name and type, and affect siblings only
  after a nonzero executed fill.
- Position/account metrics, open/closed trade counts, profits/fees, and
  per-trade getters are available. Conversion helpers support same-currency
  values; cross-currency broker accounting is not implemented.
  Historical scalar metrics (e.g. `strategy.position_size[1]`) use per-bar
  calculation snapshots; unavailable history is `na`. Current-bar reads remain
  live. Snapshots precede optional post-evaluation close fills so they agree
  with script variables recorded during that calculation.

```pine
strategy("Entry transition", margin_long=0, margin_short=0)
if bar_index == 0
    strategy.entry("L", strategy.long)
opened = strategy.position_size > 0 and strategy.position_size[1] == 0
// opened is true on bar 1, when the next-open market entry fills.
```

**Not full TradingView broker parity:** unsupported options error rather than
silently approximating advanced broker behavior. See
[Strategy APIs](#strategy-apis) for the remaining broker/risk limitations and
validation scope.

### Built-in Type Placeholders

- `color` - Color type placeholder
- `line.new` - Line object constructor placeholder
- `label.new` - Label object constructor placeholder

### Drawing and UI Compatibility

Drawing and UI functions are available for script compatibility. Built-in fallbacks do not render by themselves, but host applications can register exact-name hooks to capture drawing objects and render them elsewhere. This is how applications can support TradingView-style drawing scripts while keeping rendering outside the core runtime.

- `line.*` - Line drawing functions (stubs)
- `label.*` - Label drawing functions and deletion hooks
- `box.*` - Box drawing functions and deletion hooks
- `polyline.*` - Polyline drawing functions and deletion hooks
- `chart.point.from_index` - Chart-point construction for drawing APIs
- `table.*` - Table creation, cell, clear, and merge hooks
- `linefill.*` - Line fill functions (stubs)
- `barcolor` - Bar color function (stub)

Hookable drawing calls can be overridden with `RegisterFunctionWithParamNames`, even when a compatibility stub exists. This is intentionally narrower than general built-in overriding: ordinary implemented built-ins such as `ta.rsi` remain protected, while supported drawing hook points such as `box.new`, `polyline.new`, `table.cell`, and `table.clear` may be registered by the host.

## Unsupported Features

The following areas remain incomplete or unsupported. Exact-name strategy and
request hooks can override native behavior or provide unsupported functions.

### Strategy APIs

The native broker supports the [historical subset above](#native-historical-strategies--limited-broker-model).
Strategy operations require a `strategy()` declaration unless overridden by a
registered host hook. Unsupported options return explicit errors:

- **Broker execution:** margin liquidation (native mode requires explicit zero
  margins), trailing stops, fill/tick recalculation, bar magnifier, and limit-fill
  verification are not implemented. Historical execution has no live-tick
  rollback.
- **Exit/accounting behavior:** cross-currency accounting, combined relative/
  absolute exit-leg precedence, FIFO reassignment across distinct entry IDs,
  full exit reservation semantics, and order-fill alerts are not implemented.
- **Risk rules:** `strategy.risk.max_cons_loss_days`,
  `strategy.risk.max_drawdown`, `strategy.risk.max_intraday_filled_orders`, and
  `strategy.risk.max_intraday_loss` are unsupported. `allow_entry_in` and
  `max_position_size` are implemented.

The broker, ledger, and excursion statistics are tested against deterministic,
hand-calculated fixtures, not certified against every TradingView strategy
field, reporting convention, or fill scenario.

### Request APIs

`request.security`, `request.security_lower_tf`, and `request.footprint` have
native implementations with the limits above. Financial, economic,
corporate-action, Quandl, currency-rate, and seed data functions remain
unsupported without host hooks. Data-less placeholders are not treated as data.
Cross-currency conversion and arbitrary mutable function-local captures are
also unsupported. The [historical request model](#historical-requests) documents
dependency replay, alignment, and resource limits; complete TradingView
compatibility for every expression/data-gap case has not been independently
verified. Alternative-chart ticker IDs require genuine provider-transformed
bars, not ordinary candles returned under another name.

Exact-name custom function hooks can provide selected request behavior. Use `RegisterFunctionWithParamNames` when the Pine call may use named arguments, such as `request.security(symbol = ..., timeframe = ..., expression = ...)`.

### Plot APIs

All unregistered plotting functions return an unsupported feature error:

- `plot`, `plotshape`, `plotchar`, `plotbar`, `plotcandle`
- `plotarrow`, `fill`
- And all other visualization functions

Exact-name custom function hooks can also provide selected plotting behavior. Use `RegisterFunction` for simple positional hooks such as `plot(close)`, or `RegisterFunctionWithParamNames` when Pine scripts may use named arguments, such as `plot(close, title = "Close")`.
