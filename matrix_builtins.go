// SPDX-FileCopyrightText: 2026 Woodstock K.K.
//
// SPDX-License-Identifier: AGPL-3.0-only

package pinescription

import (
	"errors"
	"fmt"
	"math"
)

func (r *Runtime) callMatrixBuiltin(name string, args []interface{}) (interface{}, bool, error) {
	switch name {
	case "matrix.new", "matrix.new_float", "matrix.new_int", "matrix.new_bool", "matrix.new_string":
		if len(args) < 2 || len(args) > 3 {
			return nil, true, fmt.Errorf("%s expects (rows, cols, [fill])", name)
		}
		rowsF, _ := toFloat(args[0])
		colsF, _ := toFloat(args[1])
		fill := 0.0
		if len(args) == 3 {
			fill, _ = toFloat(args[2])
		}
		m, err := newMatrix(int(rowsF), int(colsF), fill)
		return m, true, err
	case "matrix.copy":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return m.copy(), true, nil
	case "matrix.get":
		m, i, j, err := matrixIJ(args)
		if err != nil {
			return nil, true, err
		}
		v, err := m.get(i, j)
		return v, true, err
	case "matrix.set":
		if len(args) != 4 {
			return nil, true, errors.New("matrix.set expects (matrix, row, col, value)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		rf, _ := toFloat(args[1])
		cf, _ := toFloat(args[2])
		v, _ := toFloat(args[3])
		if err := m.set(int(rf), int(cf), v); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.row":
		if len(args) != 2 {
			return nil, true, errors.New("matrix.row expects (matrix, row)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		rf, _ := toFloat(args[1])
		row, err := m.row(int(rf))
		return row, true, err
	case "matrix.col":
		if len(args) != 2 {
			return nil, true, errors.New("matrix.col expects (matrix, col)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		cf, _ := toFloat(args[1])
		col, err := m.col(int(cf))
		return col, true, err
	case "matrix.rows":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return float64(m.rows()), true, nil
	case "matrix.columns":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return float64(m.cols()), true, nil
	case "matrix.elements_count":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return float64(m.elementsCount()), true, nil
	case "matrix.fill":
		if len(args) != 2 && len(args) != 6 {
			return nil, true, errors.New("matrix.fill expects (matrix, value) or (matrix, value, row_from, row_to, col_from, col_to)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		v, _ := toFloat(args[1])
		if len(args) == 2 {
			m.fill(v)
		} else {
			rf, _ := toFloat(args[2])
			rt, _ := toFloat(args[3])
			cf, _ := toFloat(args[4])
			ct, _ := toFloat(args[5])
			if err := m.fillRange(v, int(rf), int(rt), int(cf), int(ct)); err != nil {
				return nil, true, err
			}
		}
		return m, true, nil
	case "matrix.reshape":
		if len(args) != 3 {
			return nil, true, errors.New("matrix.reshape expects (matrix, rows, cols)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		rf, _ := toFloat(args[1])
		cf, _ := toFloat(args[2])
		if err := m.reshape(int(rf), int(cf)); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.submatrix":
		if len(args) != 5 {
			return nil, true, errors.New("matrix.submatrix expects (matrix, row_from, row_to, col_from, col_to)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		rf, _ := toFloat(args[1])
		rt, _ := toFloat(args[2])
		cf, _ := toFloat(args[3])
		ct, _ := toFloat(args[4])
		sub, err := m.submatrix(int(rf), int(rt), int(cf), int(ct))
		return sub, true, err
	case "matrix.add_row":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		if len(args) != 2 && len(args) != 3 {
			return nil, true, errors.New("matrix.add_row expects (matrix, row_array) or (matrix, index, row_array)")
		}
		idx := m.rows()
		arrArg := args[1]
		if len(args) == 3 {
			idxF, _ := toFloat(args[1])
			idx = int(idxF)
			arrArg = args[2]
		}
		arr, ok := arrArg.([]interface{})
		if !ok {
			return nil, true, errors.New("matrix.add_row requires array argument")
		}
		row, err := floatSliceFromInterfaces(arr)
		if err != nil {
			return nil, true, err
		}
		if err := m.addRow(idx, row); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.add_col":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		if len(args) != 2 && len(args) != 3 {
			return nil, true, errors.New("matrix.add_col expects (matrix, col_array) or (matrix, index, col_array)")
		}
		idx := m.cols()
		arrArg := args[1]
		if len(args) == 3 {
			idxF, _ := toFloat(args[1])
			idx = int(idxF)
			arrArg = args[2]
		}
		arr, ok := arrArg.([]interface{})
		if !ok {
			return nil, true, errors.New("matrix.add_col requires array argument")
		}
		col, err := floatSliceFromInterfaces(arr)
		if err != nil {
			return nil, true, err
		}
		if err := m.addCol(idx, col); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.remove_row":
		if len(args) != 2 {
			return nil, true, errors.New("matrix.remove_row expects (matrix, row)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		rf, _ := toFloat(args[1])
		if err := m.removeRow(int(rf)); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.remove_col":
		if len(args) != 2 {
			return nil, true, errors.New("matrix.remove_col expects (matrix, col)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		cf, _ := toFloat(args[1])
		if err := m.removeCol(int(cf)); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.swap_rows":
		if len(args) != 3 {
			return nil, true, errors.New("matrix.swap_rows expects (matrix, row1, row2)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		a, _ := toFloat(args[1])
		b, _ := toFloat(args[2])
		if err := m.swapRows(int(a), int(b)); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.swap_columns":
		if len(args) != 3 {
			return nil, true, errors.New("matrix.swap_columns expects (matrix, col1, col2)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		a, _ := toFloat(args[1])
		b, _ := toFloat(args[2])
		if err := m.swapCols(int(a), int(b)); err != nil {
			return nil, true, err
		}
		return m, true, nil
	case "matrix.reverse":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		m.reverse()
		return m, true, nil
	case "matrix.sort":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		m.sort()
		return m, true, nil
	case "matrix.sum", "matrix.avg", "matrix.min", "matrix.max", "matrix.median", "matrix.mode":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		sum, minV, maxV, avg, median, mode := matrixStats(m.flatten())
		switch name {
		case "matrix.sum":
			return sum, true, nil
		case "matrix.avg":
			return avg, true, nil
		case "matrix.min":
			return minV, true, nil
		case "matrix.max":
			return maxV, true, nil
		case "matrix.median":
			return median, true, nil
		default:
			return mode, true, nil
		}
	case "matrix.concat":
		a, b, err := matrixAB(args)
		if err != nil {
			return nil, true, err
		}
		out, err := matrixConcat(a, b)
		return out, true, err
	case "matrix.diff":
		a, b, err := matrixAB(args)
		if err != nil {
			return nil, true, err
		}
		out, err := matrixDiff(a, b)
		return out, true, err
	case "matrix.kron":
		a, b, err := matrixAB(args)
		if err != nil {
			return nil, true, err
		}
		return matrixKron(a, b), true, nil
	case "matrix.mult":
		a, b, err := matrixAB(args)
		if err != nil {
			return nil, true, err
		}
		out, err := matrixMult(a, b)
		return out, true, err
	case "matrix.pow":
		if len(args) != 2 {
			return nil, true, errors.New("matrix.pow expects (matrix, power)")
		}
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		pf, _ := toFloat(args[1])
		out, err := matrixPow(m, int(pf))
		return out, true, err
	case "matrix.transpose":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return matrixTranspose(m), true, nil
	case "matrix.det":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		d, err := matrixDet(m)
		return d, true, err
	case "matrix.rank":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		return float64(matrixRank(m)), true, nil
	case "matrix.trace":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		tr, err := matrixTrace(m)
		return tr, true, err
	case "matrix.inv":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		out, err := matrixInv(m)
		return out, true, err
	case "matrix.pinv":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		out, err := matrixPinv(m)
		return out, true, err
	case "matrix.eigenvalues":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		vals, _, err := matrixEigen(m)
		if err != nil {
			return nil, true, err
		}
		out := make([]interface{}, len(vals))
		for i := range vals {
			out[i] = vals[i]
		}
		return out, true, nil
	case "matrix.eigenvectors":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		_, vecs, err := matrixEigen(m)
		return vecs, true, err
	case "matrix.is_square", "matrix.is_symmetric", "matrix.is_diagonal", "matrix.is_identity", "matrix.is_zero", "matrix.is_triangular", "matrix.is_binary", "matrix.is_antidiagonal", "matrix.is_antisymmetric", "matrix.is_stochastic":
		m, err := matrixFromArgAt(args, 0)
		if err != nil {
			return nil, true, err
		}
		v := matrixProperty(name, m)
		return v, true, nil
	default:
		return nil, false, nil
	}
}

func matrixProperty(name string, m *Matrix) bool {
	r, c := m.rows(), m.cols()
	square := r == c
	eps := 1e-9
	switch name {
	case "matrix.is_square":
		return square
	case "matrix.is_zero":
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if math.Abs(m.Data[i][j]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_binary":
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				v := m.Data[i][j]
				if math.Abs(v) > eps && math.Abs(v-1) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_diagonal":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if i != j && math.Abs(m.Data[i][j]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_antidiagonal":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if i+j != r-1 && math.Abs(m.Data[i][j]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_identity":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if i == j {
					if math.Abs(m.Data[i][j]-1) > eps {
						return false
					}
				} else if math.Abs(m.Data[i][j]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_symmetric":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			for j := i + 1; j < c; j++ {
				if math.Abs(m.Data[i][j]-m.Data[j][i]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_antisymmetric":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			if math.Abs(m.Data[i][i]) > eps {
				return false
			}
			for j := i + 1; j < c; j++ {
				if math.Abs(m.Data[i][j]+m.Data[j][i]) > eps {
					return false
				}
			}
		}
		return true
	case "matrix.is_triangular":
		if !square {
			return false
		}
		upper := true
		lower := true
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if i > j && math.Abs(m.Data[i][j]) > eps {
					upper = false
				}
				if i < j && math.Abs(m.Data[i][j]) > eps {
					lower = false
				}
			}
		}
		return upper || lower
	case "matrix.is_stochastic":
		if !square {
			return false
		}
		for i := 0; i < r; i++ {
			s := 0.0
			for j := 0; j < c; j++ {
				if m.Data[i][j] < -eps {
					return false
				}
				s += m.Data[i][j]
			}
			if math.Abs(s-1) > 1e-6 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func matrixFromArgAt(args []interface{}, idx int) (*Matrix, error) {
	if len(args) <= idx {
		return nil, errors.New("missing matrix argument")
	}
	return matrixFromArg(args[idx])
}

func matrixIJ(args []interface{}) (*Matrix, int, int, error) {
	if len(args) != 3 {
		return nil, 0, 0, errors.New("matrix.get expects (matrix, row, col)")
	}
	m, err := matrixFromArgAt(args, 0)
	if err != nil {
		return nil, 0, 0, err
	}
	rf, _ := toFloat(args[1])
	cf, _ := toFloat(args[2])
	return m, int(rf), int(cf), nil
}

func matrixAB(args []interface{}) (*Matrix, *Matrix, error) {
	if len(args) != 2 {
		return nil, nil, errors.New("operation expects (matrixA, matrixB)")
	}
	a, err := matrixFromArgAt(args, 0)
	if err != nil {
		return nil, nil, err
	}
	b, err := matrixFromArgAt(args, 1)
	if err != nil {
		return nil, nil, err
	}
	return a, b, nil
}
