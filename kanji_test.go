package specqr

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
)

func TestCanonicalKanjiMapping(t *testing.T) {
	if len(kanjiMap) != 6953 {
		t.Fatal(len(kanjiMap))
	}
	h := sha256.New()
	previous := uint16(0)
	for _, pair := range kanjiMap {
		if pair[0] <= previous {
			t.Fatal("unsorted map")
		}
		previous = pair[0]
		raw := make([]byte, 4)
		binary.BigEndian.PutUint16(raw, pair[0])
		binary.BigEndian.PutUint16(raw[2:], pair[1])
		h.Write(raw)
		code, ok := kanjiCode(rune(pair[0]))
		if !ok || code != int(pair[1]) {
			t.Fatal(pair)
		}
		value, e := KanjiValue(rune(pair[0]))
		if e != nil || value < 0 || value > 8191 {
			t.Fatal(pair, value, e)
		}
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != "be0a434df7babc88848f0930ceb3ea9424c6715d89ed4b1b19bcb416f9892fa8" {
		t.Fatal("canonical mapping changed", got)
	}
	for _, r := range []rune{'A', '🙂', -1, 0xd800, 0x110000} {
		if CanEncodeKanji(r) {
			t.Fatal(r)
		}
		if _, e := KanjiValue(r); e == nil {
			t.Fatal(r)
		}
	}
}
