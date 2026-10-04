package specqr

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestAllTablesPinnedJavaScript(t *testing.T) {
	h := sha256.New()
	for v := 1; v <= 40; v++ {
		for _, ecc := range []ECC{L, M, Q, H} {
			b, e := BlockInfo(v, ecc)
			if e != nil {
				t.Fatal(e)
			}
			a, e := AlignmentPositions(v)
			if e != nil {
				t.Fatal(e)
			}
			str := make([]string, len(a))
			for i, n := range a {
				str[i] = fmt.Sprint(n)
			}
			fmt.Fprintf(h, "%d,%s,%d,%d,%d,%d,%s\n", v, ecc, b.Blocks, b.ECCPerBlock, b.RawCodewords, b.DataCodewords, strings.Join(str, ":"))
		}
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != "2f617f80f458cc973713cc8c9370b68b69bd1e5d6cd2d7b4bca714f9d83fe194" {
		t.Fatalf("pinned JS tables digest: %s", got)
	}
}
func TestTableBoundaries(t *testing.T) {
	for _, tc := range []struct {
		v int
		p []int
	}{{1, []int{}}, {2, []int{6, 18}}, {7, []int{6, 22, 38}}, {32, []int{6, 34, 60, 86, 112, 138}}, {40, []int{6, 30, 58, 86, 114, 142, 170}}} {
		got, e := AlignmentPositions(tc.v)
		if e != nil || !reflect.DeepEqual(got, tc.p) {
			t.Fatal(tc.v, got, e)
		}
	}
	for _, tc := range []struct {
		mode   Mode
		counts [3]int
	}{{Numeric, [3]int{10, 12, 14}}, {Alphanumeric, [3]int{9, 11, 13}}, {Byte, [3]int{8, 16, 16}}, {Kanji, [3]int{8, 10, 12}}} {
		for i, v := range []int{9, 10, 27} {
			got, e := CountBits(tc.mode, v)
			if e != nil || got != tc.counts[i] {
				t.Fatal(tc.mode, v, got, e)
			}
		}
	}
	for _, v := range []int{-1, 0, 41} {
		if _, e := RawCodewordCount(v); e == nil {
			t.Fatal(v)
		}
		if _, e := CountBits(Byte, v); e == nil {
			t.Fatal(v)
		}
		if _, e := AlignmentPositions(v); e == nil {
			t.Fatal(v)
		}
	}
	if _, e := CountBits(Auto, 1); e == nil {
		t.Fatal("auto accepted")
	}
	if _, e := DataCodewordCount(1, ECC("bad")); e == nil {
		t.Fatal("bad ecc")
	}
	if n, e := DataCodewordCount(40, L); e != nil || n != 2956 {
		t.Fatal(n, e)
	}
}
