// Dependency-free QR Model 2 block coding, module placement, and mask selection.
// Ported from user-owned SpecQR commit 15ad15e5c770ea0e39072f8f88b2733018f02ffd,
// src/core/{codewords,galois-field,reed-solomon,matrix,mask}.js.
// Copyright (c) 2026 SpecQR contributors. MIT license.
package specqr

import "fmt"

const maxCodewords = 3706

// CodewordBlock contains one data block and its Reed-Solomon parity bytes.
type CodewordBlock struct{ Data, ECC []byte }

// InterleavedResult contains complete independently owned encoding diagnostics.
type InterleavedResult struct {
	Codewords                                               []byte
	Blocks                                                  []CodewordBlock
	DataCodewords, ErrorCorrectionCodewords, TotalCodewords int
}

// MaskPenalty records the score of one candidate QR mask.
type MaskPenalty struct {
	MaskPattern int `json:"maskPattern"`
	Penalty     int `json:"penalty"`
}

// MatrixResult contains rows indexed [y][x], excluding the quiet zone.
// A forced mask produces one penalty entry; automatic selection produces eight.
type MatrixResult struct {
	Matrix               [][]bool
	MaskPattern, Penalty int
	MaskPenalties        []MaskPenalty
}

// PadDataBits checks bit values and capacity before allocating data codewords.
// Bits are bytes with values exactly 0 or 1. Terminator, alignment, and EC/11 pads
// are added to fill the exact version/ECC capacity.
func PadDataBits(bits []byte, version int, ecc ECC) ([]byte, error) {
	capacity, e := DataCodewordCount(version, ecc)
	if e != nil {
		return nil, e
	}
	if len(bits) > capacity*8 {
		return nil, errCode(DataTooLong, fmt.Sprintf("Input needs %d bits; version %d-%s has %d", len(bits), version, ecc, capacity*8))
	}
	for _, v := range bits {
		if v > 1 {
			return nil, errCode(InvalidInput, "Bits must contain only 0 and 1")
		}
	}
	result := make([]byte, capacity)
	for i, v := range bits {
		result[i/8] |= v << uint(7-i%8)
	}
	terminated := len(bits) + min(4, capacity*8-len(bits))
	padded := (terminated + 7) / 8
	for i := padded; i < capacity; i++ {
		if (i-padded)%2 == 0 {
			result[i] = 0xec
		} else {
			result[i] = 0x11
		}
	}
	return result, nil
}
func gfMultiply(left, right int) int {
	product := 0
	for right != 0 {
		if right&1 != 0 {
			product ^= left
		}
		right >>= 1
		left <<= 1
		if left&0x100 != 0 {
			left ^= 0x11d
		}
	}
	return product
}

// GFMultiply multiplies byte values in QR's GF(256), polynomial 0x11D.
func GFMultiply(left, right int) (int, error) {
	if left < 0 || left > 255 || right < 0 || right > 255 {
		return 0, errCode(InvalidInput, "GF operands must be from 0 to 255")
	}
	return gfMultiply(left, right), nil
}

// ReedSolomonDivisor returns descending coefficients including the leading 1.
func ReedSolomonDivisor(degree int) ([]byte, error) {
	if degree < 1 || degree > 255 {
		return nil, errCode(InvalidInput, "Reed-Solomon degree must be from 1 to 255")
	}
	coefficients := make([]byte, degree+1)
	coefficients[0] = 1
	root := 1
	for factor := 0; factor < degree; factor++ {
		for i := factor + 1; i >= 1; i-- {
			coefficients[i] ^= byte(gfMultiply(int(coefficients[i-1]), root))
		}
		root = gfMultiply(root, 2)
	}
	return coefficients, nil
}
func rsRemainder(data, divisor []byte) []byte {
	degree := len(divisor) - 1
	result := make([]byte, degree)
	for _, v := range data {
		factor := int(v ^ result[0])
		for i := 0; i < degree-1; i++ {
			result[i] = result[i+1] ^ byte(gfMultiply(int(divisor[i+1]), factor))
		}
		result[degree-1] = byte(gfMultiply(int(divisor[degree]), factor))
	}
	return result
}

// ReedSolomonRemainder computes at most one symbol's worth of parity input.
func ReedSolomonRemainder(data []byte, degree int) ([]byte, error) {
	if len(data) > maxCodewords {
		return nil, errCode(InvalidInput, "Data exceeds maximum QR codeword count")
	}
	divisor, e := ReedSolomonDivisor(degree)
	if e != nil {
		return nil, e
	}
	return rsRemainder(data, divisor), nil
}

// InterleaveCodewords splits exact-length padded data into RS blocks, computes
// parity, and interleaves short/long blocks according to the Model 2 tables.
func InterleaveCodewords(data []byte, version int, ecc ECC) (InterleavedResult, error) {
	info, e := BlockInfo(version, ecc)
	if e != nil {
		return InterleavedResult{}, e
	}
	if len(data) != info.DataCodewords {
		return InterleavedResult{}, errCode(InvalidInput, fmt.Sprintf("Expected %d data codewords; got %d", info.DataCodewords, len(data)))
	}
	shortCount := info.Blocks - info.RawCodewords%info.Blocks
	shortLength := info.RawCodewords/info.Blocks - info.ECCPerBlock
	divisor, _ := ReedSolomonDivisor(info.ECCPerBlock)
	blocks := make([]CodewordBlock, info.Blocks)
	offset := 0
	for i := range blocks {
		length := shortLength
		if i >= shortCount {
			length++
		}
		blockData := append([]byte(nil), data[offset:offset+length]...)
		blocks[i] = CodewordBlock{blockData, rsRemainder(blockData, divisor)}
		offset += length
	}
	result := make([]byte, 0, info.RawCodewords)
	for column := 0; column <= shortLength; column++ {
		for _, b := range blocks {
			if column < len(b.Data) {
				result = append(result, b.Data[column])
			}
		}
	}
	for column := 0; column < info.ECCPerBlock; column++ {
		for _, b := range blocks {
			result = append(result, b.ECC[column])
		}
	}
	if offset != len(data) || len(result) != info.RawCodewords {
		return InterleavedResult{}, errCode(InvalidInput, "Inconsistent QR block interleaving length")
	}
	return InterleavedResult{result, blocks, info.DataCodewords, info.RawCodewords - info.DataCodewords, info.RawCodewords}, nil
}
func maskCondition(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

// MaskCondition evaluates one mask at a valid Model 2 module coordinate.
func MaskCondition(mask, x, y int) (bool, error) {
	if mask < 0 || mask > 7 || x < 0 || x > 176 || y < 0 || y > 176 {
		return false, errCode(InvalidInput, "Mask must be 0..7 and coordinates 0..176")
	}
	return maskCondition(mask, x, y), nil
}
func linePenalty(line []bool) int {
	penalty, run, window := 0, 0, 0
	var color bool
	for i, value := range line {
		if i != 0 && value == color {
			run++
		} else {
			if run >= 5 {
				penalty += run - 2
			}
			color = value
			run = 1
		}
		window = window << 1 & 0x7ff
		if value {
			window |= 1
		}
		if i >= 10 && (window == 0b10111010000 || window == 0b00001011101) {
			penalty += 40
		}
	}
	if run >= 5 {
		penalty += run - 2
	}
	return penalty
}
func penaltyScore(matrix [][]bool) int {
	side := len(matrix)
	score, dark := 0, 0
	column := make([]bool, side)
	for _, row := range matrix {
		score += linePenalty(row)
		for _, v := range row {
			if v {
				dark++
			}
		}
	}
	for x := 0; x < side; x++ {
		for y := 0; y < side; y++ {
			column[y] = matrix[y][x]
		}
		score += linePenalty(column)
	}
	for y := 0; y < side-1; y++ {
		for x := 0; x < side-1; x++ {
			v := matrix[y][x]
			if v == matrix[y][x+1] && v == matrix[y+1][x] && v == matrix[y+1][x+1] {
				score += 3
			}
		}
	}
	total := side * side
	balance := dark*20 - total*10
	if balance < 0 {
		balance = -balance
	}
	return score + balance/total*10
}

// PenaltyScore applies SpecQR's exact N1/N2/N3/N4 rules to a square 1..177 grid.
func PenaltyScore(matrix [][]bool) (int, error) {
	if len(matrix) < 1 || len(matrix) > 177 {
		return 0, errCode(InvalidInput, "Matrix must have 1 to 177 square rows")
	}
	for _, row := range matrix {
		if len(row) != len(matrix) {
			return 0, errCode(InvalidInput, "Matrix must be square")
		}
	}
	return penaltyScore(matrix), nil
}

type qrGrid struct {
	side               int
	modules, functions [][]bool
}

func newGrid(side int) *qrGrid {
	g := &qrGrid{side: side, modules: make([][]bool, side), functions: make([][]bool, side)}
	for i := 0; i < side; i++ {
		g.modules[i] = make([]bool, side)
		g.functions[i] = make([]bool, side)
	}
	return g
}
func (g *qrGrid) function(x, y int, dark bool) {
	if x >= 0 && x < g.side && y >= 0 && y < g.side {
		g.modules[y][x] = dark
		g.functions[y][x] = true
	}
}
func (g *qrGrid) finder(left, top int) {
	for dy := -1; dy <= 7; dy++ {
		for dx := -1; dx <= 7; dx++ {
			inside := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6
			dark := inside && (dx == 0 || dx == 6 || dy == 0 || dy == 6 || dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4)
			g.function(left+dx, top+dy, dark)
		}
	}
}
func (g *qrGrid) drawFormat(ecc ECC, mask int) {
	level, _ := FormatBits(ecc)
	data := level<<3 | mask
	remainder := data
	for i := 0; i < 10; i++ {
		remainder = remainder<<1 ^ ((remainder >> 9 & 1) * 0x537)
	}
	bits := (data<<10 | remainder) ^ 0x5412
	bit := func(i int) bool { return bits>>i&1 != 0 }
	for i := 0; i < 6; i++ {
		g.function(8, i, bit(i))
	}
	g.function(8, 7, bit(6))
	g.function(8, 8, bit(7))
	g.function(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		g.function(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		g.function(g.side-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		g.function(8, g.side-15+i, bit(i))
	}
}
func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
func (g *qrGrid) drawFunctions(version int, ecc ECC) {
	g.finder(0, 0)
	g.finder(g.side-7, 0)
	g.finder(0, g.side-7)
	for i := 8; i < g.side-8; i++ {
		g.function(i, 6, i%2 == 0)
		g.function(6, i, i%2 == 0)
	}
	positions, _ := AlignmentPositions(version)
	last := len(positions) - 1
	for yi, y := range positions {
		for xi, x := range positions {
			if xi == 0 && yi == 0 || xi == last && yi == 0 || xi == 0 && yi == last {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					g.function(x+dx, y+dy, max(absInt(dx), absInt(dy)) != 1)
				}
			}
		}
	}
	g.drawFormat(ecc, 0)
	g.function(8, g.side-8, true)
	if version >= 7 {
		remainder := version
		for i := 0; i < 12; i++ {
			remainder = remainder<<1 ^ ((remainder >> 11 & 1) * 0x1f25)
		}
		bits := version<<12 | remainder
		for i := 0; i < 18; i++ {
			a, b := g.side-11+i%3, i/3
			dark := bits>>i&1 != 0
			g.function(a, b, dark)
			g.function(b, a, dark)
		}
	}
}
func (g *qrGrid) drawCodewords(codewords []byte) error {
	bitIndex := 0
	for right := g.side - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vertical := 0; vertical < g.side; vertical++ {
			y := vertical
			if (right+1)&2 == 0 {
				y = g.side - 1 - vertical
			}
			for offset := 0; offset < 2; offset++ {
				x := right - offset
				if !g.functions[y][x] {
					if bitIndex < len(codewords)*8 {
						g.modules[y][x] = (codewords[bitIndex/8]>>uint(7-bitIndex%8))&1 != 0
					}
					bitIndex++
				}
			}
		}
	}
	if bitIndex-len(codewords)*8 < 0 || bitIndex-len(codewords)*8 > 7 {
		return errCode(InvalidInput, "Inconsistent QR data-module count")
	}
	return nil
}
func (g *qrGrid) masked(ecc ECC, mask int) *qrGrid {
	candidate := &qrGrid{side: g.side, modules: make([][]bool, g.side), functions: g.functions}
	for y := 0; y < g.side; y++ {
		candidate.modules[y] = append([]bool(nil), g.modules[y]...)
		for x := 0; x < g.side; x++ {
			if !g.functions[y][x] && maskCondition(mask, x, y) {
				candidate.modules[y][x] = !candidate.modules[y][x]
			}
		}
	}
	candidate.drawFormat(ecc, mask)
	return candidate
}

// BuildMatrix places full interleaved codewords and selects the lowest-penalty
// mask, breaking exact ties in favor of the lowest mask number. A nil mask means
// automatic selection; otherwise the pointed value must be from 0 to 7.
func BuildMatrix(codewords []byte, version int, ecc ECC, mask *int) (MatrixResult, error) {
	side, e := Size(version)
	if e != nil {
		return MatrixResult{}, e
	}
	if e := ValidateECC(ecc); e != nil {
		return MatrixResult{}, e
	}
	if mask != nil && (*mask < 0 || *mask > 7) {
		return MatrixResult{}, errCode(InvalidInput, "Mask must be from 0 to 7")
	}
	expected, _ := RawCodewordCount(version)
	if len(codewords) != expected {
		return MatrixResult{}, errCode(InvalidInput, fmt.Sprintf("Expected %d interleaved codewords; got %d", expected, len(codewords)))
	}
	base := newGrid(side)
	base.drawFunctions(version, ecc)
	if e := base.drawCodewords(codewords); e != nil {
		return MatrixResult{}, e
	}
	start, end := 0, 8
	if mask != nil {
		start = *mask
		end = start + 1
	}
	result := MatrixResult{MaskPenalties: make([]MaskPenalty, 0, end-start)}
	for candidateMask := start; candidateMask < end; candidateMask++ {
		candidate := base.masked(ecc, candidateMask)
		penalty := penaltyScore(candidate.modules)
		result.MaskPenalties = append(result.MaskPenalties, MaskPenalty{candidateMask, penalty})
		if result.Matrix == nil || penalty < result.Penalty {
			result.Matrix = candidate.modules
			result.MaskPattern = candidateMask
			result.Penalty = penalty
		}
	}
	return result, nil
}
