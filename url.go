package specqr

import (
	"math"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// gs1URL is a local, intentionally scoped special-URL adapter. Using net/url
// alone would change the WHATWG-compatible host, port, credential, and path
// behavior used by GS1 Digital Links. The frozen Unicode 15 profile below is a
// conservative subset, not a full IDNA/UTS #46 implementation. It accepts stable
// Unicode scalar labels, maps fullwidth ASCII and dot variants, removes soft
// hyphens, applies frozen lowercase mappings, and checks a restricted RTL form.
// Marks, compatibility-unstable scalars, contextual Hangul Jamo, controls, and
// unassigned code points are rejected. ACE labels must decode to an accepted,
// already-mapped Unicode label and round-trip to their canonical encoding.
// Nothing here depends on the host OS, network, Go Unicode version, or locale.
type gs1URL struct {
	scheme, authority, path string
	query, fragment         *string
}

func (u gs1URL) serialize() (string, error) {
	var out strings.Builder
	out.WriteString(u.scheme)
	out.WriteString("://")
	out.WriteString(u.authority)
	out.WriteString(u.path)
	if u.query != nil {
		out.WriteByte('?')
		out.WriteString(*u.query)
	}
	if u.fragment != nil {
		out.WriteByte('#')
		out.WriteString(*u.fragment)
	}
	value := out.String()
	if err := gs1Text(value, "GS1 Digital Link output"); err != nil {
		return "", err
	}
	return value, nil
}

func gs1InvalidURI() error {
	return gs1Failure("GS1 Digital Link URI must be an absolute http or https URL")
}

func gs1Escape(out *strings.Builder, value byte) {
	const digits = "0123456789ABCDEF"
	out.WriteByte('%')
	out.WriteByte(digits[value>>4])
	out.WriteByte(digits[value&15])
}

func gs1ASCIIAlpha(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
func gs1ASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// Modes: 0 encodeURIComponent, 1 form encoding, 2 path, 3 special query.
func gs1Encode(value string, mode byte) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		b := value[i]
		var keep bool
		switch mode {
		case 0:
			keep = gs1ASCIIAlpha(b) || gs1ASCIIDigit(b) || strings.ContainsRune("~!*'()-._", rune(b))
		case 1:
			keep = gs1ASCIIAlpha(b) || gs1ASCIIDigit(b) || strings.ContainsRune("*-._", rune(b))
		case 2:
			keep = b > 32 && b < 127 && !strings.ContainsRune("\"#<>?`{}^", rune(b))
		default:
			keep = b > 32 && b < 127 && !strings.ContainsRune("\"#'<>", rune(b))
		}
		if keep {
			out.WriteByte(b)
		} else if mode == 1 && b == ' ' {
			out.WriteByte('+')
		} else {
			gs1Escape(&out, b)
		}
	}
	return out.String()
}

func gs1Hex(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

func gs1InvalidPercent(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			continue
		}
		if i+2 >= len(value) {
			return true
		}
		_, a := gs1Hex(value[i+1])
		_, b := gs1Hex(value[i+2])
		if !a || !b {
			return true
		}
	}
	return false
}

func gs1PercentBytes(value string, form bool) []byte {
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '%' && i+2 < len(value) {
			a, okA := gs1Hex(value[i+1])
			b, okB := gs1Hex(value[i+2])
			if okA && okB {
				out = append(out, a*16+b)
				i += 2
				continue
			}
		}
		b := value[i]
		if form && b == '+' {
			b = ' '
		}
		out = append(out, b)
	}
	return out
}

func gs1StrictDecode(value, label string) (string, error) {
	decoded := gs1PercentBytes(value, false)
	if gs1InvalidPercent(value) || !utf8.Valid(decoded) {
		return "", gs1Failure("GS1 Digital Link path " + label + " must be valid percent-encoding")
	}
	return string(decoded), nil
}

// gs1FormDecode follows the replacement behavior of the UTF-8 decoder used by
// URLSearchParams: one U+FFFD per maximal ill-formed subsequence. Go's range and
// strings.ToValidUTF8 have different behavior on truncated multi-byte sequences.
func gs1FormDecode(value string) string {
	data := gs1PercentBytes(value, true)
	var out strings.Builder
	out.Grow(len(data))
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r != utf8.RuneError || size != 1 {
			out.WriteRune(r)
			i += size
			continue
		}
		end := i + 1
		b := data[i]
		expected, low, high := 0, byte(0x80), byte(0xbf)
		switch {
		case b >= 0xc2 && b <= 0xdf:
			expected = 2
		case b >= 0xe0 && b <= 0xef:
			expected = 3
			if b == 0xe0 {
				low = 0xa0
			}
			if b == 0xed {
				high = 0x9f
			}
		case b >= 0xf0 && b <= 0xf4:
			expected = 4
			if b == 0xf0 {
				low = 0x90
			}
			if b == 0xf4 {
				high = 0x8f
			}
		}
		if expected > 0 && end < len(data) && data[end] >= low && data[end] <= high {
			end++
			for end < len(data) && end < i+expected && data[end] >= 0x80 && data[end] <= 0xbf {
				end++
			}
		}
		out.WriteRune(utf8.RuneError)
		i = end
	}
	return out.String()
}

func gs1Credentials(input string) string {
	var out strings.Builder
	colon := false
	for i := 0; i < len(input); i++ {
		b := input[i]
		if b == ':' && !colon {
			out.WriteByte(':')
			colon = true
		} else if b <= 32 || b > 126 || strings.ContainsRune("\"#<>?`{}/:;=@[\\]^|", rune(b)) {
			gs1Escape(&out, b)
		} else {
			out.WriteByte(b)
		}
	}
	return strings.TrimSuffix(out.String(), ":")
}

// Bounded RFC 3492 Punycode; checked arithmetic makes malformed ACE input a
// normal validation failure rather than an overflow, panic, or oversized loop.
func gs1PunyAdapt(delta, points uint64, first bool) uint64 {
	if first {
		delta /= 700
	} else {
		delta /= 2
	}
	delta += delta / points
	var k uint64
	for delta > 455 {
		delta /= 35
		k += 36
	}
	return k + 36*delta/(delta+38)
}
func gs1PunyDigit(d uint64) byte {
	if d < 26 {
		return 'a' + byte(d)
	}
	return '0' + byte(d-26)
}
func gs1PunyValue(c byte) (uint64, bool) {
	switch {
	case c >= 'a' && c <= 'z':
		return uint64(c - 'a'), true
	case c >= 'A' && c <= 'Z':
		return uint64(c - 'A'), true
	case c >= '0' && c <= '9':
		return uint64(c - '0' + 26), true
	}
	return 0, false
}
func gs1PunyThreshold(k, bias uint64) uint64 {
	if k <= bias {
		return 1
	}
	if k >= bias+26 {
		return 26
	}
	return k - bias
}
func gs1Add(a, b uint64) (uint64, bool) {
	if b > math.MaxUint64-a {
		return 0, false
	}
	return a + b, true
}
func gs1Mul(a, b uint64) (uint64, bool) {
	if b != 0 && a > math.MaxUint64/b {
		return 0, false
	}
	return a * b, true
}

func gs1PunycodeEncode(input string) (string, error) {
	if !utf8.ValidString(input) || utf8.RuneCountInString(input) > 1024 {
		return "", gs1InvalidURI()
	}
	points := []rune(input)
	var out strings.Builder
	for _, cp := range points {
		if cp < 128 {
			out.WriteByte(byte(cp))
		}
	}
	basic := uint64(out.Len())
	h := basic
	if basic > 0 {
		out.WriteByte('-')
	}
	n, delta, bias := uint64(128), uint64(0), uint64(72)
	for h < uint64(len(points)) {
		m := uint64(math.MaxUint64)
		for _, cp := range points {
			if uint64(cp) >= n && uint64(cp) < m {
				m = uint64(cp)
			}
		}
		product, ok := gs1Mul(m-n, h+1)
		if !ok {
			return "", gs1InvalidURI()
		}
		delta, ok = gs1Add(delta, product)
		if !ok {
			return "", gs1InvalidURI()
		}
		n = m
		for _, r := range points {
			cp := uint64(r)
			if cp < n {
				delta, ok = gs1Add(delta, 1)
				if !ok {
					return "", gs1InvalidURI()
				}
			} else if cp == n {
				q := delta
				for k := uint64(36); ; k += 36 {
					t := gs1PunyThreshold(k, bias)
					if q < t {
						break
					}
					out.WriteByte(gs1PunyDigit(t + (q-t)%(36-t)))
					q = (q - t) / (36 - t)
				}
				out.WriteByte(gs1PunyDigit(q))
				bias = gs1PunyAdapt(delta, h+1, h == basic)
				delta = 0
				h++
			}
		}
		delta, ok = gs1Add(delta, 1)
		if !ok {
			return "", gs1InvalidURI()
		}
		n, ok = gs1Add(n, 1)
		if !ok {
			return "", gs1InvalidURI()
		}
	}
	return out.String(), nil
}

func gs1PunycodeDecode(input string) (string, error) {
	if len(input) > 4096 {
		return "", gs1InvalidURI()
	}
	for i := range len(input) {
		if input[i] >= 128 {
			return "", gs1InvalidURI()
		}
	}
	out := make([]rune, 0)
	position := 0
	if d := strings.LastIndexByte(input, '-'); d >= 0 {
		out = []rune(input[:d])
		position = d + 1
	}
	n, index, bias := uint64(128), uint64(0), uint64(72)
	for position < len(input) {
		old, weight := index, uint64(1)
		for k := uint64(36); ; k += 36 {
			if position >= len(input) {
				return "", gs1InvalidURI()
			}
			digit, ok := gs1PunyValue(input[position])
			position++
			if !ok {
				return "", gs1InvalidURI()
			}
			product, ok := gs1Mul(digit, weight)
			if !ok {
				return "", gs1InvalidURI()
			}
			index, ok = gs1Add(index, product)
			if !ok {
				return "", gs1InvalidURI()
			}
			t := gs1PunyThreshold(k, bias)
			if digit < t {
				break
			}
			weight, ok = gs1Mul(weight, 36-t)
			if !ok {
				return "", gs1InvalidURI()
			}
		}
		count := uint64(len(out)) + 1
		bias = gs1PunyAdapt(index-old, count, old == 0)
		var ok bool
		n, ok = gs1Add(n, index/count)
		if !ok || n > utf8.MaxRune || n >= 0xd800 && n <= 0xdfff {
			return "", gs1InvalidURI()
		}
		index %= count
		if len(out) >= 1024 {
			return "", gs1InvalidURI()
		}
		out = append(out, 0)
		copy(out[index+1:], out[index:])
		out[index] = rune(n)
		index++
	}
	return string(out), nil
}

func gs1InRanges(cp rune, ranges []gs1IDNARange) bool {
	i := sort.Search(len(ranges), func(i int) bool { return ranges[i].last >= cp })
	return i < len(ranges) && ranges[i].first <= cp
}
func gs1ForbiddenUnicode(c rune) bool {
	return c <= 32 || c == 127 || c == utf8.RuneError || strings.ContainsRune("#/:<>?@[\\]^|%", c) || gs1InRanges(c, gs1IDNARejected[:])
}
func gs1SupportedLabel(label string) bool {
	rtlPresent, arabicPresent, asciiDigits := false, false, false
	for _, c := range label {
		if gs1ForbiddenUnicode(c) {
			return false
		}
		rtlPresent = rtlPresent || gs1InRanges(c, gs1IDNARTLLetters[:])
		arabicPresent = arabicPresent || gs1InRanges(c, gs1IDNAArabicDigits[:])
		asciiDigits = asciiDigits || c >= '0' && c <= '9'
	}
	if !rtlPresent && !arabicPresent {
		return true
	}
	first, _ := utf8.DecodeRuneInString(label)
	if !gs1InRanges(first, gs1IDNARTLLetters[:]) || strings.HasSuffix(label, "-") || arabicPresent && asciiDigits {
		return false
	}
	for _, c := range label {
		if !gs1InRanges(c, gs1IDNARTLLetters[:]) && !gs1InRanges(c, gs1IDNAArabicDigits[:]) && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}
func gs1MapHost(input string) string {
	var out strings.Builder
	out.Grow(len(input))
	for _, c := range input {
		switch {
		case c == 0x3002 || c == 0xff0e || c == 0xff61:
			c = '.'
		case c == 0xad:
			continue
		case c >= 0xff01 && c <= 0xff5e:
			c -= 0xfee0
		}
		i := sort.Search(len(gs1IDNALowercase), func(i int) bool { return gs1IDNALowercase[i].scalar >= c })
		if i < len(gs1IDNALowercase) && gs1IDNALowercase[i].scalar == c {
			out.WriteString(gs1IDNALowercase[i].value)
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}
func gs1ASCII(value string) bool {
	for i := range len(value) {
		if value[i] >= 128 {
			return false
		}
	}
	return true
}

func gs1Host(raw string) (string, error) {
	if raw == "" {
		return "", gs1InvalidURI()
	}
	if raw[0] == '[' {
		return gs1IPv6(raw)
	}
	decoded, err := gs1StrictDecode(raw, "host")
	if err != nil {
		return "", gs1InvalidURI()
	}
	mapped := gs1MapHost(decoded)
	labels := strings.Split(mapped, ".")
	for i, label := range labels {
		if !gs1SupportedLabel(label) {
			return "", gs1InvalidURI()
		}
		if strings.HasPrefix(label, "xn--") {
			decoded, err := gs1PunycodeDecode(label[4:])
			if err != nil || gs1ASCII(decoded) || !gs1SupportedLabel(decoded) || gs1MapHost(decoded) != decoded {
				return "", gs1InvalidURI()
			}
			encoded, err := gs1PunycodeEncode(decoded)
			if err != nil || "xn--"+encoded != label {
				return "", gs1InvalidURI()
			}
		} else if !gs1ASCII(label) {
			encoded, err := gs1PunycodeEncode(label)
			if err != nil {
				return "", err
			}
			labels[i] = "xn--" + encoded
		}
	}
	value := strings.Join(labels, ".")
	if value == "" {
		return "", gs1InvalidURI()
	}
	for i := range len(value) {
		b := value[i]
		if b <= 32 || b == 127 || strings.ContainsRune("#/:<>?@[\\]^|%", rune(b)) {
			return "", gs1InvalidURI()
		}
	}
	pieces := strings.Split(strings.TrimSuffix(value, "."), ".")
	last := pieces[len(pieces)-1]
	numericEnd := gs1Digits(last)
	if strings.HasPrefix(last, "0x") {
		numericEnd = true
		for i := 2; i < len(last); i++ {
			if _, ok := gs1Hex(last[i]); !ok {
				numericEnd = false
				break
			}
		}
	}
	if !numericEnd {
		return value, nil
	}
	if len(pieces) > 4 {
		return "", gs1InvalidURI()
	}
	var address uint64
	for i, piece := range pieces {
		if piece == "" {
			return "", gs1InvalidURI()
		}
		radix, body := uint64(10), piece
		if strings.HasPrefix(piece, "0x") {
			radix, body = 16, piece[2:]
		} else if len(piece) > 1 && piece[0] == '0' {
			radix, body = 8, piece[1:]
		}
		var number uint64
		for j := range len(body) {
			d, ok := gs1Hex(body[j])
			if !ok || uint64(d) >= radix {
				return "", gs1InvalidURI()
			}
			number = number*radix + uint64(d)
			if number > math.MaxUint32 {
				return "", gs1InvalidURI()
			}
		}
		if i+1 < len(pieces) {
			if number > 255 {
				return "", gs1InvalidURI()
			}
			address += number << (8 * (3 - i))
		} else {
			if number >= uint64(1)<<(8*(5-len(pieces))) {
				return "", gs1InvalidURI()
			}
			address += number
		}
	}
	return strconv.FormatUint(address>>24&255, 10) + "." + strconv.FormatUint(address>>16&255, 10) + "." + strconv.FormatUint(address>>8&255, 10) + "." + strconv.FormatUint(address&255, 10), nil
}

func gs1IPv6(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return "", gs1InvalidURI()
	}
	address, err := netip.ParseAddr(raw[1 : len(raw)-1])
	if err != nil || !address.Is6() || address.Zone() != "" {
		return "", gs1InvalidURI()
	}
	bytes := address.As16()
	var groups [8]uint16
	for i := range 8 {
		groups[i] = uint16(bytes[2*i])<<8 | uint16(bytes[2*i+1])
	}
	bestStart, bestLen := 0, 1
	for i := 0; i < 8; {
		if groups[i] != 0 {
			i++
			continue
		}
		start := i
		for i < 8 && groups[i] == 0 {
			i++
		}
		if i-start > bestLen {
			bestStart, bestLen = start, i-start
		}
	}
	var out strings.Builder
	out.WriteByte('[')
	for i := 0; i < 8; {
		if bestLen > 1 && i == bestStart {
			out.WriteString("::")
			i += bestLen
		} else {
			if i > 0 && !(bestLen > 1 && i == bestStart+bestLen) {
				out.WriteByte(':')
			}
			out.WriteString(strconv.FormatUint(uint64(groups[i]), 16))
			i++
		}
	}
	out.WriteByte(']')
	return out.String(), nil
}

func gs1NormalizePath(path string, base bool) (string, error) {
	if strings.Count(path, "/") > GS1MaxElements {
		return "", gs1Failure("GS1 Digital Link path component count exceeds limit")
	}
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	primarySeen := false
	for i, part := range parts {
		if !base && gs1Primary(part) {
			primarySeen = true
		}
		dot := strings.ReplaceAll(strings.ReplaceAll(part, "%2e", "."), "%2E", ".")
		if !primarySeen && (dot == "." || dot == "..") {
			if dot == ".." && len(out) > 1 {
				out = out[:len(out)-1]
			}
			if i+1 == len(parts) {
				out = append(out, "")
			}
		} else {
			out = append(out, part)
		}
	}
	result := strings.Join(out, "/")
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	return gs1Encode(result, 2), nil
}

func gs1ParseURL(input string, base bool) (gs1URL, error) {
	if err := gs1Text(input, "GS1 Digital Link URI"); err != nil {
		return gs1URL{}, err
	}
	clean := strings.TrimFunc(input, func(c rune) bool { return c <= 0x20 })
	clean = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(clean)
	colon := strings.IndexByte(clean, ':')
	if colon <= 0 || !gs1ASCIIAlpha(clean[0]) {
		return gs1URL{}, gs1InvalidURI()
	}
	for i := 0; i < colon; i++ {
		if !gs1ASCIIAlpha(clean[i]) && !gs1ASCIIDigit(clean[i]) && !strings.ContainsRune("+.-", rune(clean[i])) {
			return gs1URL{}, gs1InvalidURI()
		}
	}
	u := gs1URL{scheme: strings.ToLower(clean[:colon])}
	rest := clean[colon+1:]
	if hash := strings.IndexByte(rest, '#'); hash >= 0 {
		fragment := rest[hash+1:]
		u.fragment = &fragment
		rest = rest[:hash]
	}
	if question := strings.IndexByte(rest, '?'); question >= 0 {
		query := gs1Encode(rest[question+1:], 3)
		u.query = &query
		rest = rest[:question]
	}
	if u.scheme != "http" && u.scheme != "https" && u.scheme != "ftp" && u.scheme != "ws" && u.scheme != "wss" {
		u.path = rest
		return u, nil
	}
	rest = strings.TrimLeft(strings.ReplaceAll(rest, "\\", "/"), "/")
	authority, path := rest, "/"
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		authority, path = rest[:slash], rest[slash:]
	}
	if authority == "" {
		return gs1URL{}, gs1InvalidURI()
	}
	user := ""
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		user, authority = gs1Credentials(authority[:at]), authority[at+1:]
		if user != "" {
			user += "@"
		}
	}
	hostname, port := authority, ""
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return gs1URL{}, gs1InvalidURI()
		}
		hostname = authority[:end+1]
		suffix := authority[end+1:]
		if suffix != "" {
			if suffix[0] != ':' {
				return gs1URL{}, gs1InvalidURI()
			}
			port = suffix[1:]
		}
	} else if end := strings.IndexByte(authority, ':'); end >= 0 {
		hostname, port = authority[:end], authority[end+1:]
	}
	if port != "" {
		if !gs1Digits(port) {
			return gs1URL{}, gs1InvalidURI()
		}
		var number uint64
		for i := range len(port) {
			number = number*10 + uint64(port[i]-'0')
			if number > 65535 {
				return gs1URL{}, gs1InvalidURI()
			}
		}
		if (u.scheme == "http" || u.scheme == "ws") && number == 80 || (u.scheme == "https" || u.scheme == "wss") && number == 443 || u.scheme == "ftp" && number == 21 {
			port = ""
		} else {
			port = ":" + strconv.FormatUint(number, 10)
		}
	}
	host, err := gs1Host(hostname)
	if err != nil {
		return gs1URL{}, err
	}
	u.authority = user + host + port
	u.path, err = gs1NormalizePath(path, base)
	if err != nil {
		return gs1URL{}, err
	}
	return u, nil
}

func gs1CheckURI(u gs1URL, base bool) error {
	if u.scheme != "http" && u.scheme != "https" {
		return gs1Failure("GS1 Digital Link URI must use http or https")
	}
	if base {
		if u.query != nil && *u.query != "" || u.fragment != nil && *u.fragment != "" {
			return gs1Failure("GS1 Digital Link baseUrl must not include query or fragment components")
		}
	} else if u.fragment != nil && *u.fragment != "" {
		return gs1Failure("GS1 Digital Link URI must not include a fragment")
	}
	return nil
}
