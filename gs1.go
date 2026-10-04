package specqr

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// GS1FNC1Separator separates a non-final variable-length GS1 element.
const GS1FNC1Separator = "\x1d"

// GS1MaxInputCharacters bounds helper input in UTF-16 code units.
const GS1MaxInputCharacters = 1_000_000

// GS1MaxElements bounds elements and URI components.
const GS1MaxElements = 16_384

// GS1Element preserves leading zeroes in both AI and Value.
type GS1Element struct {
	AI    string `json:"ai"`
	Value string `json:"value"`
}

// GS1AILength describes the catalog's fixed or bounded variable length.
type GS1AILength struct {
	Type  string `json:"type"`
	Exact int    `json:"exact,omitempty"`
	Min   int    `json:"min,omitempty"`
	Max   int    `json:"max,omitempty"`
}

// GS1AIInfo is a detached copy of one supported application identifier.
type GS1AIInfo struct {
	AI                        string      `json:"ai"`
	Label                     string      `json:"label"`
	Length                    GS1AILength `json:"length"`
	ValueKind                 string      `json:"valueKind"`
	CheckDigitRule            string      `json:"checkDigitRule"`
	DigitalLinkRole           string      `json:"digitalLinkRole"`
	Separator                 string      `json:"separator"`
	DigitalLinkPathForPrimary []string    `json:"digitalLinkPathForPrimary,omitempty"`
}

// GS1ElementStringParseResult is a validated raw element string.
type GS1ElementStringParseResult struct {
	Elements      []GS1Element `json:"elements"`
	HasSeparators bool         `json:"hasSeparators"`
}

// GS1ValidationIssue contains stable semantic diagnostics. Offsets count UTF-16 units.
type GS1ValidationIssue struct {
	Code         string  `json:"code"`
	Message      string  `json:"message"`
	Reason       string  `json:"reason"`
	AI           *string `json:"ai,omitempty"`
	Value        *string `json:"value,omitempty"`
	Key          *string `json:"key,omitempty"`
	Offset       *int    `json:"offset,omitempty"`
	ElementIndex *int    `json:"elementIndex,omitempty"`
	Expected     any     `json:"expected,omitempty"`
	Count        *int    `json:"count,omitempty"`
}

// GS1ValidationOptions controls validation. Nil CollectAllErrors means true.
// Empty Context selects element-string. Unsupported AIs cannot be enabled.
type GS1ValidationOptions struct {
	Context            string
	CollectAllErrors   *bool
	AllowUnsupportedAI bool
}

// GS1ValidationResult returns errors on failure, normalized elements on success.
type GS1ValidationResult struct {
	OK            bool                 `json:"ok"`
	Elements      []GS1Element         `json:"elements,omitempty"`
	HasSeparators *bool                `json:"hasSeparators,omitempty"`
	Errors        []GS1ValidationIssue `json:"errors,omitempty"`
	Warnings      []GS1ValidationIssue `json:"warnings"`
}

// GS1UnknownQuery preserves a non-GS1 query pair in original order.
type GS1UnknownQuery struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GS1DigitalLinkParseResult preserves path order, query order, and unknown pairs.
type GS1DigitalLinkParseResult struct {
	Elements      []GS1Element      `json:"elements"`
	Primary       GS1Element        `json:"primary"`
	PathElements  []GS1Element      `json:"pathElements"`
	QueryElements []GS1Element      `json:"queryElements"`
	UnknownQuery  []GS1UnknownQuery `json:"unknownQuery"`
}

// GS1DigitalLinkValidationResult is a parse result or structured errors.
type GS1DigitalLinkValidationResult struct {
	OK       bool                       `json:"ok"`
	Result   *GS1DigitalLinkParseResult `json:"result,omitempty"`
	Errors   []GS1ValidationIssue       `json:"errors,omitempty"`
	Warnings []GS1ValidationIssue       `json:"warnings"`
}

// GS1DigitalLinkOptions selects link creation, parsing and normalization behavior.
// Empty values select primary 01 for creation, any primary for parsing,
// unknown-query preservation and specqr-deterministic normalization. A nil
// PathAIs selects eligible qualifiers; an empty non-nil slice keeps all in query.
type GS1DigitalLinkOptions struct {
	BaseURL      string
	PrimaryAI    string
	PathAIs      []string
	UnknownQuery string
	Normalize    bool
	Mode         string
}

func gs1Failure(message string) error { return errCode(InvalidGS1, message) }
func gs1UTF16Len(value string) int {
	n := 0
	for _, r := range value {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
func gs1Text(value, label string) error {
	if len(value) > GS1MaxInputCharacters*3 || gs1UTF16Len(value) > GS1MaxInputCharacters {
		return gs1Failure(fmt.Sprintf("%s must contain at most %d characters", label, GS1MaxInputCharacters))
	}
	if !utf8.ValidString(value) {
		return gs1Failure(label + " must contain valid UTF-8")
	}
	return nil
}
func gs1Digits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
func gs1AI(value string) bool      { return len(value) >= 2 && len(value) <= 4 && gs1Digits(value) }
func gs1Primary(value string) bool { return value == "00" || value == "01" || value == "414" }
func gs1Bounded(values []GS1Element) error {
	if len(values) > GS1MaxElements {
		return gs1Failure(fmt.Sprintf("GS1 elements must contain at most %d elements", GS1MaxElements))
	}
	work := 0
	for _, e := range values {
		if err := gs1Text(e.AI, "GS1 AI"); err != nil {
			return err
		}
		if err := gs1Text(e.Value, "GS1 value"); err != nil {
			return err
		}
		work += gs1UTF16Len(e.AI) + gs1UTF16Len(e.Value)
		if work > GS1MaxInputCharacters {
			return gs1Failure(fmt.Sprintf("GS1 elements aggregate text exceeds the character work budget (%d)", GS1MaxInputCharacters))
		}
	}
	return nil
}

var gs1Catalog = func() []GS1AIInfo {
	result := make([]GS1AIInfo, 0, 50)
	add := func(ai, label string, size int, variable bool, kind, check, role string) {
		v := GS1AIInfo{AI: ai, Label: label, ValueKind: kind, CheckDigitRule: check, DigitalLinkRole: role, Separator: "none", Length: GS1AILength{Type: "fixed", Exact: size}}
		if variable {
			v.Length = GS1AILength{Type: "variable", Min: 1, Max: size}
			v.Separator = "required-when-followed"
		}
		if role == "key-qualifier" {
			v.DigitalLinkPathForPrimary = []string{"01"}
		}
		result = append(result, v)
	}
	add("00", "Serial shipping container code", 18, false, "numeric", "sscc", "primary-key")
	add("01", "Global trade item number", 14, false, "numeric", "gtin", "primary-key")
	add("02", "Contained trade item GTIN", 14, false, "numeric", "gtin", "data-attribute")
	add("10", "Batch or lot number", 20, true, "text", "none", "key-qualifier")
	for _, p := range [][2]string{{"11", "Production date"}, {"12", "Due date"}, {"13", "Packaging date"}, {"15", "Best before date"}, {"16", "Sell by date"}, {"17", "Expiration date"}} {
		add(p[0], p[1], 6, false, "numeric", "none", "data-attribute")
	}
	add("20", "Internal product variant", 2, false, "numeric", "none", "data-attribute")
	for _, p := range [][2]string{{"21", "Serial number"}, {"22", "Consumer product variant"}} {
		add(p[0], p[1], 20, true, "text", "none", "key-qualifier")
	}
	for _, p := range [][2]string{{"30", "Variable count"}, {"37", "Count of contained trade items"}} {
		add(p[0], p[1], 8, true, "numeric", "none", "data-attribute")
	}
	for _, p := range [][2]string{{"240", "Additional product identification"}, {"241", "Customer part number"}, {"400", "Customer purchase order number"}} {
		add(p[0], p[1], 30, true, "text", "none", "data-attribute")
	}
	for i, label := range []string{"Ship to global location number", "Bill to global location number", "Purchased from global location number", "Ship for global location number", "Identification of a physical location", "Global location number of the invoicing party"} {
		role := "data-attribute"
		if i == 4 {
			role = "primary-key"
		}
		add(strconv.Itoa(410+i), label, 13, false, "numeric", "none", role)
	}
	add("420", "Ship to postal code", 20, true, "text", "none", "data-attribute")
	for _, p := range [][2]string{{"422", "Country of origin"}, {"424", "Country of processing"}, {"425", "Country of disassembly"}, {"426", "Country covering full process chain"}} {
		add(p[0], p[1], 3, false, "numeric", "none", "data-attribute")
	}
	for _, start := range []int{3100, 3200} {
		label := "Net weight in kilograms"
		if start == 3200 {
			label = "Net weight in pounds"
		}
		for i := 0; i < 6; i++ {
			add(strconv.Itoa(start+i), label, 6, false, "numeric", "none", "data-attribute")
		}
	}
	for i := 91; i <= 99; i++ {
		add(strconv.Itoa(i), "Company internal information", 90, true, "text", "none", "data-attribute")
	}
	return result
}()

func gs1Info(ai string) *GS1AIInfo {
	for i := range gs1Catalog {
		if gs1Catalog[i].AI == ai {
			return &gs1Catalog[i]
		}
	}
	return nil
}
func gs1CloneInfo(v GS1AIInfo) GS1AIInfo {
	v.DigitalLinkPathForPrimary = append([]string(nil), v.DigitalLinkPathForPrimary...)
	return v
}

// GS1GetSupportedAIs returns independent catalog copies, in source catalog order.
func GS1GetSupportedAIs() []GS1AIInfo {
	v := make([]GS1AIInfo, len(gs1Catalog))
	for i, x := range gs1Catalog {
		v[i] = gs1CloneInfo(x)
	}
	return v
}

// GS1GetAIInfo returns a detached catalog entry or nil for unsupported identifiers.
func GS1GetAIInfo(ai string) *GS1AIInfo {
	v := gs1Info(ai)
	if v == nil {
		return nil
	}
	x := gs1CloneInfo(*v)
	return &x
}
func gs1Numeric(value, label string) error {
	if err := gs1Text(value, label); err != nil {
		return err
	}
	if !gs1Digits(value) {
		return gs1Failure(label + " must contain digits only")
	}
	return nil
}

// GS1CalculateCheckDigit calculates the GS1 mod-10 digit.
func GS1CalculateCheckDigit(body string) (string, error) {
	if err := gs1Numeric(body, "GS1 check digit input"); err != nil {
		return "", err
	}
	sum, weight := 0, 3
	for i := len(body) - 1; i >= 0; i-- {
		sum += int(body[i]-'0') * weight
		weight = 4 - weight
	}
	return strconv.Itoa((10 - sum%10) % 10), nil
}

// GS1ValidateCheckDigit compares the final digit with the GS1 mod-10 check.
func GS1ValidateCheckDigit(value string) (bool, error) {
	if err := gs1Numeric(value, "GS1 check digit value"); err != nil {
		return false, err
	}
	if len(value) < 2 {
		return false, gs1Failure("GS1 check digit value must include body digits and one check digit")
	}
	digit, err := GS1CalculateCheckDigit(value[:len(value)-1])
	return digit == value[len(value)-1:], err
}

// GS1CalculateGTINCheckDigit accepts 7, 11, 12 or 13 body digits.
func GS1CalculateGTINCheckDigit(body string) (string, error) {
	if err := gs1Numeric(body, "GTIN body"); err != nil {
		return "", err
	}
	switch len(body) {
	case 7, 11, 12, 13:
	default:
		return "", gs1Failure("GTIN body must be 7, 11, 12, or 13 digits")
	}
	return GS1CalculateCheckDigit(body)
}

// GS1AppendGTINCheckDigit appends a GTIN check digit.
func GS1AppendGTINCheckDigit(body string) (string, error) {
	d, e := GS1CalculateGTINCheckDigit(body)
	if e != nil {
		return "", e
	}
	return body + d, nil
}

// GS1ValidateGTINCheckDigit accepts 8, 12, 13 or 14 complete digits.
func GS1ValidateGTINCheckDigit(value string) (bool, error) {
	if err := gs1Numeric(value, "GTIN"); err != nil {
		return false, err
	}
	switch len(value) {
	case 8, 12, 13, 14:
	default:
		return false, gs1Failure("GTIN must be 8, 12, 13, or 14 digits")
	}
	return GS1ValidateCheckDigit(value)
}

// GS1CalculateSSCCCheckDigit accepts exactly 17 body digits.
func GS1CalculateSSCCCheckDigit(body string) (string, error) {
	if err := gs1Numeric(body, "SSCC body"); err != nil {
		return "", err
	}
	if len(body) != 17 {
		return "", gs1Failure("SSCC body must be exactly 17 digits")
	}
	return GS1CalculateCheckDigit(body)
}

// GS1AppendSSCCCheckDigit appends an SSCC check digit.
func GS1AppendSSCCCheckDigit(body string) (string, error) {
	d, e := GS1CalculateSSCCCheckDigit(body)
	if e != nil {
		return "", e
	}
	return body + d, nil
}

// GS1ValidateSSCCCheckDigit accepts exactly 18 complete digits.
func GS1ValidateSSCCCheckDigit(value string) (bool, error) {
	if err := gs1Numeric(value, "SSCC"); err != nil {
		return false, err
	}
	if len(value) != 18 {
		return false, gs1Failure("SSCC must be exactly 18 digits")
	}
	return GS1ValidateCheckDigit(value)
}
func gs1JSONQuote(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
func gs1Element(e GS1Element, index int) (GS1Element, error) {
	if err := gs1Text(e.AI, "GS1 AI"); err != nil {
		return GS1Element{}, err
	}
	if err := gs1Text(e.Value, "GS1 value"); err != nil {
		return GS1Element{}, err
	}
	if !gs1AI(e.AI) {
		return GS1Element{}, gs1Failure(fmt.Sprintf("GS1 element %d has invalid AI %s; expected 2 to 4 digits", index, gs1JSONQuote(e.AI)))
	}
	info := gs1Info(e.AI)
	if info == nil {
		return GS1Element{}, gs1Failure(fmt.Sprintf("Unsupported GS1 AI %s. Add explicit support before using it.", e.AI))
	}
	value := e.Value
	prefix := "GS1 AI " + e.AI + " value "
	problem := ""
	switch {
	case value == "":
		problem = "must not be empty"
	case strings.Contains(value, GS1FNC1Separator):
		problem = "must not contain the FNC1 separator"
	case strings.ContainsAny(value, "()"):
		problem = "must be raw data without human-readable parentheses"
	default:
		for _, b := range []byte(value) {
			if b < 32 || b > 126 {
				problem = "must use printable ASCII characters"
				break
			}
		}
		if problem == "" && info.ValueKind == "numeric" && !gs1Digits(value) {
			problem = "must contain digits only"
		}
	}
	if problem != "" {
		return GS1Element{}, gs1Failure(prefix + problem)
	}
	if info.Length.Type == "variable" && len(value) > info.Length.Max {
		return GS1Element{}, gs1Failure(fmt.Sprintf("%smust be at most %d characters", prefix, info.Length.Max))
	}
	if info.Length.Type == "fixed" && len(value) != info.Length.Exact {
		return GS1Element{}, gs1Failure(fmt.Sprintf("%smust be exactly %d characters", prefix, info.Length.Exact))
	}
	if info.CheckDigitRule == "gtin" {
		ok, err := GS1ValidateGTINCheckDigit(value)
		if err != nil {
			return GS1Element{}, err
		}
		if !ok {
			return GS1Element{}, gs1Failure(prefix + "has an invalid GTIN check digit")
		}
	}
	if info.CheckDigitRule == "sscc" {
		ok, err := GS1ValidateSSCCCheckDigit(value)
		if err != nil {
			return GS1Element{}, err
		}
		if !ok {
			return GS1Element{}, gs1Failure(prefix + "has an invalid SSCC check digit")
		}
	}
	return e, nil
}

// GS1FromHumanReadable parses parenthesized AIs, validating each element.
func GS1FromHumanReadable(input string) ([]GS1Element, error) {
	if err := gs1Text(input, "GS1 human-readable input"); err != nil {
		return nil, err
	}
	if input == "" {
		return nil, gs1Failure("GS1 human-readable input must not be empty")
	}
	out := []GS1Element{}
	for pos := 0; pos < len(input); {
		if len(out) == GS1MaxElements {
			return nil, gs1Failure("GS1 elements exceed element limit")
		}
		if input[pos] != '(' {
			return nil, gs1Failure(fmt.Sprintf("GS1 human-readable input must contain an AI in parentheses at offset %d", pos))
		}
		close := strings.IndexByte(input[pos+1:], ')')
		if close < 0 {
			return nil, gs1Failure(fmt.Sprintf("GS1 AI starting at offset %d is missing a closing parenthesis", pos))
		}
		close += pos + 1
		end := len(input)
		if n := strings.IndexByte(input[close+1:], '('); n >= 0 {
			end = close + 1 + n
		}
		e, err := gs1Element(GS1Element{input[pos+1 : close], input[close+1 : end]}, len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, e)
		pos = end
	}
	return out, nil
}

// GS1ToElementString emits required FNC1 separators without escaping percent.
func GS1ToElementString(elements []GS1Element) (string, error) { return gs1String(elements, false) }

// GS1ToHumanReadable surrounds each AI with parentheses.
func GS1ToHumanReadable(elements []GS1Element) (string, error) { return gs1String(elements, true) }
func gs1String(elements []GS1Element, human bool) (string, error) {
	if err := gs1Bounded(elements); err != nil {
		return "", err
	}
	if len(elements) == 0 {
		return "", gs1Failure("GS1 elements must not be empty")
	}
	var out strings.Builder
	for i, raw := range elements {
		e, err := gs1Element(raw, i)
		if err != nil {
			return "", err
		}
		if human {
			out.WriteByte('(')
		}
		out.WriteString(e.AI)
		if human {
			out.WriteByte(')')
		}
		out.WriteString(e.Value)
		if !human && i+1 < len(elements) && gs1Info(e.AI).Length.Type == "variable" {
			out.WriteString(GS1FNC1Separator)
		}
		if out.Len() > GS1MaxInputCharacters {
			label := "element string"
			if human {
				label = "human-readable"
			}
			return "", gs1Failure("GS1 " + label + " output exceeds character limit")
		}
	}
	return out.String(), nil
}

// GS1ElementStringToHumanReadable parses then formats a raw element string.
func GS1ElementStringToHumanReadable(input string) (string, error) {
	r, e := GS1ParseElementString(input)
	if e != nil {
		return "", e
	}
	return GS1ToHumanReadable(r.Elements)
}
func gs1ReadAI(input string, offset int) *GS1AIInfo {
	for _, n := range []int{4, 3, 2} {
		if offset+n <= len(input) {
			if v := gs1Info(input[offset : offset+n]); v != nil {
				return v
			}
		}
	}
	return nil
}
func gs1UTF16End(input string, start, count int) int {
	units := 0
	for i, r := range input[start:] {
		units++
		if r > 0xffff {
			units++
		}
		if units >= count {
			return start + i + utf8.RuneLen(r)
		}
	}
	return len(input)
}

// GS1ParseElementString parses the bounded raw format. A final variable field
// ending in a complete fixed-field-looking suffix is treated as ambiguous.
func GS1ParseElementString(input string) (GS1ElementStringParseResult, error) {
	bad := func(e error) (GS1ElementStringParseResult, error) { return GS1ElementStringParseResult{}, e }
	if err := gs1Text(input, "GS1 element string input"); err != nil {
		return bad(err)
	}
	if input == "" {
		return bad(gs1Failure("GS1 element string input must not be empty"))
	}
	if strings.ContainsAny(input, "()") {
		return bad(gs1Failure("GS1 element string input must be raw data without human-readable parentheses; use parseGs1HumanReadable() and createGs1ElementString() before generate(..., { gs1: true })"))
	}
	out := []GS1Element{}
	for pos := 0; pos < len(input); {
		if len(out) == GS1MaxElements {
			return bad(gs1Failure("GS1 elements exceed element limit"))
		}
		if input[pos] == 29 {
			return bad(gs1Failure(fmt.Sprintf("GS1 element string has an unexpected FNC1 separator at offset %d", pos)))
		}
		info := gs1ReadAI(input, pos)
		if info == nil {
			return bad(gs1Failure(fmt.Sprintf("Unsupported GS1 AI at offset %d", pos)))
		}
		start := pos + len(info.AI)
		end := len(input)
		variable := info.Length.Type == "variable"
		if variable {
			if n := strings.IndexByte(input[start:], 29); n >= 0 {
				end = start + n
			}
		} else {
			end = gs1UTF16End(input, start, info.Length.Exact)
		}
		if variable && end == len(input) { // Only the final 22 UTF-16 units can be a fixed catalog suffix.
			for rel := range input[start:end] {
				scan := start + rel
				if rel == 0 || end-scan > 88 {
					continue
				}
				n := gs1UTF16Len(input[scan:end])
				if n > 22 {
					continue
				}
				if tail := gs1ReadAI(input, scan); tail != nil && tail.Length.Type == "fixed" && n == len(tail.AI)+tail.Length.Exact {
					return bad(gs1Failure(fmt.Sprintf("GS1 variable-length element at offset %d is missing an FNC1 separator before offset %d", start, gs1UTF16Len(input[:scan]))))
				}
			}
		}
		e, err := gs1Element(GS1Element{info.AI, input[start:end]}, len(out))
		if err != nil {
			return bad(err)
		}
		out = append(out, e)
		pos = end
		if pos < len(input) && input[pos] == 29 && variable {
			pos++
			if pos == len(input) {
				return bad(gs1Failure("GS1 element string must not end with an FNC1 separator"))
			}
		}
	}
	return GS1ElementStringParseResult{out, strings.Contains(input, GS1FNC1Separator)}, nil
}
func gs1SimpleIssue(code, message, reason string, expected any) GS1ValidationIssue {
	return GS1ValidationIssue{Code: code, Message: message, Reason: reason, Expected: expected}
}
func gs1Invalid(issues ...GS1ValidationIssue) GS1ValidationResult {
	return GS1ValidationResult{Errors: issues, Warnings: []GS1ValidationIssue{}}
}
func gs1ValidationOptions(options []GS1ValidationOptions) (GS1ValidationOptions, *GS1ValidationIssue) {
	opts := GS1ValidationOptions{}
	if len(options) > 1 {
		v := gs1SimpleIssue("GS1_INVALID_INPUT", "GS1 validation accepts at most one options value", "invalid-options", nil)
		return opts, &v
	}
	if len(options) == 1 {
		opts = options[0]
	}
	if opts.Context == "" {
		opts.Context = "element-string"
	}
	if opts.Context != "element-string" && opts.Context != "digital-link" {
		v := gs1SimpleIssue("GS1_INVALID_INPUT", "GS1 validation options.context must be \"element-string\" or \"digital-link\"", "invalid-options", "element-string or digital-link")
		return opts, &v
	}
	if opts.AllowUnsupportedAI {
		v := gs1SimpleIssue("GS1_INVALID_INPUT", "GS1 validation options.allowUnsupportedAi must be false", "invalid-options", false)
		return opts, &v
	}
	return opts, nil
}

// GS1ValidateElements validates all elements by default and optionally requires
// a Digital Link primary identifier. It does not imply full GS1 conformance.
func GS1ValidateElements(elements []GS1Element, options ...GS1ValidationOptions) GS1ValidationResult {
	opts, issue := gs1ValidationOptions(options)
	if issue != nil {
		return gs1Invalid(*issue)
	}
	if err := gs1Bounded(elements); err != nil {
		return gs1Invalid(gs1Issue(err, nil, nil, "", false))
	}
	if len(elements) == 0 {
		return gs1Invalid(gs1Issue(gs1Failure("GS1 elements must not be empty"), nil, nil, "", false))
	}
	out := make([]GS1Element, 0, len(elements))
	issues := []GS1ValidationIssue{}
	for i, raw := range elements {
		e, err := gs1Element(raw, i)
		if err != nil {
			issues = append(issues, gs1Issue(err, &raw, &i, "", false))
			if opts.CollectAllErrors != nil && !*opts.CollectAllErrors {
				break
			}
		} else {
			out = append(out, e)
		}
	}
	if len(issues) > 0 {
		return gs1Invalid(issues...)
	}
	if opts.Context == "digital-link" {
		found := false
		for _, e := range out {
			found = found || gs1Primary(e.AI)
		}
		if !found {
			return gs1Invalid(gs1SimpleIssue("GS1_INVALID_DIGITAL_LINK_PLACEMENT", "GS1 Digital Link elements must include a primary AI 00, 01, or 414", "invalid-digital-link-placement", "primary AI 00, 01, or 414"))
		}
	}
	return GS1ValidationResult{OK: true, Elements: out, Warnings: []GS1ValidationIssue{}}
}

// GS1ValidateElementString returns structured diagnostics rather than an error.
func GS1ValidateElementString(input string, options ...GS1ValidationOptions) GS1ValidationResult {
	_, issue := gs1ValidationOptions(options)
	if issue != nil {
		return gs1Invalid(*issue)
	}
	p, e := GS1ParseElementString(input)
	if e != nil {
		return gs1Invalid(gs1Issue(e, nil, nil, input, false))
	}
	return GS1ValidationResult{OK: true, Elements: p.Elements, HasSeparators: &p.HasSeparators, Warnings: []GS1ValidationIssue{}}
}
func gs1DigitsAfter(value, pattern string, max int) string {
	p := strings.Index(value, pattern)
	if p < 0 {
		return ""
	}
	p += len(pattern)
	end := p
	for end < len(value) && end-p < max && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	return value[p:end]
}
func gs1NumberAfter(value, pattern string) *int {
	s := gs1DigitsAfter(value, pattern, 20)
	if s == "" {
		return nil
	}
	n, e := strconv.Atoi(s)
	if e != nil {
		return nil
	}
	return &n
}
func gs1ByteAtUTF16(value string, offset int) (int, bool) {
	n := 0
	for i, r := range value {
		if n == offset {
			return i, true
		}
		n++
		if r > 0xffff {
			n++
		}
	}
	return len(value), n == offset
}
func gs1Issue(err error, raw *GS1Element, index *int, input string, digital bool) GS1ValidationIssue {
	message := err.Error()
	if e, ok := err.(*Error); ok {
		message = e.Message
	}
	code, reason := "GS1_INVALID_INPUT", "invalid-input"
	var expected any
	contains := func(s string) bool { return strings.Contains(message, s) }
	length := ""
	for _, p := range []string{"exactly ", "at most "} {
		n := gs1DigitsAfter(message, p, 20)
		if n != "" && contains(p+n+" characters") {
			length = p + n + " characters"
			break
		}
	}
	switch {
	case digital && (contains("absolute http or https URL") || contains("must use http or https")):
		code = "GS1_DIGITAL_LINK_INVALID_URI"
		reason = "invalid-uri"
		expected = "absolute http or https URL"
	case digital && contains("must not include a fragment"):
		code = "GS1_DIGITAL_LINK_FRAGMENT_NOT_ALLOWED"
		reason = "fragment-not-allowed"
		expected = "URI without fragment"
	case digital && contains("valid percent-encoding"):
		code = "GS1_INVALID_PERCENT_ENCODING"
		reason = "invalid-percent-encoding"
		expected = "percent escapes must use two hexadecimal digits"
	case digital && contains("query parameter") && contains("is not a GS1 AI"):
		code = "GS1_DIGITAL_LINK_UNKNOWN_QUERY"
		reason = "unknown-query"
		expected = "GS1 AI query parameter or unknownQuery: \"preserve\""
	case digital && (contains("primaryAi must be one") || contains("unknownQuery must be")):
		reason = "invalid-options"
		expected = "preserve or reject"
		if contains("primaryAi") {
			expected = "00, 01, or 414"
		}
	case digital && (contains("path must") || contains("path segment")):
		reason = "malformed-path"
		expected = "Digital Link path containing primary AI and AI/value pairs"
	case contains("Unsupported GS1 AI"):
		code = "GS1_UNSUPPORTED_AI"
		reason = "unsupported-ai"
		expected = "supported GS1 AI"
	case length != "":
		code = "GS1_INVALID_LENGTH"
		reason = "invalid-length"
		expected = length
	case contains("digits only") || contains("printable ASCII"):
		code = "GS1_INVALID_CHARSET"
		reason = "invalid-charset"
		expected = "printable ASCII"
		if contains("digits only") {
			expected = "digits only"
		}
	case contains("missing an FNC1 separator"):
		code = "GS1_MISSING_SEPARATOR"
		reason = "missing-separator"
		expected = "FNC1 separator before the next GS1 element"
	case contains("unexpected FNC1 separator") || contains("must not end with an FNC1 separator") || contains("must not contain the FNC1 separator"):
		code = "GS1_UNEXPECTED_SEPARATOR"
		reason = "unexpected-separator"
		expected = "separator only after a non-final variable-length GS1 element"
	case contains("invalid GTIN check digit") || contains("invalid SSCC check digit"):
		code = "GS1_INVALID_CHECK_DIGIT"
		reason = "invalid-check-digit"
		expected = "valid GTIN check digit"
		if contains("SSCC") {
			expected = "valid SSCC check digit"
		}
	case contains("cannot be placed in the Digital Link path"):
		code = "GS1_INVALID_DIGITAL_LINK_PLACEMENT"
		reason = "invalid-digital-link-placement"
	case contains("duplicate AI"):
		code = "GS1_DUPLICATE_AI"
		reason = "duplicate-ai"
		expected = "unique GS1 AI within the Digital Link URI"
	}
	out := gs1SimpleIssue(code, message, reason, expected)
	ai := gs1DigitsAfter(message, "GS1 AI ", 4)
	if len(ai) < 2 {
		ai = gs1DigitsAfter(message, "duplicate AI ", 4)
	}
	if len(ai) >= 2 {
		out.AI = &ai
	}
	out.Offset = gs1NumberAfter(message, "offset ")
	if out.AI == nil && out.Offset != nil && input != "" {
		if offset, ok := gs1ByteAtUTF16(input, *out.Offset); ok {
			n := 0
			for n < 4 && offset+n < len(input) && input[offset+n] >= '0' && input[offset+n] <= '9' {
				n++
			}
			if n >= 2 {
				v := input[offset : offset+n]
				out.AI = &v
			} else {
				for size := 2; size <= 4; size++ {
					if *out.Offset < size {
						continue
					}
					if start, ok := gs1ByteAtUTF16(input, *out.Offset-size); ok && gs1Info(input[start:offset]) != nil {
						v := input[start:offset]
						out.AI = &v
						break
					}
				}
			}
		}
	}
	out.ElementIndex = gs1NumberAfter(message, "GS1 element ")
	if out.ElementIndex == nil && index != nil {
		i := *index
		out.ElementIndex = &i
	}
	if raw != nil {
		v := raw.Value
		out.Value = &v
	}
	if code == "GS1_DIGITAL_LINK_UNKNOWN_QUERY" {
		v := strings.TrimPrefix(message, "GS1 Digital Link query parameter ")
		v = strings.TrimSuffix(v, " is not a GS1 AI")
		var key string
		if json.Unmarshal([]byte(v), &key) == nil {
			out.Key = &key
		}
	}
	return out
}
func gs1LinkOptions(values []GS1DigitalLinkOptions) (GS1DigitalLinkOptions, error) {
	if len(values) > 1 {
		return GS1DigitalLinkOptions{}, gs1Failure("GS1 Digital Link accepts at most one options value")
	}
	v := GS1DigitalLinkOptions{}
	if len(values) == 1 {
		v = values[0]
	}
	if v.UnknownQuery == "" {
		v.UnknownQuery = "preserve"
	}
	if v.Mode == "" {
		v.Mode = "specqr-deterministic"
	}
	return v, nil
}
func gs1CheckPrimary(ai string) error {
	if !gs1Primary(ai) {
		return gs1Failure("GS1 Digital Link primaryAi must be one of 00, 01, or 414")
	}
	return nil
}
func gs1CheckPolicy(policy string) error {
	if policy != "preserve" && policy != "reject" {
		return gs1Failure("GS1 Digital Link unknownQuery must be \"preserve\" or \"reject\"")
	}
	return nil
}
func gs1Eligible(ai, primary string) bool {
	return primary == "01" && (ai == "10" || ai == "21" || ai == "22")
}
func gs1Placement(ai, primary string) error {
	if gs1Info(ai) == nil {
		return gs1Failure(fmt.Sprintf("Unsupported GS1 AI %s. Add explicit support before using it.", ai))
	}
	if !gs1Eligible(ai, primary) {
		return gs1Failure(fmt.Sprintf("GS1 AI %s cannot be placed in the Digital Link path after primary AI %s", ai, primary))
	}
	return nil
}
func gs1Unique(ai string, seen map[string]bool) error {
	if seen[ai] {
		return gs1Failure("GS1 Digital Link input must not contain duplicate AI " + ai)
	}
	seen[ai] = true
	return nil
}

// GS1CreateDigitalLink creates a deterministic local URI, without fetching it.
// Literal dot-only path values are moved to query to prevent URL data loss.
func GS1CreateDigitalLink(elements []GS1Element, options GS1DigitalLinkOptions) (string, error) {
	if err := gs1Bounded(elements); err != nil {
		return "", err
	}
	if options.BaseURL == "" {
		return "", gs1Failure("GS1 Digital Link options.baseUrl is required")
	}
	u, err := gs1ParseURL(options.BaseURL, true)
	if err != nil {
		return "", gs1Failure("GS1 Digital Link options.baseUrl must be a valid http or https URL")
	}
	if u.scheme != "http" && u.scheme != "https" {
		return "", gs1Failure("GS1 Digital Link options.baseUrl must use http or https")
	}
	if (u.query != nil && *u.query != "") || (u.fragment != nil && *u.fragment != "") {
		return "", gs1Failure("GS1 Digital Link options.baseUrl must not include query or fragment components")
	}
	primary := options.PrimaryAI
	if primary == "" {
		primary = "01"
	}
	if err := gs1CheckPrimary(primary); err != nil {
		return "", err
	}
	var paths map[string]bool
	if options.PathAIs != nil {
		if len(options.PathAIs) > GS1MaxElements {
			return "", gs1Failure("GS1 Digital Link pathAis exceeds element limit")
		}
		paths = map[string]bool{}
		for _, ai := range options.PathAIs {
			if !gs1AI(ai) {
				return "", gs1Failure("GS1 Digital Link pathAis entries must be 2 to 4 digit AI strings")
			}
			if ai != primary {
				if err := gs1Placement(ai, primary); err != nil {
					return "", err
				}
				paths[ai] = true
			}
		}
	}
	if len(elements) == 0 {
		return "", gs1Failure("GS1 Digital Link input elements must not be empty")
	}
	normalized := make([]GS1Element, 0, len(elements))
	seen := map[string]bool{}
	var selected *GS1Element
	for i, raw := range elements {
		e, err := gs1Element(raw, i)
		if err != nil {
			return "", err
		}
		if err := gs1Unique(e.AI, seen); err != nil {
			return "", err
		}
		normalized = append(normalized, e)
		if e.AI == primary {
			selected = &normalized[len(normalized)-1]
		}
	}
	if selected == nil {
		return "", gs1Failure("GS1 Digital Link input must include primary AI " + primary)
	}
	path := []GS1Element{*selected}
	query := []GS1Element{}
	for _, e := range normalized {
		if e.AI == primary {
			continue
		}
		inPath := gs1Eligible(e.AI, primary)
		if paths != nil {
			inPath = paths[e.AI]
		}
		if inPath && e.Value != "." && e.Value != ".." {
			path = append(path, e)
		} else {
			query = append(query, e)
		}
	}
	var pathname strings.Builder
	pathname.WriteString(strings.TrimRight(u.path, "/"))
	for _, e := range path {
		pathname.WriteByte('/')
		pathname.WriteString(gs1Encode(e.AI, 0))
		pathname.WriteByte('/')
		pathname.WriteString(gs1Encode(e.Value, 0))
	}
	sort.Slice(query, func(i, j int) bool {
		if query[i].AI != query[j].AI {
			return query[i].AI < query[j].AI
		}
		return query[i].Value < query[j].Value
	})
	var search strings.Builder
	for i, e := range query {
		if i > 0 {
			search.WriteByte('&')
		}
		search.WriteString(gs1Encode(e.AI, 1))
		search.WriteByte('=')
		search.WriteString(gs1Encode(e.Value, 1))
	}
	u.path = pathname.String()
	u.query = nil
	u.fragment = nil
	if search.Len() > 0 {
		s := search.String()
		u.query = &s
	}
	return u.serialize()
}
func gs1PathParts(path string) ([]string, error) {
	v := strings.Trim(path, "/")
	if v == "" {
		return nil, gs1Failure("GS1 Digital Link path must include primary AI 00, 01, or 414")
	}
	parts := strings.Split(v, "/")
	for _, p := range parts {
		if p == "" {
			return nil, gs1Failure("GS1 Digital Link path must not contain empty segments")
		}
	}
	return parts, nil
}
func gs1FirstAI(parts []string, selected string) (int, error) {
	for i, p := range parts {
		if (selected == "" && gs1Primary(p)) || (selected != "" && selected == p) {
			return i, nil
		}
	}
	return 0, gs1Failure("GS1 Digital Link path must include primary AI 00, 01, or 414")
}
func gs1ParseLink(u gs1URL, opts GS1DigitalLinkOptions) (GS1DigitalLinkParseResult, error) {
	bad := func(e error) (GS1DigitalLinkParseResult, error) { return GS1DigitalLinkParseResult{}, e }
	if err := gs1CheckURI(u, false); err != nil {
		return bad(err)
	}
	if opts.PrimaryAI != "" {
		if err := gs1CheckPrimary(opts.PrimaryAI); err != nil {
			return bad(err)
		}
	}
	if err := gs1CheckPolicy(opts.UnknownQuery); err != nil {
		return bad(err)
	}
	parts, err := gs1PathParts(u.path)
	if err != nil {
		return bad(err)
	}
	start, err := gs1FirstAI(parts, opts.PrimaryAI)
	if err != nil {
		return bad(err)
	}
	if (len(parts)-start)%2 != 0 {
		return bad(gs1Failure("GS1 Digital Link path must contain AI/value pairs"))
	}
	path := []GS1Element{}
	query := []GS1Element{}
	unknown := []GS1UnknownQuery{}
	seen := map[string]bool{}
	for i := start; i < len(parts); i += 2 {
		ai := parts[i]
		if !gs1AI(ai) {
			return bad(gs1Failure(fmt.Sprintf("GS1 Digital Link path segment %d must be a GS1 AI", i+1)))
		}
		value, err := gs1StrictDecode(parts[i+1], "value for AI "+ai)
		if err != nil {
			return bad(err)
		}
		e, err := gs1Element(GS1Element{ai, value}, len(path))
		if err != nil {
			return bad(err)
		}
		if len(path) > 0 {
			if err := gs1Placement(ai, path[0].AI); err != nil {
				return bad(err)
			}
		}
		if err := gs1Unique(ai, seen); err != nil {
			return bad(err)
		}
		path = append(path, e)
	}
	if u.query != nil {
		raw := *u.query
		if strings.Count(raw, "&")+1 > GS1MaxElements {
			return bad(gs1Failure("GS1 Digital Link query component count exceeds limit"))
		}
		for _, pair := range strings.Split(raw, "&") {
			if pair == "" {
				continue
			}
			k, v, _ := strings.Cut(pair, "=")
			key, value := gs1FormDecode(k), gs1FormDecode(v)
			if gs1AI(key) {
				e, err := gs1Element(GS1Element{key, value}, len(path)+len(query))
				if err != nil {
					return bad(err)
				}
				if err := gs1Unique(key, seen); err != nil {
					return bad(err)
				}
				query = append(query, e)
			} else if opts.UnknownQuery == "preserve" {
				unknown = append(unknown, GS1UnknownQuery{key, value})
			} else {
				return bad(gs1Failure("GS1 Digital Link query parameter " + gs1JSONQuote(key) + " is not a GS1 AI"))
			}
		}
	}
	elements := make([]GS1Element, 0, len(path)+len(query))
	elements = append(elements, path...)
	elements = append(elements, query...)
	return GS1DigitalLinkParseResult{elements, path[0], path, query, unknown}, nil
}

// GS1ParseDigitalLink parses a local HTTP(S) URI using the documented host profile.
func GS1ParseDigitalLink(uri string, options ...GS1DigitalLinkOptions) (GS1DigitalLinkParseResult, error) {
	opts, err := gs1LinkOptions(options)
	if err != nil {
		return GS1DigitalLinkParseResult{}, err
	}
	u, err := gs1ParseURL(uri, false)
	if err != nil {
		return GS1DigitalLinkParseResult{}, err
	}
	return gs1ParseLink(u, opts)
}

// GS1ValidateDigitalLink checks percent syntax and returns errors and warnings.
func GS1ValidateDigitalLink(uri string, options ...GS1DigitalLinkOptions) GS1DigitalLinkValidationResult {
	bad := func(issue GS1ValidationIssue) GS1DigitalLinkValidationResult {
		return GS1DigitalLinkValidationResult{Errors: []GS1ValidationIssue{issue}, Warnings: []GS1ValidationIssue{}}
	}
	opts, err := gs1LinkOptions(options)
	if err != nil {
		return bad(gs1Issue(err, nil, nil, "", true))
	}
	if opts.Normalize {
		return bad(gs1SimpleIssue("GS1_INVALID_INPUT", "GS1 Digital Link validation options.normalize is not implemented yet", "unsupported-option", false))
	}
	u, err := gs1ParseURL(uri, false)
	if err != nil {
		return bad(gs1Issue(err, nil, nil, "", true))
	}
	if gs1InvalidPercent(u.path) || (u.query != nil && gs1InvalidPercent(*u.query)) {
		return bad(gs1Issue(gs1Failure("GS1 Digital Link URI must use valid percent-encoding"), nil, nil, "", true))
	}
	r, err := gs1ParseLink(u, opts)
	if err != nil {
		return bad(gs1Issue(err, nil, nil, "", true))
	}
	warnings := []GS1ValidationIssue{}
	if u.scheme == "http" {
		warnings = append(warnings, gs1SimpleIssue("GS1_DIGITAL_LINK_HTTP", "GS1 Digital Link URI uses http. Use https when transport security is required.", "http-uri", nil))
	}
	if len(r.UnknownQuery) > 0 {
		w := gs1SimpleIssue("GS1_DIGITAL_LINK_UNKNOWN_QUERY_PRESERVED", "GS1 Digital Link URI contains non-GS1 query parameters preserved in unknownQuery.", "unknown-query-preserved", nil)
		count := len(r.UnknownQuery)
		w.Count = &count
		warnings = append(warnings, w)
	}
	return GS1DigitalLinkValidationResult{OK: true, Result: &r, Warnings: warnings}
}

// GS1NormalizeDigitalLink emits canonical ordering while preserving payload and
// unknown query pairs. Dot-only GS1 values are safely serialized in the query.
func GS1NormalizeDigitalLink(uri string, options ...GS1DigitalLinkOptions) (string, error) {
	opts, err := gs1LinkOptions(options)
	if err != nil {
		return "", err
	}
	if opts.Mode != "specqr-deterministic" {
		return "", gs1Failure("GS1 Digital Link normalization mode must be \"specqr-deterministic\"")
	}
	u, err := gs1ParseURL(uri, false)
	if err != nil {
		return "", err
	}
	if err := gs1CheckURI(u, false); err != nil {
		return "", err
	}
	if gs1InvalidPercent(u.path) || (u.query != nil && gs1InvalidPercent(*u.query)) {
		return "", gs1Failure("GS1 Digital Link URI must use valid percent-encoding")
	}
	parsed, err := gs1ParseLink(u, opts)
	if err != nil {
		return "", err
	}
	parts, err := gs1PathParts(u.path)
	if err != nil {
		return "", err
	}
	start, err := gs1FirstAI(parts, opts.PrimaryAI)
	if err != nil {
		return "", err
	}
	u.path = "/" + strings.Join(parts[:start], "/")
	u.query = nil
	u.fragment = nil
	stem, err := u.serialize()
	if err != nil {
		return "", err
	}
	out, err := GS1CreateDigitalLink(parsed.Elements, GS1DigitalLinkOptions{BaseURL: stem, PrimaryAI: parsed.Primary.AI})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(out)
	query := strings.Contains(out, "?")
	for _, q := range parsed.UnknownQuery {
		if query {
			b.WriteByte('&')
		} else {
			b.WriteByte('?')
		}
		query = true
		b.WriteString(gs1Encode(q.Key, 1))
		b.WriteByte('=')
		b.WriteString(gs1Encode(q.Value, 1))
	}
	out = b.String()
	if err := gs1Text(out, "GS1 Digital Link output"); err != nil {
		return "", err
	}
	return out, nil
}
