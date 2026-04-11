// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Matrix represents a two-dimensional matrix of float64 values backed by a
// slice of row slices. Matrices are used internally by Pine Script's
// matrix operations (matrix.new, matrix.* functions) and are also returned
// by matrix-valued built-in functions.
type Matrix struct {
	Data [][]float64
}

// newMatrix creates a rows-by-cols matrix filled with the given value.
func newMatrix(rows, cols int, fill float64) (*Matrix, error) {
	if rows < 0 || cols < 0 {
		return nil, errors.New("matrix dimensions must be non-negative")
	}
	data := make([][]float64, rows)
	for i := range data {
		data[i] = make([]float64, cols)
		for j := range data[i] {
			data[i][j] = fill
		}
	}
	return &Matrix{Data: data}, nil
}

func (m *Matrix) copy() *Matrix {
	if m == nil {
		return &Matrix{}
	}
	data := make([][]float64, len(m.Data))
	for i := range m.Data {
		data[i] = append([]float64{}, m.Data[i]...)
	}
	return &Matrix{Data: data}
}

func (m *Matrix) rows() int {
	if m == nil {
		return 0
	}
	return len(m.Data)
}

func (m *Matrix) cols() int {
	if m == nil || len(m.Data) == 0 {
		return 0
	}
	return len(m.Data[0])
}

func (m *Matrix) elementsCount() int {
	return m.rows() * m.cols()
}

func (m *Matrix) validateRect() error {
	if m == nil {
		return errors.New("matrix is nil")
	}
	if len(m.Data) == 0 {
		return nil
	}
	c := len(m.Data[0])
	for i := 1; i < len(m.Data); i++ {
		if len(m.Data[i]) != c {
			return errors.New("matrix is not rectangular")
		}
	}
	return nil
}

func (m *Matrix) get(r, c int) (float64, error) {
	if err := m.validateRect(); err != nil {
		return 0, err
	}
	if r < 0 || c < 0 || r >= m.rows() || c >= m.cols() {
		return 0, fmt.Errorf("matrix index out of range (%d,%d)", r, c)
	}
	return m.Data[r][c], nil
}

func (m *Matrix) set(r, c int, v float64) error {
	if err := m.validateRect(); err != nil {
		return err
	}
	if r < 0 || c < 0 || r >= m.rows() || c >= m.cols() {
		return fmt.Errorf("matrix index out of range (%d,%d)", r, c)
	}
	m.Data[r][c] = v
	return nil
}

func (m *Matrix) row(i int) ([]interface{}, error) {
	if err := m.validateRect(); err != nil {
		return nil, err
	}
	if i < 0 || i >= m.rows() {
		return nil, fmt.Errorf("row index out of range: %d", i)
	}
	out := make([]interface{}, m.cols())
	for c := 0; c < m.cols(); c++ {
		out[c] = m.Data[i][c]
	}
	return out, nil
}

func (m *Matrix) col(i int) ([]interface{}, error) {
	if err := m.validateRect(); err != nil {
		return nil, err
	}
	if i < 0 || i >= m.cols() {
		return nil, fmt.Errorf("column index out of range: %d", i)
	}
	out := make([]interface{}, m.rows())
	for r := 0; r < m.rows(); r++ {
		out[r] = m.Data[r][i]
	}
	return out, nil
}

func (m *Matrix) flatten() []float64 {
	out := make([]float64, 0, m.elementsCount())
	for r := 0; r < m.rows(); r++ {
		out = append(out, m.Data[r]...)
	}
	return out
}

func (m *Matrix) fill(v float64) {
	for r := 0; r < m.rows(); r++ {
		for c := 0; c < m.cols(); c++ {
			m.Data[r][c] = v
		}
	}
}

func (m *Matrix) fillRange(v float64, rowFrom, rowTo, colFrom, colTo int) error {
	if err := m.validateRect(); err != nil {
		return err
	}
	if rowFrom < 0 || colFrom < 0 || rowTo > m.rows() || colTo > m.cols() || rowFrom > rowTo || colFrom > colTo {
		return errors.New("invalid fill range")
	}
	for r := rowFrom; r < rowTo; r++ {
		for c := colFrom; c < colTo; c++ {
			m.Data[r][c] = v
		}
	}
	return nil
}

func (m *Matrix) reshape(rows, cols int) error {
	if rows < 0 || cols < 0 {
		return errors.New("matrix dimensions must be non-negative")
	}
	flat := m.flatten()
	if rows*cols != len(flat) {
		return errors.New("reshape dimensions mismatch")
	}
	idx := 0
	data := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		data[r] = make([]float64, cols)
		for c := 0; c < cols; c++ {
			data[r][c] = flat[idx]
			idx++
		}
	}
	m.Data = data
	return nil
}

func (m *Matrix) addRow(index int, row []float64) error {
	if err := m.validateRect(); err != nil {
		return err
	}
	if index < 0 || index > m.rows() {
		return errors.New("row insert index out of range")
	}
	if m.rows() > 0 && len(row) != m.cols() {
		return errors.New("new row has invalid width")
	}
	if m.rows() == 0 {
		m.Data = append(m.Data, append([]float64{}, row...))
		return nil
	}
	m.Data = append(m.Data, nil)
	copy(m.Data[index+1:], m.Data[index:])
	m.Data[index] = append([]float64{}, row...)
	return nil
}

func (m *Matrix) addCol(index int, col []float64) error {
	if err := m.validateRect(); err != nil {
		return err
	}
	if m.rows() == 0 {
		for i := 0; i < len(col); i++ {
			m.Data = append(m.Data, []float64{col[i]})
		}
		return nil
	}
	if index < 0 || index > m.cols() {
		return errors.New("column insert index out of range")
	}
	if len(col) != m.rows() {
		return errors.New("new column has invalid height")
	}
	for r := 0; r < m.rows(); r++ {
		row := append([]float64{}, m.Data[r]...)
		row = append(row, 0)
		copy(row[index+1:], row[index:])
		row[index] = col[r]
		m.Data[r] = row
	}
	return nil
}

func (m *Matrix) removeRow(index int) error {
	if index < 0 || index >= m.rows() {
		return errors.New("row index out of range")
	}
	m.Data = append(m.Data[:index], m.Data[index+1:]...)
	return nil
}

func (m *Matrix) removeCol(index int) error {
	if index < 0 || index >= m.cols() {
		return errors.New("column index out of range")
	}
	for r := 0; r < m.rows(); r++ {
		row := m.Data[r]
		m.Data[r] = append(row[:index], row[index+1:]...)
	}
	return nil
}

func (m *Matrix) swapRows(i, j int) error {
	if i < 0 || j < 0 || i >= m.rows() || j >= m.rows() {
		return errors.New("row index out of range")
	}
	m.Data[i], m.Data[j] = m.Data[j], m.Data[i]
	return nil
}

func (m *Matrix) swapCols(i, j int) error {
	if i < 0 || j < 0 || i >= m.cols() || j >= m.cols() {
		return errors.New("column index out of range")
	}
	for r := 0; r < m.rows(); r++ {
		m.Data[r][i], m.Data[r][j] = m.Data[r][j], m.Data[r][i]
	}
	return nil
}

func (m *Matrix) reverse() {
	flat := m.flatten()
	for i, j := 0, len(flat)-1; i < j; i, j = i+1, j-1 {
		flat[i], flat[j] = flat[j], flat[i]
	}
	idx := 0
	for r := 0; r < m.rows(); r++ {
		for c := 0; c < m.cols(); c++ {
			m.Data[r][c] = flat[idx]
			idx++
		}
	}
}

func (m *Matrix) sort() {
	flat := m.flatten()
	sort.Float64s(flat)
	idx := 0
	for r := 0; r < m.rows(); r++ {
		for c := 0; c < m.cols(); c++ {
			m.Data[r][c] = flat[idx]
			idx++
		}
	}
}

func (m *Matrix) submatrix(rowFrom, rowTo, colFrom, colTo int) (*Matrix, error) {
	if rowFrom < 0 || colFrom < 0 || rowTo > m.rows() || colTo > m.cols() || rowFrom > rowTo || colFrom > colTo {
		return nil, errors.New("invalid submatrix bounds")
	}
	data := make([][]float64, 0, rowTo-rowFrom)
	for r := rowFrom; r < rowTo; r++ {
		data = append(data, append([]float64{}, m.Data[r][colFrom:colTo]...))
	}
	return &Matrix{Data: data}, nil
}

// matrixConcat concatenates two matrices either horizontally (same row count)
// or vertically (same column count). Returns an error if neither dimension matches.
func matrixConcat(a, b *Matrix) (*Matrix, error) {
	if a.rows() == b.rows() {
		out := a.copy()
		for r := 0; r < out.rows(); r++ {
			out.Data[r] = append(out.Data[r], b.Data[r]...)
		}
		return out, nil
	}
	if a.cols() == b.cols() {
		out := a.copy()
		for r := 0; r < b.rows(); r++ {
			out.Data = append(out.Data, append([]float64{}, b.Data[r]...))
		}
		return out, nil
	}
	return nil, errors.New("concat requires equal rows (horizontal) or equal columns (vertical)")
}

// matrixDiff returns the element-wise difference a - b. Both matrices must have
// the same dimensions. Returns an error if dimensions do not match.
func matrixDiff(a, b *Matrix) (*Matrix, error) {
	if a.rows() != b.rows() || a.cols() != b.cols() {
		return nil, errors.New("matrix dimensions mismatch")
	}
	out := a.copy()
	for r := 0; r < out.rows(); r++ {
		for c := 0; c < out.cols(); c++ {
			out.Data[r][c] -= b.Data[r][c]
		}
	}
	return out, nil
}

// matrixMult computes the standard matrix product of a and b, returning the
// resulting matrix. The number of columns in a must equal the number of rows in b.
// Returns an error if the dimensions are incompatible.
func matrixMult(a, b *Matrix) (*Matrix, error) {
	if a.cols() != b.rows() {
		return nil, errors.New("matrix multiplication dimension mismatch")
	}
	bt := matrixTranspose(b)
	out, _ := newMatrix(a.rows(), b.cols(), 0)
	for i := 0; i < a.rows(); i++ {
		ai := a.Data[i]
		oi := out.Data[i]
		for j := 0; j < bt.rows(); j++ {
			bj := bt.Data[j]
			sum := 0.0
			for k, av := range ai {
				sum += av * bj[k]
			}
			oi[j] = sum
		}
	}
	return out, nil
}

// matrixKron computes the Kronecker product of a and b, producing a block matrix
// where each element of a is multiplied by the entire matrix b.
func matrixKron(a, b *Matrix) *Matrix {
	out, _ := newMatrix(a.rows()*b.rows(), a.cols()*b.cols(), 0)
	for i := 0; i < a.rows(); i++ {
		for j := 0; j < a.cols(); j++ {
			for bi := 0; bi < b.rows(); bi++ {
				for bj := 0; bj < b.cols(); bj++ {
					out.Data[i*b.rows()+bi][j*b.cols()+bj] = a.Data[i][j] * b.Data[bi][bj]
				}
			}
		}
	}
	return out
}

// matrixPow computes the matrix power a^p using binary exponentiation.
// a must be square. p must be non-negative. Returns an error if a is not square
// or if p is negative.
func matrixPow(a *Matrix, p int) (*Matrix, error) {
	if a.rows() != a.cols() {
		return nil, errors.New("matrix.pow requires square matrix")
	}
	if p < 0 {
		return nil, errors.New("matrix.pow requires non-negative power")
	}
	identity, _ := newMatrix(a.rows(), a.cols(), 0)
	for i := 0; i < identity.rows(); i++ {
		identity.Data[i][i] = 1
	}
	if p == 0 {
		return identity, nil
	}
	result := identity
	base := a.copy()
	for p > 0 {
		if p%2 == 1 {
			tmp, err := matrixMult(result, base)
			if err != nil {
				return nil, err
			}
			result = tmp
		}
		p /= 2
		if p > 0 {
			tmp, err := matrixMult(base, base)
			if err != nil {
				return nil, err
			}
			base = tmp
		}
	}
	return result, nil
}

// matrixTranspose returns the transpose of matrix a, where rows become columns.
func matrixTranspose(a *Matrix) *Matrix {
	out, _ := newMatrix(a.cols(), a.rows(), 0)
	for r := 0; r < a.rows(); r++ {
		for c := 0; c < a.cols(); c++ {
			out.Data[c][r] = a.Data[r][c]
		}
	}
	return out
}

// matrixDet computes the determinant of a square matrix using Gaussian elimination.
// Returns an error if the matrix is not square.
func matrixDet(a *Matrix) (float64, error) {
	if a.rows() != a.cols() {
		return 0, errors.New("determinant requires square matrix")
	}
	n := a.rows()
	m := a.copy().Data
	det := 1.0
	sign := 1.0
	for i := 0; i < n; i++ {
		pivot := i
		for r := i + 1; r < n; r++ {
			if math.Abs(m[r][i]) > math.Abs(m[pivot][i]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][i]) < 1e-12 {
			return 0, nil
		}
		if pivot != i {
			m[pivot], m[i] = m[i], m[pivot]
			sign *= -1
		}
		pivotVal := m[i][i]
		det *= pivotVal
		for r := i + 1; r < n; r++ {
			factor := m[r][i] / pivotVal
			for c := i; c < n; c++ {
				m[r][c] -= factor * m[i][c]
			}
		}
	}
	return det * sign, nil
}

// matrixRank computes the rank (number of linearly independent rows/columns) of a
// matrix using Gaussian elimination with partial pivoting.
func matrixRank(a *Matrix) int {
	m := a.copy().Data
	rows, cols := a.rows(), a.cols()
	rank := 0
	r := 0
	for c := 0; c < cols && r < rows; c++ {
		pivot := r
		for i := r + 1; i < rows; i++ {
			if math.Abs(m[i][c]) > math.Abs(m[pivot][c]) {
				pivot = i
			}
		}
		if math.Abs(m[pivot][c]) < 1e-10 {
			continue
		}
		m[pivot], m[r] = m[r], m[pivot]
		pv := m[r][c]
		for j := c; j < cols; j++ {
			m[r][j] /= pv
		}
		for i := 0; i < rows; i++ {
			if i == r {
				continue
			}
			f := m[i][c]
			for j := c; j < cols; j++ {
				m[i][j] -= f * m[r][j]
			}
		}
		r++
		rank++
	}
	return rank
}

// matrixInv computes the inverse of a square matrix using Gauss-Jordan elimination.
// Returns an error if the matrix is not square or is singular.
func matrixInv(a *Matrix) (*Matrix, error) {
	if a.rows() != a.cols() {
		return nil, errors.New("inverse requires square matrix")
	}
	n := a.rows()
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		for j := 0; j < n; j++ {
			aug[i][j] = a.Data[i][j]
		}
		aug[i][n+i] = 1
	}
	for i := 0; i < n; i++ {
		pivot := i
		for r := i + 1; r < n; r++ {
			if math.Abs(aug[r][i]) > math.Abs(aug[pivot][i]) {
				pivot = r
			}
		}
		if math.Abs(aug[pivot][i]) < 1e-12 {
			return nil, errors.New("matrix is singular")
		}
		aug[pivot], aug[i] = aug[i], aug[pivot]
		pv := aug[i][i]
		for c := 0; c < 2*n; c++ {
			aug[i][c] /= pv
		}
		for r := 0; r < n; r++ {
			if r == i {
				continue
			}
			f := aug[r][i]
			for c := 0; c < 2*n; c++ {
				aug[r][c] -= f * aug[i][c]
			}
		}
	}
	out, _ := newMatrix(n, n, 0)
	for r := 0; r < n; r++ {
		copy(out.Data[r], aug[r][n:])
	}
	return out, nil
}

// matrixPinv computes the Moore-Penrose pseudoinverse of matrix a. For tall matrices
// (rows >= cols) it computes (A^T A)^-1 A^T; for wide matrices it computes
// A^T (A A^T)^-1. Returns an error if the intermediate matrix is singular.
func matrixPinv(a *Matrix) (*Matrix, error) {
	at := matrixTranspose(a)
	if a.rows() >= a.cols() {
		ata, err := matrixMult(at, a)
		if err != nil {
			return nil, err
		}
		inv, err := matrixInv(ata)
		if err != nil {
			return nil, err
		}
		return matrixMult(inv, at)
	}
	aat, err := matrixMult(a, at)
	if err != nil {
		return nil, err
	}
	inv, err := matrixInv(aat)
	if err != nil {
		return nil, err
	}
	return matrixMult(at, inv)
}

// matrixTrace returns the trace (sum of diagonal elements) of a square matrix.
// Returns an error if the matrix is not square.
func matrixTrace(a *Matrix) (float64, error) {
	if a.rows() != a.cols() {
		return 0, errors.New("trace requires square matrix")
	}
	t := 0.0
	for i := 0; i < a.rows(); i++ {
		t += a.Data[i][i]
	}
	return t, nil
}

// matrixEigen computes the eigenvalues and eigenvectors of a square matrix using
// the QR iteration algorithm. eigenvalues is returned as a slice of floats sorted
// in descending order. eigenvectors is a square matrix where each column is the
// corresponding eigenvector. Returns an error if the matrix is not square.
func matrixEigen(a *Matrix) ([]float64, *Matrix, error) {
	if a.rows() != a.cols() {
		return nil, nil, errors.New("eigenvalues/eigenvectors require square matrix")
	}
	n := a.rows()
	if n == 0 {
		return []float64{}, &Matrix{Data: [][]float64{}}, nil
	}
	if n == 1 {
		vec, _ := newMatrix(1, 1, 1)
		return []float64{a.Data[0][0]}, vec, nil
	}

	ak := a.copy()
	v := matrixIdentity(n)

	const maxIter = 128
	const tol = 1e-10
	for iter := 0; iter < maxIter; iter++ {
		q, r, err := matrixQRDecompose(ak)
		if err != nil {
			return nil, nil, err
		}
		next, err := matrixMult(r, q)
		if err != nil {
			return nil, nil, err
		}
		vq, err := matrixMult(v, q)
		if err != nil {
			return nil, nil, err
		}
		ak = next
		v = vq

		off := 0.0
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i != j {
					off += math.Abs(ak.Data[i][j])
				}
			}
		}
		if off < tol {
			break
		}
	}

	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = ak.Data[i][i]
	}

	type pair struct {
		idx int
		val float64
	}
	pairs := make([]pair, n)
	for i := 0; i < n; i++ {
		pairs[i] = pair{idx: i, val: vals[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].val > pairs[j].val })

	sortedVals := make([]float64, n)
	sortedVecs, _ := newMatrix(n, n, 0)
	for col := 0; col < n; col++ {
		sortedVals[col] = pairs[col].val
		src := pairs[col].idx
		norm := 0.0
		for r := 0; r < n; r++ {
			norm += v.Data[r][src] * v.Data[r][src]
		}
		norm = math.Sqrt(norm)
		if norm < 1e-12 {
			norm = 1
		}
		for r := 0; r < n; r++ {
			sortedVecs.Data[r][col] = v.Data[r][src] / norm
		}
	}

	return sortedVals, sortedVecs, nil
}

// matrixIdentity returns an n-by-n identity matrix with ones on the main diagonal.
func matrixIdentity(n int) *Matrix {
	m, _ := newMatrix(n, n, 0)
	for i := 0; i < n; i++ {
		m.Data[i][i] = 1
	}
	return m
}

// matrixQRDecompose computes the QR decomposition of a square matrix a, returning
// orthogonal matrix Q and upper-triangular matrix R such that a = QR.
// Returns an error if the matrix is not square.
func matrixQRDecompose(a *Matrix) (*Matrix, *Matrix, error) {
	if a.rows() != a.cols() {
		return nil, nil, errors.New("QR decomposition currently requires square matrix")
	}
	n := a.rows()
	q, _ := newMatrix(n, n, 0)
	r, _ := newMatrix(n, n, 0)

	vectors := make([][]float64, n)
	for j := 0; j < n; j++ {
		v := make([]float64, n)
		for i := 0; i < n; i++ {
			v[i] = a.Data[i][j]
		}
		vectors[j] = v
	}

	for j := 0; j < n; j++ {
		for i := 0; i < j; i++ {
			dot := 0.0
			for k := 0; k < n; k++ {
				dot += q.Data[k][i] * vectors[j][k]
			}
			r.Data[i][j] = dot
			for k := 0; k < n; k++ {
				vectors[j][k] -= dot * q.Data[k][i]
			}
		}
		norm := 0.0
		for k := 0; k < n; k++ {
			norm += vectors[j][k] * vectors[j][k]
		}
		norm = math.Sqrt(norm)
		if norm < 1e-12 {
			return nil, nil, errors.New("matrix appears rank deficient for eigen decomposition")
		}
		r.Data[j][j] = norm
		for k := 0; k < n; k++ {
			q.Data[k][j] = vectors[j][k] / norm
		}
	}

	return q, r, nil
}

// matrixStats computes descriptive statistics for a slice of float64 values,
// returning sum, min, max, average, median, and mode. For an empty slice all
// returned values are NaN.
func matrixStats(values []float64) (sum, min, max, avg, median, mode float64) {
	if len(values) == 0 {
		return 0, math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()
	}
	sum = 0
	min, max = values[0], values[0]
	freq := map[float64]int{}
	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		freq[v]++
	}
	avg = sum / float64(len(values))
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	if len(sorted)%2 == 1 {
		median = sorted[len(sorted)/2]
	} else {
		i := len(sorted) / 2
		median = (sorted[i-1] + sorted[i]) / 2
	}
	bestCount := -1
	mode = sorted[0]
	for v, c := range freq {
		if c > bestCount || (c == bestCount && v < mode) {
			bestCount = c
			mode = v
		}
	}
	return
}

// matrixFromArg extracts a *Matrix from a runtime value. It returns an error if the
// value is not a matrix or is malformed.
func matrixFromArg(v interface{}) (*Matrix, error) {
	m, ok := v.(*Matrix)
	if !ok || m == nil {
		return nil, errors.New("argument must be matrix")
	}
	if err := m.validateRect(); err != nil {
		return nil, err
	}
	return m, nil
}

// floatSliceFromInterfaces converts a slice of interface{} values to []float64,
// returning an error if any item is not convertible to a float.
func floatSliceFromInterfaces(items []interface{}) ([]float64, error) {
	out := make([]float64, len(items))
	for i, item := range items {
		f, ok := toFloat(item)
		if !ok || math.IsNaN(f) {
			return nil, fmt.Errorf("array item at %d is not numeric", i)
		}
		out[i] = f
	}
	return out, nil
}

// boolToFloat converts a boolean to 1.0 (true) or 0.0 (false).
func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
