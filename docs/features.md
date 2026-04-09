<!--
SPDX-FileCopyrightText: 2026 Woodstock K.K.

SPDX-License-Identifier: AGPL-3.0-only
-->

# Supported Features

Complete reference of Pine Script v6 features supported by Pinescription.

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

### NA Helpers

- `na(value)` - Check if a value is not available (null)
- `nz(value, replacement)` - Return value or replacement if value is NA

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

### Search

- `array.includes(arr, value)` - Check if value exists
- `array.indexof(arr, value)` - Find index of first occurrence
- `array.lastindexof(arr, value)` - Find index of last occurrence
- `array.binary_search_leftmost(arr, value)` - Binary search for leftmost insertion point
- `array.binary_search_rightmost(arr, value)` - Binary search for rightmost insertion point

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

### Utilities

- `array.join(arr, separator)` - Join array elements into a string
- `array.every(arr, condition)` - Check if all elements satisfy a condition

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

- `str.tostring(value)` - Convert value to string
- `str.length(str)` - Get string length
- `str.upper(str)` - Convert to uppercase
- `str.lower(str)` - Convert to lowercase
- `str.contains(str, substring)` - Check if substring exists
- `str.startswith(str, prefix)` - Check if string starts with prefix
- `str.endswith(str, suffix)` - Check if string ends with suffix
- `str.replace(str, old, new)` - Replace substring
- `str.substring(str, from, to)` - Extract substring
- `str.split(str, delimiter)` - Split into array
- `str.format(format, args)` - Format string with arguments

### Built-in Type Placeholders

- `color` - Color type placeholder
- `line.new` - Line object constructor placeholder
- `label.new` - Label object constructor placeholder

### No-Render UI Stubs

These functions are available for script compatibility but perform no actual rendering. They allow scripts to execute without producing visual output.

- `line.*` - Line drawing functions (stubs)
- `label.*` - Label drawing functions (stubs)
- `box.*` - Box drawing functions (stubs)
- `table.*` - Table drawing functions (stubs)
- `linefill.*` - Line fill functions (stubs)
- `barcolor` - Bar color function (stub)

## Unsupported Features

The following Pine Script features are not implemented in Pinescription. Attempting to use them will produce a runtime error: `unsupported feature: ...`

### Strategy APIs

All `strategy.*` functions return an unsupported feature error:

- `strategy.entry`, `strategy.exit`, `strategy.order`
- `strategy.position`, `strategy.closedtrades`, `strategy.opentrades`
- And all other strategy-related functions

### Plot APIs

All plotting functions return an unsupported feature error:

- `plot`, `plotshape`, `plotchar`, `plotbar`, `plotcandle`
- `plotarrow`, `fill` 
- And all other visualization functions
