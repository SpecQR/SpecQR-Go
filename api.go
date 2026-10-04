package specqr

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Version is the prerelease edition identifier; this module has no release tag.
const Version = "0.1.0-rc.1"

// Options configures encoding. Its zero value chooses M, automatic version/mask,
// mixed-mode optimization and the default renderer. Use pointers for explicit zero controls.
type Options struct {
	ECC                             ECC
	Mode                            Mode
	Version, MinVersion, MaxVersion int
	Mask                            *int
	DisableOptimization, BoostECC   bool
	ECI                             *int
	GS1                             bool
	FNC1Second                      *string
	StructuredAppend                *Segment
	Render                          RenderOptions
	PrintDPI                        float64
}

func DefaultOptions() Options {
	return Options{ECC: M, Mode: Auto, MinVersion: 1, MaxVersion: 40, Render: DefaultRenderOptions()}
}
func normalizeOptions(o Options) (Options, error) {
	if o.ECC == "" {
		o.ECC = M
	}
	if o.Mode == "" {
		o.Mode = Auto
	}
	if o.MinVersion == 0 {
		o.MinVersion = 1
	}
	if o.MaxVersion == 0 {
		o.MaxVersion = 40
	}
	o.Render = normalizeRender(o.Render)
	if o.MinVersion < 1 || o.MinVersion > 40 || o.MaxVersion < 1 || o.MaxVersion > 40 || o.MinVersion > o.MaxVersion || o.Version < 0 || o.Version > 40 {
		return o, errCode(InvalidVersion, "versions must be 1..40 with ordered bounds")
	}
	if o.ECC != L && o.ECC != M && o.ECC != Q && o.ECC != H {
		return o, errCode(InvalidECC, "ECC must be L, M, Q or H")
	}
	if o.Mode != Auto && o.Mode != Numeric && o.Mode != Alphanumeric && o.Mode != Byte && o.Mode != Kanji {
		return o, errCode(InvalidMode, "high-level mode must be auto or a data mode")
	}
	if o.Mask != nil {
		if *o.Mask < 0 || *o.Mask > 7 {
			return o, errCode(InvalidInput, "mask must be 0..7")
		}
		v := *o.Mask
		o.Mask = &v
	}
	count := 0
	if o.ECI != nil {
		count++
		v := *o.ECI
		if _, e := ECISegment(v); e != nil {
			return o, e
		}
		o.ECI = &v
	}
	if o.GS1 {
		count++
	}
	if o.FNC1Second != nil {
		count++
		v := *o.FNC1Second
		if _, e := FNC1SecondSegment(v); e != nil {
			return o, e
		}
		o.FNC1Second = &v
	}
	if o.StructuredAppend != nil {
		count++
		v := *o.StructuredAppend
		if v.Mode() != StructuredAppend {
			return o, errCode(InvalidMode, "StructuredAppend must be a Structured Append header")
		}
		o.StructuredAppend = &v
	}
	if count > 1 {
		return o, errCode(InvalidMode, "ECI, GS1, FNC1 second and Structured Append cannot be combined")
	}
	if o.Render.Margin < 0 || o.Render.Scale < 1 {
		return o, errCode(InvalidInput, "margin must be nonnegative and scale positive")
	}
	if len(o.Render.Foreground) > SVGCharacterBudget/12 || len(o.Render.Background) > SVGCharacterBudget/12 {
		return o, errCode(InvalidColor, "color exceeds resource budget")
	}
	if !utf8.ValidString(o.Render.Foreground) || !utf8.ValidString(o.Render.Background) {
		return o, errCode(InvalidColor, "color must be valid UTF-8")
	}
	if math.IsNaN(o.PrintDPI) || math.IsInf(o.PrintDPI, 0) || o.PrintDPI < 0 {
		return o, errCode(InvalidInput, "print DPI must be finite and positive")
	}
	if o.PrintDPI > 0 {
		extent := (177 + 2*float64(o.Render.Margin)) * float64(o.Render.Scale) / o.PrintDPI * 25.4
		if math.IsInf(extent, 0) || math.IsNaN(extent) {
			return o, errCode(InvalidInput, "print geometry must be finite")
		}
	}
	return o, nil
}

// Validate checks option bounds without generating a symbol.
func (o Options) Validate() error { _, e := normalizeOptions(o); return e }
func copyOptions(o Options) Options {
	if o.Mask != nil {
		v := *o.Mask
		o.Mask = &v
	}
	if o.ECI != nil {
		v := *o.ECI
		o.ECI = &v
	}
	if o.FNC1Second != nil {
		v := *o.FNC1Second
		o.FNC1Second = &v
	}
	if o.StructuredAppend != nil {
		v := *o.StructuredAppend
		o.StructuredAppend = &v
	}
	return o
}
func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, v := range x {
			m[k] = cloneValue(v)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, v := range x {
			s[i] = cloneValue(v)
		}
		return s
	case []string:
		return append([]string(nil), x...)
	case []int:
		return append([]int(nil), x...)
	case []byte:
		return append([]byte(nil), x...)
	default:
		return x
	}
}
func cloneMap(m map[string]any) map[string]any { return cloneValue(m).(map[string]any) }

// Plan describes capacity without generating ECC, matrix, or rendered output.
type Plan struct {
	ok                         bool
	version, capacityVersion   int
	ecc, requestedECC          ECC
	requiredBits, capacityBits int
	segments                   []Segment
	diagnostics                map[string]any
}

func (p *Plan) OK() bool             { return p.ok }
func (p *Plan) Version() int         { return p.version }
func (p *Plan) CapacityVersion() int { return p.capacityVersion }
func (p *Plan) ECC() ECC             { return p.ecc }
func (p *Plan) RequestedECC() ECC    { return p.requestedECC }
func (p *Plan) BoostedECC() bool     { return p.ecc != p.requestedECC }
func (p *Plan) RequiredBits() int    { return p.requiredBits }
func (p *Plan) DataBitLength() int   { return p.requiredBits }
func (p *Plan) CapacityBits() int    { return p.capacityBits }
func (p *Plan) RemainingBits() int   { return p.capacityBits - p.requiredBits }
func (p *Plan) OverflowBits() int    { return max(0, p.requiredBits-p.capacityBits) }
func (p *Plan) CapacityUtilization() float64 {
	return float64(p.requiredBits) / float64(p.capacityBits)
}
func (p *Plan) Segments() []Segment         { return append([]Segment(nil), p.segments...) }
func (p *Plan) Diagnostics() map[string]any { return cloneMap(p.diagnostics) }

// QRCode is an immutable symbol safe for concurrent readers. Returned slices and
// maps are detached copies. Do not mutate caller inputs concurrently with a call.
type QRCode struct {
	matrix          [][]bool
	version, mask   int
	ecc             ECC
	data, codewords []byte
	segments        []Segment
	diagnostics     map[string]any
	options         Options
}

func (q *QRCode) Matrix() [][]bool      { return copyMatrix(q.matrix) }
func (q *QRCode) Version() int          { return q.version }
func (q *QRCode) Size() int             { return len(q.matrix) }
func (q *QRCode) Mask() int             { return q.mask }
func (q *QRCode) ECC() ECC              { return q.ecc }
func (q *QRCode) DataCodewords() []byte { return append([]byte(nil), q.data...) }
func (q *QRCode) Codewords() []byte     { return append([]byte(nil), q.codewords...) }
func (q *QRCode) ErrorCorrectionCodewords() []byte {
	return append([]byte(nil), q.codewords[len(q.data):]...)
}
func (q *QRCode) Segments() []Segment         { return append([]Segment(nil), q.segments...) }
func (q *QRCode) Diagnostics() map[string]any { return cloneMap(q.diagnostics) }
func (q *QRCode) Options() Options            { return copyOptions(q.options) }
func (q *QRCode) Module(x, y int) (bool, error) {
	if x < 0 || y < 0 || y >= len(q.matrix) || x >= len(q.matrix) {
		return false, errCode(InvalidInput, "module coordinate out of bounds")
	}
	return q.matrix[y][x], nil
}
func (q *QRCode) ToSVG() (string, error)        { return ToSVG(q.matrix, q.options.Render) }
func (q *QRCode) ToPNG() ([]byte, error)        { return ToPNG(q.matrix, q.options.Render) }
func (q *QRCode) ToPixels() (*Pixels, error)    { return ToPixels(q.matrix, q.options.Render) }
func (q *QRCode) ToSVGDataURL() (string, error) { return ToSVGDataURL(q.matrix, q.options.Render) }
func (q *QRCode) ToPNGDataURL() (string, error) { return ToPNGDataURL(q.matrix, q.options.Render) }

// Generate encodes valid UTF-8 text. Unicode QR support is independent of URL-host validation.
func Generate(text string, options Options) (*QRCode, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	p, e := Estimate(text, o)
	if e != nil {
		return nil, e
	}
	return build(p, o)
}

// GenerateBytes encodes arbitrary raw bytes without UTF-8 replacement or reinterpretation.
func GenerateBytes(data []byte, options Options) (*QRCode, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	p, e := EstimateBytes(data, o)
	if e != nil {
		return nil, e
	}
	return build(p, o)
}

// GenerateSegments preserves caller segment boundaries and low-level control escaping.
func GenerateSegments(segments []Segment, options Options) (*QRCode, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	p, e := AnalyzeSegments(segments, o)
	if e != nil {
		return nil, e
	}
	return build(p, o)
}
func addControls(segments []Segment, o Options) ([]Segment, error) {
	var h Segment
	has := true
	var e error
	switch {
	case o.ECI != nil:
		h, e = ECISegment(*o.ECI)
	case o.GS1:
		h, e = FNC1Segment()
	case o.FNC1Second != nil:
		h, e = FNC1SecondSegment(*o.FNC1Second)
	case o.StructuredAppend != nil:
		h = *o.StructuredAppend
	default:
		has = false
	}
	if e != nil {
		return nil, e
	}
	out := make([]Segment, 0, len(segments)+1)
	if has {
		out = append(out, h)
	}
	out = append(out, segments...)
	if e := ValidateSegments(out); e != nil {
		return nil, e
	}
	return out, nil
}

// Estimate plans text without allocating a matrix.
func Estimate(text string, options Options) (*Plan, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	if !utf8.ValidString(text) {
		return nil, errCode(InvalidInput, "text must be valid UTF-8")
	}
	if len(text) > 4_000_000 || utf8.RuneCountInString(text) > 1_000_000 {
		return nil, errCode(DataTooLong, "text exceeds resource budget")
	}
	mode := o.Mode
	var gs1 any
	if o.GS1 {
		r, e := GS1ParseElementString(text)
		if e != nil {
			return nil, e
		}
		ais := make([]any, len(r.Elements))
		for i, v := range r.Elements {
			ais[i] = v.AI
		}
		gs1 = map[string]any{"enabled": true, "elementCount": len(r.Elements), "ais": ais, "hasSeparators": strings.ContainsRune(text, 29)}
	}
	if (o.GS1 || o.FNC1Second != nil) && strings.Contains(text, "%") {
		if mode == Alphanumeric {
			return nil, errCode(InvalidMode, "literal percent in high-level FNC1 requires byte mode or escaped manual segments")
		}
		if mode == Auto {
			mode = Byte
		}
	}
	cache := map[int][]Segment{}
	return selectPlan(func(v int) ([]Segment, error) {
		group := 0
		if v >= 27 {
			group = 2
		} else if v >= 10 {
			group = 1
		}
		if s, ok := cache[group]; ok {
			return s, nil
		}
		s, e := CreateSegmentsWithKanji(text, v, mode, !o.DisableOptimization && utf8.RuneCountInString(text) <= 7089, o.ECI == nil)
		if e != nil {
			return nil, e
		}
		s, e = addControls(s, o)
		if e == nil {
			cache[group] = s
		}
		return s, e
	}, o, gs1)
}
func EstimateBytes(data []byte, options Options) (*Plan, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	if o.GS1 {
		return nil, errCode(InvalidGS1, "high-level GS1 requires text element string")
	}
	if o.Mode != Auto && o.Mode != Byte {
		return nil, errCode(InvalidMode, "binary input requires byte mode")
	}
	s, e := BytesSegment(data)
	if e != nil {
		return nil, e
	}
	ss, e := addControls([]Segment{s}, o)
	if e != nil {
		return nil, e
	}
	return selectPlan(func(int) ([]Segment, error) { return ss, nil }, o, nil)
}
func AnalyzeSegments(segments []Segment, options Options) (*Plan, error) {
	o, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	ss, e := addControls(segments, o)
	if e != nil {
		return nil, e
	}
	return selectPlan(func(int) ([]Segment, error) { return ss, nil }, o, nil)
}
func selectPlan(factory func(int) ([]Segment, error), o Options, gs1 any) (*Plan, error) {
	first, last := o.MinVersion, o.MaxVersion
	if o.Version != 0 {
		first = o.Version
		last = o.Version
	}
	p := &Plan{requestedECC: o.ECC, ecc: o.ECC}
	for v := first; v <= last; v++ {
		ss, e := factory(v)
		if e != nil {
			return nil, e
		}
		bits, e := BitLength(ss, v)
		if e != nil {
			return nil, e
		}
		data, e := DataCodewordCount(v, o.ECC)
		if e != nil {
			return nil, e
		}
		p.capacityVersion = v
		p.segments = ss
		p.requiredBits = bits
		p.ok = bits <= data*8
		if p.ok {
			if o.BoostECC {
				found := false
				for _, ecc := range []ECC{L, M, Q, H} {
					if ecc == o.ECC {
						found = true
					}
					if found {
						n, _ := DataCodewordCount(v, ecc)
						if bits <= n*8 {
							p.ecc = ecc
						}
					}
				}
			}
			break
		}
	}
	n, _ := DataCodewordCount(p.capacityVersion, p.ecc)
	p.capacityBits = n * 8
	if p.ok || o.Version != 0 {
		p.version = p.capacityVersion
	}
	p.diagnostics = makeDiagnostics(p.segments, p.capacityVersion, p.ecc, p.requiredBits, o, true, p.ok)
	if gs1 != nil {
		p.diagnostics["gs1Validation"] = gs1
	}
	return p, nil
}
func build(p *Plan, o Options) (*QRCode, error) {
	if !p.ok {
		return nil, errCode(DataTooLong, fmt.Sprintf("input requires %d bits; version %d-%s holds %d", p.requiredBits, p.capacityVersion, p.ecc, p.capacityBits))
	}
	bits, e := SegmentsBits(p.segments, p.capacityVersion)
	if e != nil {
		return nil, e
	}
	data, e := PadDataBits(bits, p.capacityVersion, p.ecc)
	if e != nil {
		return nil, e
	}
	cw, e := InterleaveCodewords(data, p.capacityVersion, p.ecc)
	if e != nil {
		return nil, e
	}
	m, e := BuildMatrix(cw.Codewords, p.capacityVersion, p.ecc, o.Mask)
	if e != nil {
		return nil, e
	}
	d := makeDiagnostics(p.segments, p.capacityVersion, p.ecc, p.requiredBits, o, false, true)
	d["gs1Validation"] = p.diagnostics["gs1Validation"]
	d["maskPattern"] = m.MaskPattern
	d["maskPenalty"] = m.Penalty
	ps := make([]any, len(m.MaskPenalties))
	for i, v := range m.MaskPenalties {
		ps[i] = map[string]any{"maskPattern": v.MaskPattern, "penalty": v.Penalty}
	}
	d["maskPenalties"] = ps
	reason := "Lowest penalty; first mask wins ties."
	if o.Mask != nil {
		reason = "Explicit mask requested."
	}
	d["maskSelectionReason"] = reason
	d["dataCodewords"] = len(data)
	d["errorCorrectionCodewords"] = len(cw.Codewords) - len(data)
	d["totalCodewords"] = len(cw.Codewords)
	return &QRCode{m.Matrix, p.capacityVersion, m.MaskPattern, p.ecc, data, cw.Codewords, append([]Segment(nil), p.segments...), d, copyOptions(o)}, nil
}

// Capacity is an immutable exact single-segment capacity.
type Capacity struct {
	version                     int
	ecc                         ECC
	data, total                 int
	mode                        Mode
	countBits, payload, maximum *int
	controlBits                 int
}

func (c Capacity) Version() int        { return c.version }
func (c Capacity) ECC() ECC            { return c.ecc }
func (c Capacity) Size() int           { return c.version*4 + 17 }
func (c Capacity) DataCodewords() int  { return c.data }
func (c Capacity) TotalCodewords() int { return c.total }
func (c Capacity) CapacityBits() int   { return c.data * 8 }
func (c Capacity) Mode() Mode          { return c.mode }
func (c Capacity) ControlBits() int    { return c.controlBits }
func intCopy(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func (c Capacity) CharacterCountBits() *int { return intCopy(c.countBits) }
func (c Capacity) PayloadBits() *int        { return intCopy(c.payload) }
func (c Capacity) Maximum() *int            { return intCopy(c.maximum) }
func (c Capacity) MaxCharacters() *int {
	if c.mode == Byte {
		return nil
	}
	return intCopy(c.maximum)
}
func (c Capacity) MaxBytes() *int {
	if c.mode != Byte {
		return nil
	}
	return intCopy(c.maximum)
}
func GetCapacity(version int, ecc ECC, mode Mode, controlBits int) (Capacity, error) {
	c := Capacity{version: version, ecc: ecc, mode: mode, controlBits: controlBits}
	if controlBits < 0 || int64(controlBits) > (1<<53)-1 {
		return c, errCode(InvalidInput, "control bits must be nonnegative")
	}
	data, e := DataCodewordCount(version, ecc)
	if e != nil {
		return c, e
	}
	total, e := RawCodewordCount(version)
	if e != nil {
		return c, e
	}
	c.data = data
	c.total = total
	if mode == "" || mode == Auto {
		return c, nil
	}
	if mode != Numeric && mode != Alphanumeric && mode != Byte && mode != Kanji {
		return c, errCode(InvalidMode, "capacity requires data mode")
	}
	width, e := CountBits(mode, version)
	if e != nil {
		return c, e
	}
	bits := 0
	if controlBits < data*8 {
		bits = max(0, data*8-controlBits-4-width)
	}
	n := 0
	switch mode {
	case Numeric:
		n = bits / 10 * 3
		if bits%10 >= 7 {
			n += 2
		} else if bits%10 >= 4 {
			n++
		}
	case Alphanumeric:
		n = bits / 11 * 2
		if bits%11 >= 6 {
			n++
		}
	case Byte:
		n = bits / 8
	case Kanji:
		n = bits / 13
	}
	n = min(n, (1<<width)-1)
	c.countBits = &width
	c.payload = &bits
	c.maximum = &n
	return c, nil
}
