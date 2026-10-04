// QR Model 2 tables ported from user-owned SpecQR at
// 15ad15e5c770ea0e39072f8f88b2733018f02ffd, src/core/tables.js.
// Copyright (c) 2026 SpecQR contributors. MIT license.
package specqr

// ECC identifies a QR error-correction level.
type ECC string

const (
	L ECC = "L"
	M ECC = "M"
	Q ECC = "Q"
	H ECC = "H"
)

func validateVersion(version int) error {
	if version < 1 || version > 40 {
		return errCode(InvalidVersion, "QR version must be from 1 to 40")
	}
	return nil
}
func eccIndex(ecc ECC) (int, error) {
	switch ecc {
	case L:
		return 0, nil
	case M:
		return 1, nil
	case Q:
		return 2, nil
	case H:
		return 3, nil
	}
	return 0, errCode(InvalidECC, "Error correction level must be L, M, Q, or H")
}

// ValidateVersion checks the Model 2 version range.
func ValidateVersion(version int) error { return validateVersion(version) }

// ValidateECC checks the error-correction level.
func ValidateECC(ecc ECC) error { _, err := eccIndex(ecc); return err }

// FormatBits returns the two-bit format encoding of an ECC level.
func FormatBits(ecc ECC) (int, error) {
	i, e := eccIndex(ecc)
	if e != nil {
		return 0, e
	}
	return [...]int{1, 0, 3, 2}[i], nil
}

// Size returns a symbol's side length in modules, excluding quiet zone.
func Size(version int) (int, error) {
	if e := validateVersion(version); e != nil {
		return 0, e
	}
	return 4*version + 17, nil
}

// RawCodewordCount returns all data and parity codewords in a symbol.
func RawCodewordCount(version int) (int, error) {
	if e := validateVersion(version); e != nil {
		return 0, e
	}
	n := (16*version+128)*version + 64
	if version >= 2 {
		c := version/7 + 2
		n -= (25*c-10)*c - 55
		if version >= 7 {
			n -= 36
		}
	}
	return n / 8, nil
}

// BlockInformation describes one version/ECC's Reed-Solomon block layout.
type BlockInformation struct{ Blocks, ECCPerBlock, RawCodewords, DataCodewords int }

// BlockInfo returns the block layout for a version/ECC pair.
func BlockInfo(version int, ecc ECC) (BlockInformation, error) {
	if e := validateVersion(version); e != nil {
		return BlockInformation{}, e
	}
	i, e := eccIndex(ecc)
	if e != nil {
		return BlockInformation{}, e
	}
	raw, _ := RawCodewordCount(version)
	blocks := numErrorCorrectionBlocks[i][version]
	parity := eccCodewordsPerBlock[i][version]
	return BlockInformation{blocks, parity, raw, raw - blocks*parity}, nil
}

// DataCodewordCount returns the data capacity before Reed-Solomon parity.
func DataCodewordCount(version int, ecc ECC) (int, error) {
	b, e := BlockInfo(version, ecc)
	return b.DataCodewords, e
}

// AlignmentPositions returns independent copies of alignment-center coordinates.
func AlignmentPositions(version int) ([]int, error) {
	if e := validateVersion(version); e != nil {
		return nil, e
	}
	if version == 1 {
		return []int{}, nil
	}
	count := version/7 + 2
	den := count*2 - 2
	step := ((version*4 + 4 + den - 1) / den) * 2
	if version == 32 {
		step = 26
	}
	p := make([]int, count)
	p[0] = 6
	side := version*4 + 17
	for i := count - 1; i >= 1; i-- {
		p[i] = side - 7 - (count-1-i)*step
	}
	return p, nil
}

// CountBits returns the character-count field width for a data mode/version.
func CountBits(mode Mode, version int) (int, error) {
	if e := validateVersion(version); e != nil {
		return 0, e
	}
	group := 0
	if version >= 27 {
		group = 2
	} else if version >= 10 {
		group = 1
	}
	switch mode {
	case Numeric:
		return [...]int{10, 12, 14}[group], nil
	case Alphanumeric:
		return [...]int{9, 11, 13}[group], nil
	case Byte:
		return [...]int{8, 16, 16}[group], nil
	case Kanji:
		return [...]int{8, 10, 12}[group], nil
	}
	return 0, errCode(InvalidMode, "Count fields require a data mode")
}

var eccCodewordsPerBlock = [4][41]int{
	{-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

var numErrorCorrectionBlocks = [4][41]int{
	{-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}
