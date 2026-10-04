package specqr

import (
	"fmt"
	"sort"
	"unicode/utf8"
)

// StructuredAppendOptions configures greedy splitting into 2..16 equal-version symbols.
type StructuredAppendOptions struct {
	Options                           Options
	MaxSymbols                        int
	FullSplitUnits, SymbolDiagnostics bool
}

// SAResult is an immutable generated Structured Append set.
type SAResult struct {
	symbols                                []*QRCode
	total, parity, inputLength, byteLength int
	diagnostics                            map[string]any
}

func (s *SAResult) Symbols() []*QRCode          { return append([]*QRCode(nil), s.symbols...) }
func (s *SAResult) Total() int                  { return s.total }
func (s *SAResult) Parity() int                 { return s.parity }
func (s *SAResult) InputLength() int            { return s.inputLength }
func (s *SAResult) ByteLength() int             { return s.byteLength }
func (s *SAResult) Diagnostics() map[string]any { return cloneMap(s.diagnostics) }
func xorBytes(b []byte) int {
	n := 0
	for _, v := range b {
		n ^= int(v)
	}
	return n
}

// CalculateStructuredAppendParity XORs original UTF-8 bytes, never Shift JIS bytes.
func CalculateStructuredAppendParity(text string) (int, error) {
	if _, e := validateText(text); e != nil {
		return 0, e
	}
	n := 0
	for i := range len(text) {
		n ^= int(text[i])
	}
	return n, nil
}
func CalculateStructuredAppendBytesParity(data []byte) (int, error) {
	if len(data) > MaxPayloadUnits {
		return 0, errCode(DataTooLong, "payload exceeds resource budget")
	}
	return xorBytes(data), nil
}
func CalculateStructuredAppendSegmentsParity(ss []Segment) (int, error) {
	if e := validateSASegments(ss); e != nil {
		return 0, e
	}
	n := 0
	for _, s := range ss {
		n ^= xorBytes(s.LogicalBytes())
	}
	return n, nil
}
func validateSASegments(ss []Segment) error {
	if len(ss) == 0 {
		return errCode(InvalidInput, "Structured Append requires nonempty segments")
	}
	if e := ValidateSegments(ss); e != nil {
		return e
	}
	for _, s := range ss {
		if s.Mode() == FNC1First {
			return errCode(InvalidGS1, "Structured Append cannot combine with GS1")
		}
		if s.IsControl() {
			return errCode(InvalidMode, "Structured Append cannot combine with controls")
		}
		if s.Count() == 0 {
			return errCode(InvalidInput, "Structured Append data segments must be nonempty")
		}
	}
	return nil
}
func normalizeSA(opts StructuredAppendOptions, manual bool) (StructuredAppendOptions, error) {
	o, e := normalizeOptions(opts.Options)
	if e != nil {
		return opts, e
	}
	if opts.MaxSymbols == 0 {
		opts.MaxSymbols = 16
	}
	if opts.MaxSymbols < 2 || opts.MaxSymbols > 16 {
		return opts, errCode(InvalidMode, "max symbols must be 2..16")
	}
	if o.GS1 {
		return opts, errCode(InvalidGS1, "Structured Append cannot combine with GS1")
	}
	if o.ECI != nil || o.FNC1Second != nil || o.StructuredAppend != nil || o.BoostECC {
		return opts, errCode(InvalidMode, "Structured Append owns its header and does not combine with ECI, FNC1 or ECC boosting")
	}
	if manual && (o.Mode != Auto || o.DisableOptimization) {
		return opts, errCode(InvalidMode, "manual Structured Append preserves caller segment modes")
	}
	opts.Options = o
	return opts, nil
}

type saDescriptor struct {
	s                                           Segment
	source, start, count, byteStart, byteLength int
	offsets                                     []int
}
type saSource struct {
	text                                    string
	raw                                     []byte
	binary, manual                          bool
	length, inputLength, byteLength, parity int
	offsets                                 []int
	descriptors                             []saDescriptor
}

func textOffsets(s string) []int {
	v := make([]int, 0, utf8.RuneCountInString(s)+1)
	for i := range s {
		v = append(v, i)
	}
	return append(v, len(s))
}
func saCapacity(o Options, v int) int { n, _ := DataCodewordCount(v, o.ECC); return n * 8 }
func saMaxVersion(o Options) int {
	if o.Version != 0 {
		return o.Version
	}
	return o.MaxVersion
}
func saUnitBudget(o StructuredAppendOptions) int {
	return o.MaxSymbols * ((saCapacity(o.Options, saMaxVersion(o.Options)) - 20) * 3 / 10)
}
func newSASource(text string, data []byte, binary bool, o StructuredAppendOptions) (*saSource, error) {
	s := &saSource{text: text, binary: binary}
	if binary {
		s.length = len(data)
	} else {
		if _, e := validateText(text); e != nil {
			return nil, e
		}
		s.length = utf8.RuneCountInString(text)
	}
	if s.length == 0 {
		return nil, errCode(InvalidInput, "Structured Append requires nonempty input")
	}
	if s.length > saUnitBudget(o) || s.length > MaxPayloadUnits {
		return nil, errCode(DataTooLong, "input exceeds selected Structured Append capacity")
	}
	s.inputLength = s.length
	if binary {
		if o.Options.Mode != Auto && o.Options.Mode != Byte {
			return nil, errCode(InvalidMode, "binary input requires byte mode")
		}
		s.raw = append([]byte(nil), data...)
		s.byteLength = len(data)
		s.parity = xorBytes(data)
	} else {
		s.byteLength = len(text)
		s.parity, _ = CalculateStructuredAppendParity(text)
		s.offsets = textOffsets(text)
		if o.Options.Mode != Auto {
			if _, e := CreateSegments(text, saMaxVersion(o.Options), o.Options.Mode, false); e != nil {
				return nil, e
			}
		}
	}
	return s, nil
}
func newSAManual(ss []Segment, o StructuredAppendOptions) (*saSource, error) {
	if e := validateSASegments(ss); e != nil {
		return nil, e
	}
	s := &saSource{manual: true, inputLength: len(ss)}
	units := 0
	for i, v := range ss {
		n := v.CharacterCount()
		if v.IsBinary() {
			n = len(v.Data())
		}
		units += n
		if units > saUnitBudget(o) {
			return nil, errCode(DataTooLong, "manual input exceeds Structured Append capacity")
		}
		d := saDescriptor{s: v, source: i, start: s.length, count: 1, byteStart: s.byteLength, byteLength: len(v.LogicalBytes())}
		if v.Mode() == Byte {
			d.count = n
			if !v.IsBinary() {
				d.offsets = textOffsets(v.Text())
			}
		}
		s.descriptors = append(s.descriptors, d)
		s.length += d.count
		s.byteLength += d.byteLength
		s.parity ^= xorBytes(v.LogicalBytes())
	}
	return s, nil
}
func (d saDescriptor) bytes(start, length int) (int, int) {
	if d.s.Mode() != Byte {
		return d.byteStart, d.byteLength
	}
	if d.offsets == nil {
		return d.byteStart + start, length
	}
	a, b := d.offsets[start], d.offsets[start+length]
	return d.byteStart + a, b - a
}
func (s *saSource) ranges(start, length int, fn func(saDescriptor, int, int) bool) {
	end := start + length
	for _, d := range s.descriptors {
		if d.start+d.count <= start {
			continue
		}
		if d.start >= end {
			break
		}
		a := max(start, d.start)
		if !fn(d, a-d.start, min(end, d.start+d.count)-a) {
			return
		}
	}
}
func saSegmentBits(mode Mode, chars, bytes, v int) int {
	count := chars
	if mode == Byte {
		count = bytes
	}
	width, e := CountBits(mode, v)
	if e != nil || count >= 1<<width {
		return 1 << 30
	}
	payload, e := PayloadBitLength(mode, count)
	if e != nil {
		return 1 << 30
	}
	return 4 + width + payload
}
func (s *saSource) bits(start, length int, o Options, v int) int {
	if s.manual {
		b := 20
		s.ranges(start, length, func(d saDescriptor, a, n int) bool {
			_, bytes := d.bytes(a, n)
			chars := d.s.CharacterCount()
			if d.s.Mode() == Byte {
				chars = n
			}
			b += saSegmentBits(d.s.Mode(), chars, bytes, v)
			return b <= saCapacity(o, v)
		})
		return b
	}
	if s.binary {
		return 20 + saSegmentBits(Byte, length, length, v)
	}
	text := s.text[s.offsets[start]:s.offsets[start+length]]
	if o.Mode != Auto {
		return 20 + saSegmentBits(o.Mode, length, len(text), v)
	}
	if !o.DisableOptimization {
		t, _ := NewOptimizationTracker(v, true)
		b := 0
		for _, r := range text {
			b, _ = t.Append(r)
			if b+20 > saCapacity(o, v) {
				break
			}
		}
		return b + 20
	}
	segments, e := CreateSegments(text, v, Auto, false)
	if e != nil {
		return 1 << 30
	}
	for _, seg := range segments {
		width, _ := CountBits(seg.Mode(), v)
		if seg.Count() >= 1<<width {
			return 1 << 30
		}
	}
	b, _ := BitLength(segments, v)
	return b + 20
}
func (s *saSource) largest(start, maximum int, o Options, v int) int {
	if !s.manual && !s.binary && o.Mode == Auto && !o.DisableOptimization {
		t, _ := NewOptimizationTracker(v, true)
		index := 0
		for _, r := range s.text[s.offsets[start]:s.offsets[start+maximum]] {
			b, _ := t.Append(r)
			if b+20 > saCapacity(o, v) {
				return index
			}
			index++
		}
		return maximum
	}
	low, high, best := 1, maximum, 0
	for low <= high {
		n := (low + high) / 2
		if s.bits(start, n, o, v) <= saCapacity(o, v) {
			best = n
			low = n + 1
		} else {
			high = n - 1
		}
	}
	return best
}
func (s *saSource) chunk(start, length int) ([]Segment, map[string]any, error) {
	if !s.manual {
		a, b := start, length
		if !s.binary {
			a = s.offsets[start]
			b = s.offsets[start+length] - a
		}
		return nil, map[string]any{"inputStart": start, "inputLength": length, "byteStart": a, "byteLength": b}, nil
	}
	ss := []Segment{}
	first, last, byteStart, bytes := -1, 0, 0, 0
	var err error
	s.ranges(start, length, func(d saDescriptor, a, n int) bool {
		offset, size := d.bytes(a, n)
		if first < 0 {
			first = d.source
			byteStart = offset
		}
		last = d.source + 1
		bytes += size
		seg := d.s
		if seg.Mode() == Byte {
			local := offset - d.byteStart
			if seg.IsBinary() {
				data := seg.Data()
				seg, err = BytesSegment(data[local : local+size])
			} else {
				seg, err = UTF8Segment(seg.Text()[local : local+size])
			}
		}
		ss = append(ss, seg)
		return err == nil
	})
	return ss, map[string]any{"sourceSegmentStart": first, "sourceSegmentEnd": last, "splitUnitStart": start, "splitUnitLength": length, "byteStart": byteStart, "byteLength": bytes}, err
}
func (s *saSource) fullDetail() []any {
	out := make([]any, 0, s.length)
	for _, d := range s.descriptors {
		for i := 0; i < d.count; i++ {
			a, b := d.bytes(i, 1)
			unitStart, unitLength := 0, d.s.CharacterCount()
			if d.s.Mode() == Byte {
				unitStart = i
				unitLength = 1
			}
			out = append(out, map[string]any{"sourceSegmentIndex": d.source, "mode": string(d.s.Mode()), "unitStart": unitStart, "unitLength": unitLength, "byteStart": a, "byteLength": b})
		}
	}
	return out
}
func generateSA(s *saSource, opts StructuredAppendOptions) (*SAResult, error) {
	o := opts.Options
	first, last := o.MinVersion, o.MaxVersion
	if o.Version != 0 {
		first = o.Version
		last = o.Version
	}
	version := 0
	var chosen [][2]int
	sawTooLong := false
	for v := first; v <= last; v++ {
		if s.bits(0, s.length, o, v) <= saCapacity(o, v) {
			continue
		}
		ranges := [][2]int{}
		start := 0
		for start < s.length && len(ranges) < opts.MaxSymbols {
			maximum := s.length - start
			if len(ranges) == 0 {
				maximum--
			}
			n := s.largest(start, maximum, o, v)
			if n == 0 {
				break
			}
			ranges = append(ranges, [2]int{start, n})
			start += n
		}
		if start == s.length && len(ranges) >= 2 {
			version = v
			chosen = ranges
			break
		}
		sawTooLong = true
	}
	if version == 0 {
		if sawTooLong {
			return nil, errCode(DataTooLong, "input cannot split within requested versions and symbol limit")
		}
		return nil, errCode(InvalidInput, "input fits in one symbol; use Generate or a low-level header")
	}
	total := len(chosen)
	result := &SAResult{total: total, parity: s.parity, inputLength: s.inputLength, byteLength: s.byteLength}
	descriptions := []any{}
	for i, r := range chosen {
		start, length := r[0], r[1]
		segments, offsets, e := s.chunk(start, length)
		if e != nil {
			return nil, e
		}
		fixed := o
		fixed.Version = version
		fixed.MinVersion = version
		fixed.MaxVersion = version
		header, _ := StructuredAppendSegment(i+1, total, s.parity)
		fixed.StructuredAppend = &header
		var q *QRCode
		if s.manual {
			q, e = GenerateSegments(segments, fixed)
		} else if s.binary {
			q, e = GenerateBytes(s.raw[start:start+length], fixed)
		} else {
			q, e = Generate(s.text[s.offsets[start]:s.offsets[start+length]], fixed)
		}
		if e != nil {
			return nil, e
		}
		result.symbols = append(result.symbols, q)
		b, _ := BitLength(q.segments, version)
		d := map[string]any{"index": i + 1, "total": total, "parity": s.parity, "sequenceIndex": i, "sequenceTotal": total - 1, "sequenceIndicator": i<<4 | (total - 1), "version": version, "errorCorrectionLevel": string(q.ecc), "dataBitLength": b, "capacityBits": saCapacity(o, version), "remainingBits": saCapacity(o, version) - b, "maskPattern": q.mask}
		for k, v := range offsets {
			d[k] = v
		}
		descriptions = append(descriptions, d)
	}
	warnings := []any{}
	if total == opts.MaxSymbols {
		warnings = append(warnings, map[string]any{"code": "STRUCTURED_APPEND_MAX_SYMBOLS_NEAR_LIMIT", "severity": "info", "message": "The generated Structured Append set uses the configured maximum number of symbols.", "details": map[string]any{"total": total, "maxSymbols": opts.MaxSymbols}})
	}
	if opts.SymbolDiagnostics {
		warnings = append(warnings, map[string]any{"code": "STRUCTURED_APPEND_DECODER_SUPPORT_VARIES", "severity": "info", "message": "Decoder APIs vary in how they expose Structured Append set metadata.", "details": map[string]any{"total": total}})
	}
	selection, label, strategy := "auto-minimum", "payload", "greedy-largest-fitting"
	if s.manual {
		label = "manual segments"
		strategy = "segment-boundary-byte-chunk"
	}
	reason := fmt.Sprintf("Version %d is the smallest version in %d..%d that can split the %s into %d Structured Append symbols at error correction %s.", version, o.MinVersion, o.MaxVersion, label, total, o.ECC)
	if o.Version != 0 {
		selection = "fixed"
		reason = fmt.Sprintf("Version %d was requested explicitly.", version)
	}
	d := map[string]any{"version": version, "errorCorrectionLevel": string(o.ECC), "versionSelection": selection, "versionSelectionReason": reason, "total": total, "parity": s.parity, "byteLength": s.byteLength, "inputLength": s.inputLength, "maxSymbols": opts.MaxSymbols, "splitStrategy": strategy, "symbols": descriptions, "warnings": warnings}
	if s.manual {
		d["segmentCount"] = s.inputLength
		d["splitUnitCount"] = s.length
		d["splitUnitsDetail"] = "summary"
		if opts.FullSplitUnits {
			d["splitUnitsDetail"] = "full"
			d["splitUnits"] = s.fullDetail()
		}
	}
	result.diagnostics = d
	return result, nil
}
func GenerateStructuredAppend(text string, options StructuredAppendOptions) (*SAResult, error) {
	o, e := normalizeSA(options, false)
	if e != nil {
		return nil, e
	}
	s, e := newSASource(text, nil, false, o)
	if e != nil {
		return nil, e
	}
	return generateSA(s, o)
}
func GenerateBytesStructuredAppend(data []byte, options StructuredAppendOptions) (*SAResult, error) {
	o, e := normalizeSA(options, false)
	if e != nil {
		return nil, e
	}
	s, e := newSASource("", data, true, o)
	if e != nil {
		return nil, e
	}
	return generateSA(s, o)
}
func GenerateSegmentsStructuredAppend(segments []Segment, options StructuredAppendOptions) (*SAResult, error) {
	o, e := normalizeSA(options, true)
	if e != nil {
		return nil, e
	}
	s, e := newSAManual(segments, o)
	if e != nil {
		return nil, e
	}
	return generateSA(s, o)
}

// SAPart contains decoded 1-based metadata and exactly one payload kind.
// Binary true uses Bytes (including an empty slice); otherwise Text must be nonnil.
type SAPart struct {
	Index, Total, Parity int
	Text                 *string
	Bytes                []byte
	Binary               bool
}
type MergeResult struct {
	text          string
	data          []byte
	binary        bool
	total, parity int
	parts         []SAPart
	metadata      []map[string]any
	diagnostics   map[string]any
}

func (m *MergeResult) Text() string                { return m.text }
func (m *MergeResult) Bytes() []byte               { return append([]byte(nil), m.data...) }
func (m *MergeResult) Binary() bool                { return m.binary }
func (m *MergeResult) Total() int                  { return m.total }
func (m *MergeResult) Parity() int                 { return m.parity }
func (m *MergeResult) Diagnostics() map[string]any { return cloneMap(m.diagnostics) }
func copyPart(p SAPart) SAPart {
	if p.Text != nil {
		v := *p.Text
		p.Text = &v
	}
	p.Bytes = append([]byte(nil), p.Bytes...)
	return p
}
func (m *MergeResult) Parts() []SAPart {
	a := make([]SAPart, len(m.parts))
	for i, p := range m.parts {
		a[i] = copyPart(p)
	}
	return a
}
func (m *MergeResult) PartMetadata() []map[string]any {
	a := make([]map[string]any, len(m.metadata))
	for i, p := range m.metadata {
		a[i] = cloneMap(p)
	}
	return a
}

// MergeStructuredAppendParts verifies completeness, ordering, type and XOR parity.
// Parity detects some corruption; it does not authenticate a message.
func MergeStructuredAppendParts(parts []SAPart) (*MergeResult, error) {
	if len(parts) < 1 || len(parts) > 16 {
		return nil, errCode(InvalidInput, "Structured Append requires 1..16 decoded parts")
	}
	total, parity, binary := parts[0].Total, parts[0].Parity, parts[0].Binary
	seen := map[int]bool{}
	bytes, units, actual := 0, 0, 0
	ordered := make([]SAPart, 0, len(parts))
	for _, p := range parts {
		if p.Index < 1 || p.Total < 2 || p.Total > 16 || p.Index > p.Total || p.Parity < 0 || p.Parity > 255 {
			return nil, errCode(InvalidInput, "invalid Structured Append metadata")
		}
		if p.Total != total || p.Parity != parity || p.Binary != binary || seen[p.Index] {
			return nil, errCode(InvalidInput, "mismatched metadata, mixed payload types or duplicate index")
		}
		seen[p.Index] = true
		var b []byte
		if binary {
			if p.Text != nil {
				return nil, errCode(InvalidInput, "binary part cannot also have text")
			}
			b = p.Bytes
			units += len(b)
		} else {
			if p.Text == nil || p.Bytes != nil {
				return nil, errCode(InvalidInput, "text part requires text only")
			}
			if _, e := validateText(*p.Text); e != nil {
				return nil, e
			}
			units += utf8.RuneCountInString(*p.Text)
			b = []byte(*p.Text)
		}
		if units > MaxPayloadUnits {
			return nil, errCode(DataTooLong, "merged payload exceeds resource budget")
		}
		bytes += len(b)
		actual ^= xorBytes(b)
		ordered = append(ordered, copyPart(p))
	}
	if len(parts) != total {
		return nil, errCode(InvalidInput, "Structured Append parts are missing")
	}
	if actual != parity {
		return nil, errCode(InvalidInput, "Structured Append parity check failed")
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	kind := "string"
	if binary {
		kind = "binary"
	}
	result := &MergeResult{binary: binary, total: total, parity: parity, parts: ordered, data: make([]byte, 0, bytes)}
	for _, p := range ordered {
		b := p.Bytes
		if !binary {
			b = []byte(*p.Text)
		}
		result.data = append(result.data, b...)
		result.metadata = append(result.metadata, map[string]any{"index": p.Index, "total": total, "parity": parity, "dataType": kind, "byteLength": len(b)})
	}
	if !binary {
		result.text = string(result.data)
	}
	result.diagnostics = map[string]any{"partCount": total, "total": total, "parity": parity, "dataType": kind, "byteLength": bytes, "missing": []any{}, "duplicate": []any{}, "parityCheck": map[string]any{"expected": parity, "actual": actual, "matches": true}}
	return result, nil
}
