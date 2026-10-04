// Immutable QR segments, ported from user-owned SpecQR at
// 15ad15e5c770ea0e39072f8f88b2733018f02ffd, src/encoding/{modes,control-segments}.js.
// Copyright (c) 2026 SpecQR contributors. MIT license.
package specqr

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Mode identifies a data or control segment. Auto is a planning option only.
type Mode string

const (
	Auto             Mode = "auto"
	Numeric          Mode = "numeric"
	Alphanumeric     Mode = "alphanumeric"
	Byte             Mode = "byte"
	Kanji            Mode = "kanji"
	ECI              Mode = "eci"
	FNC1First        Mode = "fnc1"
	FNC1Second       Mode = "fnc1-second"
	StructuredAppend Mode = "structured-append"
)
const (
	AlphanumericCharset       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
	MaxPayloadUnits           = 1_000_000
	MaxManualSegments         = 16_384
	MaxSingleSymbolCharacters = 7_089
	MaxSingleSymbolDataBits   = 23_648
)

// IsControl reports whether the mode carries a control header.
func (m Mode) IsControl() bool {
	return m == ECI || m == FNC1First || m == FNC1Second || m == StructuredAppend
}
func modeIndicator(m Mode) int {
	switch m {
	case Numeric:
		return 1
	case Alphanumeric:
		return 2
	case StructuredAppend:
		return 3
	case Byte:
		return 4
	case FNC1First:
		return 5
	case ECI:
		return 7
	case Kanji:
		return 8
	case FNC1Second:
		return 9
	}
	return -1
}
func alphaValue(r rune) int {
	if r > 127 || r < 0 {
		return -1
	}
	return strings.IndexByte(AlphanumericCharset, byte(r))
}
func validateText(text string) (int, error) {
	if len(text) > 4*MaxPayloadUnits {
		return 0, errCode(DataTooLong, "Text exceeds the payload resource limit")
	}
	if !utf8.ValidString(text) {
		return 0, errCode(InvalidInput, "Text must be well-formed UTF-8")
	}
	count := utf8.RuneCountInString(text)
	if count > MaxPayloadUnits {
		return 0, errCode(DataTooLong, "Text exceeds the payload resource limit")
	}
	return count, nil
}

// Segment is an owned, immutable data/control segment. Its zero value is invalid.
// Text byte payloads use strict UTF-8; ECI labels bytes without transcoding.
// Manual FNC1 alphanumeric data is already escaped: % denotes a separator,
// and %% denotes a literal percent. This low-level type preserves it verbatim.
type Segment struct {
	mode                 Mode
	data                 string
	textual              bool
	characters           int
	assignment           int
	application          string
	index, total, parity int
	valid                bool
}

func textSegment(mode Mode, text string) (Segment, error) {
	if mode != Numeric && mode != Alphanumeric && mode != Byte && mode != Kanji {
		return Segment{}, errCode(InvalidMode, "Text requires a data mode")
	}
	count, e := validateText(text)
	if e != nil {
		return Segment{}, e
	}
	for _, r := range text {
		ok := mode == Byte || mode == Numeric && r >= '0' && r <= '9' || mode == Alphanumeric && alphaValue(r) >= 0 || mode == Kanji && CanEncodeKanji(r)
		if !ok {
			return Segment{}, errCode(InvalidMode, fmt.Sprintf("%s mode cannot encode U+%04X", mode, r))
		}
	}
	return Segment{mode: mode, data: text, textual: true, characters: count, valid: true}, nil
}

// NumericSegment constructs a decimal-digit segment.
func NumericSegment(text string) (Segment, error) { return textSegment(Numeric, text) }

// AlphanumericSegment constructs a segment using the QR 45-character alphabet.
func AlphanumericSegment(text string) (Segment, error) { return textSegment(Alphanumeric, text) }

// UTF8Segment constructs a strict UTF-8 byte-mode text segment.
func UTF8Segment(text string) (Segment, error) { return textSegment(Byte, text) }

// BytesSegment takes an immutable owned copy of raw binary bytes.
func BytesSegment(data []byte) (Segment, error) {
	if len(data) > MaxPayloadUnits {
		return Segment{}, errCode(DataTooLong, "Bytes exceed the payload resource limit")
	}
	return Segment{mode: Byte, data: string(data), valid: true}, nil
}

// KanjiSegment constructs a segment using the pinned WHATWG Shift-JIS repertoire.
func KanjiSegment(text string) (Segment, error) { return textSegment(Kanji, text) }

// ECISegment labels subsequent byte segments without transcoding their contents.
func ECISegment(assignment int) (Segment, error) {
	if assignment < 0 || assignment > 999999 {
		return Segment{}, errCode(InvalidECI, "ECI assignment must be from 0 to 999999")
	}
	return Segment{mode: ECI, assignment: assignment, valid: true}, nil
}

// FNC1Segment constructs the GS1/FNC1 first-position header.
func FNC1Segment() (Segment, error) { return Segment{mode: FNC1First, valid: true}, nil }

// FNC1SecondSegment accepts two ASCII digits or one ASCII Latin letter.
func FNC1SecondSegment(indicator string) (Segment, error) {
	valid := len(indicator) == 2 && indicator[0] >= '0' && indicator[0] <= '9' && indicator[1] >= '0' && indicator[1] <= '9' || len(indicator) == 1 && (indicator[0] >= 'A' && indicator[0] <= 'Z' || indicator[0] >= 'a' && indicator[0] <= 'z')
	if !valid {
		return Segment{}, errCode(InvalidMode, "FNC1 second indicator must be two ASCII digits or one ASCII letter")
	}
	return Segment{mode: FNC1Second, application: indicator, valid: true}, nil
}

// StructuredAppendSegment uses one-based index and total, with total from 2 to 16.
func StructuredAppendSegment(index, total, parity int) (Segment, error) {
	if total < 2 || total > 16 || index < 1 || index > total || parity < 0 || parity > 255 {
		return Segment{}, errCode(InvalidStructuredAppend, "Structured Append needs index 1..total, total 2..16, and parity 0..255")
	}
	return Segment{mode: StructuredAppend, index: index, total: total, parity: parity, valid: true}, nil
}
func (s Segment) Mode() Mode { return s.mode }
func (s Segment) Text() string {
	if s.textual {
		return s.data
	}
	return ""
}
func (s Segment) IsBinary() bool       { return s.mode == Byte && !s.textual }
func (s Segment) IsControl() bool      { return s.mode.IsControl() }
func (s Segment) Data() []byte         { return []byte(s.data) }
func (s Segment) LogicalBytes() []byte { return []byte(s.data) }
func (s Segment) Count() int {
	if s.mode.IsControl() {
		return 0
	}
	if s.mode == Byte {
		return len(s.data)
	}
	return s.characters
}
func (s Segment) CharacterCount() int { return s.characters }
func (s Segment) ByteCount() int {
	if s.mode == Kanji {
		return s.characters * 2
	}
	return len(s.data)
}
func (s Segment) ECIAssignment() int           { return s.assignment }
func (s Segment) ApplicationIndicator() string { return s.application }
func (s Segment) ApplicationIndicatorCodeword() int {
	if s.mode != FNC1Second {
		return 0
	}
	if len(s.application) == 2 {
		return int(s.application[0]-'0')*10 + int(s.application[1]-'0')
	}
	if len(s.application) == 1 {
		return int(s.application[0]) + 100
	}
	return 0
}
func (s Segment) Index() int  { return s.index }
func (s Segment) Total() int  { return s.total }
func (s Segment) Parity() int { return s.parity }
func (s Segment) payloadUnits() int {
	if s.textual {
		return s.characters
	}
	return len(s.data)
}

// PayloadBitLength gives only data payload bits for a bounded nonnegative count.
func PayloadBitLength(mode Mode, count int) (int, error) {
	if count < 0 || count > 4*MaxPayloadUnits {
		return 0, errCode(DataTooLong, "Payload count exceeds the resource limit")
	}
	switch mode {
	case Numeric:
		return count/3*10 + [...]int{0, 4, 7}[count%3], nil
	case Alphanumeric:
		return count/2*11 + count%2*6, nil
	case Byte:
		return count * 8, nil
	case Kanji:
		return count * 13, nil
	}
	return 0, errCode(InvalidMode, "Payload length requires a data mode")
}

// BitLength returns the full unpadded header/payload length for planning.
// The arithmetic may exceed a count field; Bits enforces count/materialization limits.
func (s Segment) BitLength(version int) (int, error) {
	if e := validateVersion(version); e != nil {
		return 0, e
	}
	if !s.valid {
		return 0, errCode(InvalidInput, "Zero-value Segment is invalid")
	}
	switch s.mode {
	case ECI:
		if s.assignment < 128 {
			return 12, nil
		}
		if s.assignment < 16384 {
			return 20, nil
		}
		return 28, nil
	case FNC1First:
		return 4, nil
	case FNC1Second:
		return 12, nil
	case StructuredAppend:
		return 20, nil
	}
	width, e := CountBits(s.mode, version)
	if e != nil {
		return 0, e
	}
	payload, e := PayloadBitLength(s.mode, s.Count())
	return 4 + width + payload, e
}

// ValidateSegments checks manual boundaries, controls, and aggregate resource caps.
// Repeated ECI changes are allowed; distinct control families cannot be combined.
func ValidateSegments(segments []Segment) error {
	if len(segments) > MaxManualSegments {
		return errCode(DataTooLong, "Manual segments exceed the 16384-segment resource limit")
	}
	units := 0
	controls := [4]bool{}
	for pos, s := range segments {
		if !s.valid || modeIndicator(s.mode) < 0 {
			return errCode(InvalidInput, "Zero-value or invalid Segment")
		}
		units += s.payloadUnits()
		if units > MaxPayloadUnits {
			return errCode(DataTooLong, "Manual payload exceeds the resource limit")
		}
		control := -1
		switch s.mode {
		case FNC1First:
			control = 0
		case FNC1Second:
			control = 1
		case StructuredAppend:
			control = 2
		case ECI:
			control = 3
		}
		if control >= 0 {
			if control != 3 && (pos != 0 || controls[control]) {
				code := InvalidMode
				if control == 0 {
					code = InvalidGS1
				}
				return errCode(code, "FNC1 and Structured Append must occur once at the start")
			}
			controls[control] = true
		}
	}
	count := 0
	for _, v := range controls {
		if v {
			count++
		}
	}
	if count > 1 {
		code := InvalidMode
		if controls[0] {
			code = InvalidGS1
		}
		return errCode(code, "FNC1, FNC1 second, Structured Append, and ECI cannot be combined")
	}
	return nil
}

// BitLength sums unpadded segment lengths after validating the manual sequence.
func BitLength(segments []Segment, version int) (int, error) {
	if e := validateVersion(version); e != nil {
		return 0, e
	}
	if e := ValidateSegments(segments); e != nil {
		return 0, e
	}
	total := 0
	for _, s := range segments {
		n, e := s.BitLength(version)
		if e != nil {
			return 0, e
		}
		total += n
	}
	return total, nil
}
func appendBits(bits []byte, value, width int) []byte {
	for shift := width - 1; shift >= 0; shift-- {
		bits = append(bits, byte(value>>shift&1))
	}
	return bits
}
func (s Segment) checkMaterialization(version, length int) error {
	if length > MaxSingleSymbolDataBits {
		return errCode(DataTooLong, "Segment exceeds maximum single-symbol data capacity")
	}
	if !s.IsControl() {
		width, e := CountBits(s.mode, version)
		if e != nil {
			return e
		}
		if s.Count() >= 1<<width {
			return errCode(DataTooLong, "Segment count exceeds its character-count field")
		}
	}
	return nil
}
func (s Segment) appendBits(bits []byte, version int) []byte {
	bits = appendBits(bits, modeIndicator(s.mode), 4)
	switch s.mode {
	case ECI:
		if s.assignment < 128 {
			return appendBits(bits, s.assignment, 8)
		}
		if s.assignment < 16384 {
			return appendBits(appendBits(bits, 2, 2), s.assignment, 14)
		}
		return appendBits(appendBits(bits, 6, 3), s.assignment, 21)
	case FNC1First:
		return bits
	case FNC1Second:
		return appendBits(bits, s.ApplicationIndicatorCodeword(), 8)
	case StructuredAppend:
		return appendBits(appendBits(appendBits(bits, s.index-1, 4), s.total-1, 4), s.parity, 8)
	}
	width, _ := CountBits(s.mode, version)
	bits = appendBits(bits, s.Count(), width)
	switch s.mode {
	case Byte:
		for i := 0; i < len(s.data); i++ {
			bits = appendBits(bits, int(s.data[i]), 8)
		}
	case Numeric:
		for start := 0; start < len(s.data); start += 3 {
			end := min(start+3, len(s.data))
			value := 0
			for i := start; i < end; i++ {
				value = value*10 + int(s.data[i]-'0')
			}
			bits = appendBits(bits, value, [...]int{0, 4, 7, 10}[end-start])
		}
	case Alphanumeric:
		i := 0
		for ; i+1 < len(s.data); i += 2 {
			bits = appendBits(bits, alphaValue(rune(s.data[i]))*45+alphaValue(rune(s.data[i+1])), 11)
		}
		if i < len(s.data) {
			bits = appendBits(bits, alphaValue(rune(s.data[i])), 6)
		}
	case Kanji:
		for _, r := range s.data {
			v, _ := KanjiValue(r)
			bits = appendBits(bits, v, 13)
		}
	}
	return bits
}

// Bits materializes complete unpadded bits as bytes containing only 0 or 1.
func (s Segment) Bits(version int) ([]byte, error) {
	n, e := s.BitLength(version)
	if e != nil {
		return nil, e
	}
	if e = s.checkMaterialization(version, n); e != nil {
		return nil, e
	}
	return s.appendBits(make([]byte, 0, n), version), nil
}

// SegmentsBits validates the whole sequence and all bounds before allocating.
func SegmentsBits(segments []Segment, version int) ([]byte, error) {
	n, e := BitLength(segments, version)
	if e != nil {
		return nil, e
	}
	if n > MaxSingleSymbolDataBits {
		return nil, errCode(DataTooLong, "Segments exceed maximum single-symbol data capacity")
	}
	for _, s := range segments {
		length, _ := s.BitLength(version)
		if e := s.checkMaterialization(version, length); e != nil {
			return nil, e
		}
	}
	bits := make([]byte, 0, n)
	for _, s := range segments {
		bits = s.appendBits(bits, version)
	}
	return bits, nil
}

var dataModes = [...]Mode{Numeric, Alphanumeric, Kanji, Byte}

type optimizationState struct{ key, cost, segments, mode, remainder, previous int }

func initialStates() []optimizationState {
	return []optimizationState{{key: -1, mode: -1, previous: -1}}
}
func betterState(a, b optimizationState) bool {
	return a.cost < b.cost || a.cost == b.cost && a.segments < b.segments
}
func advanceStates(states []optimizationState, r rune, version int, allowKanji bool) []optimizationState {
	eligible := [4]bool{r >= '0' && r <= '9', alphaValue(r) >= 0, allowKanji && CanEncodeKanji(r), true}
	next := make([]optimizationState, 0, 7)
	for _, state := range states {
		for mode, dataMode := range dataModes {
			if !eligible[mode] {
				continue
			}
			same := state.mode == mode
			mod := 0
			if same {
				mod = state.remainder
			}
			payload := utf8.RuneLen(r) * 8
			remainder := 0
			switch mode {
			case 0:
				payload = 3
				if mod == 0 {
					payload = 4
				}
				remainder = (mod + 1) % 3
			case 1:
				payload = 5
				if mod == 0 {
					payload = 6
				}
				remainder = (mod + 1) % 2
			case 2:
				payload = 13
			}
			candidate := optimizationState{key: mode*3 + remainder, cost: state.cost + payload, segments: state.segments, mode: mode, remainder: remainder, previous: state.key}
			if !same {
				width, _ := CountBits(dataMode, version)
				candidate.cost += 4 + width
				candidate.segments++
			}
			found := false
			for i := range next {
				if next[i].key == candidate.key {
					if betterState(candidate, next[i]) {
						next[i] = candidate
					}
					found = true
					break
				}
			}
			if !found {
				next = append(next, candidate)
			}
		}
	}
	return next
}
func bestState(states []optimizationState) optimizationState {
	best := states[0]
	for _, s := range states[1:] {
		if betterState(s, best) {
			best = s
		}
	}
	return best
}

// CreateSegments chooses deterministic minimum-bit data segments for one symbol.
func CreateSegments(text string, version int, mode Mode, optimize bool) ([]Segment, error) {
	return CreateSegmentsWithKanji(text, version, mode, optimize, true)
}

// CreateSegmentsWithKanji can exclude automatic Kanji selection when ECI is used.
// An explicitly selected Kanji mode remains available regardless of allowKanji.
func CreateSegmentsWithKanji(text string, version int, mode Mode, optimize, allowKanji bool) ([]Segment, error) {
	if e := validateVersion(version); e != nil {
		return nil, e
	}
	if mode != Auto && mode != Numeric && mode != Alphanumeric && mode != Byte && mode != Kanji {
		return nil, errCode(InvalidMode, "Unsupported input mode")
	}
	count, e := validateText(text)
	if e != nil {
		return nil, e
	}
	if mode != Auto {
		s, e := textSegment(mode, text)
		if e != nil {
			return nil, e
		}
		return []Segment{s}, nil
	}
	if !optimize || count == 0 {
		selected := Byte
		if count > 0 {
			numeric, alpha, kanji := true, true, allowKanji
			for _, r := range text {
				numeric = numeric && r >= '0' && r <= '9'
				alpha = alpha && alphaValue(r) >= 0
				kanji = kanji && CanEncodeKanji(r)
			}
			if numeric {
				selected = Numeric
			} else if alpha {
				selected = Alphanumeric
			} else if kanji {
				selected = Kanji
			}
		}
		s, e := textSegment(selected, text)
		if e != nil {
			return nil, e
		}
		return []Segment{s}, nil
	}
	if count > MaxSingleSymbolCharacters {
		return nil, errCode(DataTooLong, "Text exceeds maximum single-symbol character capacity")
	}
	layers := make([][]optimizationState, 1, count+1)
	layers[0] = initialStates()
	offsets := make([]int, 0, count+1)
	for offset, r := range text {
		offsets = append(offsets, offset)
		layers = append(layers, advanceStates(layers[len(layers)-1], r, version, allowKanji))
	}
	offsets = append(offsets, len(text))
	key := bestState(layers[count]).key
	assignments := make([]int, count)
	for i := count; i >= 1; i-- {
		found := false
		for _, state := range layers[i] {
			if state.key == key {
				assignments[i-1] = state.mode
				key = state.previous
				found = true
				break
			}
		}
		if !found {
			return nil, errCode(InvalidInput, "Missing segmentation state")
		}
	}
	result := make([]Segment, 0, bestState(layers[count]).segments)
	start := 0
	for end := 1; end <= count; end++ {
		if end == count || assignments[end] != assignments[start] {
			s, e := textSegment(dataModes[assignments[start]], text[offsets[start]:offsets[end]])
			if e != nil {
				return nil, e
			}
			result = append(result, s)
			start = end
		}
	}
	return result, nil
}

// OptimizationTracker computes exact optimal prefix bit costs in constant memory.
// It intentionally supports multi-symbol planning; materialization enforces count widths.
// The zero value is invalid. A tracker is not safe for concurrent mutation.
type OptimizationTracker struct {
	version    int
	allowKanji bool
	characters int
	states     []optimizationState
}

func NewOptimizationTracker(version int, allowKanji bool) (*OptimizationTracker, error) {
	if e := validateVersion(version); e != nil {
		return nil, e
	}
	return &OptimizationTracker{version: version, allowKanji: allowKanji, states: initialStates()}, nil
}
func (t *OptimizationTracker) Append(r rune) (int, error) {
	if t == nil || len(t.states) == 0 {
		return 0, errCode(InvalidInput, "Uninitialized optimization tracker")
	}
	if !utf8.ValidRune(r) {
		return 0, errCode(InvalidInput, "Tracker requires a Unicode scalar")
	}
	if t.characters >= MaxPayloadUnits {
		return 0, errCode(DataTooLong, "Tracker exceeds the payload resource limit")
	}
	next := advanceStates(t.states, r, t.version, t.allowKanji)
	t.states = next
	t.characters++
	return bestState(next).cost, nil
}
