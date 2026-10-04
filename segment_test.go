package specqr

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func segmentMust(t *testing.T, s Segment, e error) Segment {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSegmentPayloadsAndOwnership(t *testing.T) {
	data := []byte{0, 1, 255}
	s, e := BytesSegment(data)
	if e != nil {
		t.Fatal(e)
	}
	data[0] = 7
	got := s.Data()
	got[1] = 7
	if !bytes.Equal(s.Data(), []byte{0, 1, 255}) {
		t.Fatal("mutable ownership")
	}
	if !s.IsBinary() || s.Count() != 3 || s.CharacterCount() != 0 {
		t.Fatal("byte diagnostics")
	}
	u, e := UTF8Segment("🙂")
	if e != nil || u.Count() != 4 || u.CharacterCount() != 1 || u.ByteCount() != 4 || u.IsBinary() {
		t.Fatal("UTF8 diagnostics", e)
	}
	k, e := KanjiSegment("漢字")
	if e != nil || k.Count() != 2 || k.ByteCount() != 4 || !bytes.Equal(k.LogicalBytes(), []byte("漢字")) {
		t.Fatal("kanji diagnostics", e)
	}
	invalid := []string{string([]byte{255}), string([]byte{0xc0, 0x80}), string([]byte{0xed, 0xa0, 0x80})}
	for _, text := range invalid {
		if _, e := UTF8Segment(text); e == nil {
			t.Fatal("ill-formed UTF8")
		}
		if _, e := CreateSegments(text, 1, Auto, true); e == nil {
			t.Fatal("ill-formed optimized text")
		}
	}
}
func TestControlWireBits(t *testing.T) {
	for _, assignment := range []int{0, 127, 128, 16383, 16384, 999999} {
		s, e := ECISegment(assignment)
		if e != nil {
			t.Fatal(e)
		}
		bits, e := s.Bits(1)
		if e != nil {
			t.Fatal(e)
		}
		want := 12
		if assignment >= 128 {
			want = 20
		}
		if assignment >= 16384 {
			want = 28
		}
		if len(bits) != want || !bytes.Equal(bits[:4], []byte{0, 1, 1, 1}) {
			t.Fatal(assignment, bits)
		}
	}
	for _, x := range []int{-1, 1000000} {
		if _, e := ECISegment(x); e == nil {
			t.Fatal(x)
		}
	}
	for _, tc := range []struct {
		s    string
		code int
	}{{"00", 0}, {"99", 99}, {"A", 165}, {"z", 222}} {
		s, e := FNC1SecondSegment(tc.s)
		if e != nil || s.ApplicationIndicatorCodeword() != tc.code {
			t.Fatal(tc, e)
		}
		bits, e := s.Bits(1)
		if e != nil || len(bits) != 12 {
			t.Fatal(e)
		}
		v := 0
		for _, b := range bits[4:] {
			v = v*2 + int(b)
		}
		if v != tc.code {
			t.Fatal(tc, v)
		}
	}
	for _, x := range []string{"", "1", "100", "é", "AA"} {
		if _, e := FNC1SecondSegment(x); e == nil {
			t.Fatal(x)
		}
	}
	sa, e := StructuredAppendSegment(2, 3, 0xab)
	if e != nil {
		t.Fatal(e)
	}
	bits, e := sa.Bits(1)
	if e != nil || !bytes.Equal(bits, []byte{0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 1}) {
		t.Fatal(bits, e)
	}
	for _, p := range [][3]int{{0, 2, 0}, {3, 2, 0}, {1, 1, 0}, {1, 17, 0}, {1, 2, 256}} {
		if _, e := StructuredAppendSegment(p[0], p[1], p[2]); e == nil {
			t.Fatal(p)
		}
	}
}
func TestManualControlsAndEmpty(t *testing.T) {
	fnc, _ := FNC1Segment()
	second, _ := FNC1SecondSegment("A")
	eci, _ := ECISegment(26)
	sa, _ := StructuredAppendSegment(1, 2, 0)
	a, _ := AlphanumericSegment("100%%")
	for _, seq := range [][]Segment{{fnc, eci}, {second, eci}, {sa, eci}, {a, fnc}, {fnc, fnc}, {Segment{}}} {
		if e := ValidateSegments(seq); e == nil {
			t.Fatal("bad controls accepted", seq)
		}
	}
	if e := ValidateSegments([]Segment{eci, a, eci}); e != nil {
		t.Fatal(e)
	}
	if e := ValidateSegments([]Segment{fnc, a}); e != nil {
		t.Fatal(e)
	}
	if a.Text() != "100%%" {
		t.Fatal("manual escaped percent changed")
	}
	for _, optimize := range []bool{true, false} {
		ss, e := CreateSegments("", 1, Auto, optimize)
		if e != nil || len(ss) != 1 || ss[0].Mode() != Byte || ss[0].Count() != 0 || ss[0].IsBinary() {
			t.Fatal(ss, e)
		}
	}
	if _, e := (Segment{}).Bits(1); e == nil {
		t.Fatal("zero segment accepted")
	}
}
func TestSegmentBitWidthsAndCaps(t *testing.T) {
	cases := []struct {
		mode  Mode
		text  string
		width int
	}{{Numeric, strings.Repeat("1", 1023), 10}, {Alphanumeric, strings.Repeat("A", 511), 9}, {Byte, strings.Repeat("a", 255), 8}, {Kanji, strings.Repeat("漢", 255), 8}}
	for _, tc := range cases {
		s, e := textSegment(tc.mode, tc.text)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := s.Bits(1); e != nil {
			t.Fatal(tc.mode, e)
		}
		extra := "a"
		if tc.mode == Numeric {
			extra = "1"
		} else if tc.mode == Alphanumeric {
			extra = "A"
		} else if tc.mode == Kanji {
			extra = "漢"
		}
		s, e = textSegment(tc.mode, tc.text+extra)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := s.BitLength(1); e != nil {
			t.Fatal("planning should remain arithmetic", e)
		}
		if _, e := s.Bits(1); e == nil {
			t.Fatal("overflow count accepted", tc.mode)
		}
	}
	if _, e := BytesSegment(make([]byte, MaxPayloadUnits+1)); e == nil {
		t.Fatal("byte cap")
	}
	if _, e := UTF8Segment(strings.Repeat("a", MaxPayloadUnits+1)); e == nil {
		t.Fatal("text cap")
	}
	if _, e := CreateSegments(strings.Repeat("1", MaxSingleSymbolCharacters+1), 40, Auto, true); e == nil {
		t.Fatal("optimizer cap")
	}
	a, _ := UTF8Segment(strings.Repeat("a", 500001))
	if e := ValidateSegments([]Segment{a, a}); e == nil {
		t.Fatal("aggregate cap")
	}
	fnc, _ := FNC1Segment()
	seq := make([]Segment, MaxManualSegments+1)
	for i := range seq {
		seq[i] = fnc
	}
	if e := ValidateSegments(seq); e == nil {
		t.Fatal("segment cap")
	}
	if _, e := SegmentsBits([]Segment{a}, 40); e == nil {
		t.Fatal("materialization cap")
	}
}
func bruteCost(text string, version int, allowKanji bool) int {
	r := []rune(text)
	dp := make([]int, len(r)+1)
	for i := 1; i <= len(r); i++ {
		dp[i] = int(^uint(0) >> 1)
		for start := 0; start < i; start++ {
			piece := string(r[start:i])
			for _, mode := range dataModes {
				if mode == Kanji && !allowKanji {
					continue
				}
				s, e := textSegment(mode, piece)
				if e != nil {
					continue
				}
				n, _ := s.BitLength(version)
				dp[i] = min(dp[i], dp[start]+n)
			}
		}
	}
	return dp[len(r)]
}
func TestOptimalSegmentationExhaustive(t *testing.T) {
	alphabet := []rune{'1', 'A', 'a', '漢', '🙂'}
	for _, version := range []int{1, 10, 27} {
		for _, allowKanji := range []bool{true, false} {
			for n := 1; n <= 4; n++ {
				total := 1
				for i := 0; i < n; i++ {
					total *= len(alphabet)
				}
				for seed := 0; seed < total; seed++ {
					chars := make([]rune, n)
					q := seed
					for i := range chars {
						chars[i] = alphabet[q%len(alphabet)]
						q /= len(alphabet)
					}
					text := string(chars)
					segments, e := CreateSegmentsWithKanji(text, version, Auto, true, allowKanji)
					if e != nil {
						t.Fatal(e)
					}
					cost, e := BitLength(segments, version)
					if e != nil || cost != bruteCost(text, version, allowKanji) {
						t.Fatal(text, version, allowKanji, cost, e)
					}
					var rebuilt strings.Builder
					for _, s := range segments {
						rebuilt.WriteString(s.Text())
					}
					if rebuilt.String() != text {
						t.Fatal("lost characters")
					}
					tracker, e := NewOptimizationTracker(version, allowKanji)
					if e != nil {
						t.Fatal(e)
					}
					prefix := ""
					for _, c := range chars {
						prefix += string(c)
						n, e := tracker.Append(c)
						if e != nil || n != bruteCost(prefix, version, allowKanji) {
							t.Fatal("prefix differs", e)
						}
					}
				}
			}
		}
	}
}
func TestOptimizationDeterminismAndTracker(t *testing.T) {
	text := "漢字ABC12345678901234567890xyz🙂"
	a, e := CreateSegments(text, 10, Auto, true)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10; i++ {
		b, e := CreateSegments(text, 10, Auto, true)
		if e != nil || !reflect.DeepEqual(a, b) {
			t.Fatal("nondeterminism")
		}
	}
	var tracker OptimizationTracker
	if _, e := tracker.Append('a'); e == nil {
		t.Fatal("zero tracker")
	}
	tr, _ := NewOptimizationTracker(1, true)
	for _, r := range []rune{-1, 0xd800, utf8.MaxRune + 1} {
		if _, e := tr.Append(r); e == nil {
			t.Fatal(r)
		}
	}
	if _, e := CreateSegments("A", 1, Mode("bad"), true); e == nil {
		t.Fatal("invalid mode")
	}
	if _, e := CreateSegments("🙂", 1, Kanji, true); e == nil {
		t.Fatal("invalid Kanji")
	}
	_, e = UTF8Segment(string([]byte{0xff}))
	var qerr *Error
	if !errors.As(e, &qerr) || qerr.Code != InvalidInput {
		t.Fatal(e)
	}
}
func FuzzSegmentsPublicInputs(f *testing.F) {
	f.Add("ABC漢字123", 1, "auto", true)
	f.Add(string([]byte{255}), -1, "byte", false)
	f.Fuzz(func(t *testing.T, text string, version int, mode string, optimize bool) {
		if len(text) > 30000 {
			return
		}
		s, e := CreateSegments(text, version, Mode(mode), optimize)
		if e == nil {
			_, _ = BitLength(s, version)
			_, _ = SegmentsBits(s, version)
		}
		_, _ = NumericSegment(text)
		_, _ = AlphanumericSegment(text)
		_, _ = UTF8Segment(text)
		_, _ = KanjiSegment(text)
		_, _ = ECISegment(version)
		_, _ = FNC1SecondSegment(text)
		_, _ = StructuredAppendSegment(version, len(text), len(mode))
	})
}
