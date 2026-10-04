package specqr

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestGenerateOwnershipAndConcurrency(t *testing.T) {
	mask := 2
	o := DefaultOptions()
	o.Mask = &mask
	data := []byte("immutable bytes")
	q, e := GenerateBytes(data, o)
	if e != nil {
		t.Fatal(e)
	}
	want := q.Matrix()
	data[0] = 0
	mask = 7
	m := q.Matrix()
	m[0][0] = !m[0][0]
	d := q.DataCodewords()
	d[0] ^= 255
	cw := q.Codewords()
	cw[0] ^= 255
	ss := q.Segments()
	ss[0] = Segment{}
	di := q.Diagnostics()
	di["segments"].([]any)[0].(map[string]any)["mode"] = "changed"
	saved := q.Options()
	*saved.Mask = 5
	if q.Mask() != 2 || q.Matrix()[0][0] != want[0][0] || q.Diagnostics()["segments"].([]any)[0].(map[string]any)["mode"] != "byte" {
		t.Fatal("ownership failed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			for j := 0; j < 8; j++ {
				a, e := Generate("日本語と123ABC", Options{})
				if e != nil || a.Size() < 21 {
					t.Errorf("concurrent generation: %v", e)
				}
				_, e = q.ToPNG()
				if e != nil {
					t.Error(e)
				}
				q.Diagnostics()
				q.Matrix()
			}
		})
	}
	wg.Wait()
}
func TestPlansCapacityAndTypedErrors(t *testing.T) {
	for v := 1; v <= 40; v++ {
		for _, ecc := range []ECC{L, M, Q, H} {
			for _, mode := range []Mode{Numeric, Alphanumeric, Byte, Kanji} {
				c, e := GetCapacity(v, ecc, mode, 0)
				if e != nil {
					t.Fatal(e)
				}
				n := *c.Maximum()
				var text string
				switch mode {
				case Numeric:
					text = strings.Repeat("1", n)
				case Alphanumeric:
					text = strings.Repeat("A", n)
				case Byte:
					text = strings.Repeat("a", n)
				case Kanji:
					text = strings.Repeat("漢", n)
				}
				o := DefaultOptions()
				o.Version = v
				o.ECC = ecc
				o.Mode = mode
				p, e := Estimate(text, o)
				if e != nil || !p.OK() {
					t.Fatalf("capacity %d %s %s: %v", v, ecc, mode, e)
				}
				p, e = Estimate(text+string([]rune(text)[0]), o)
				if e != nil {
					t.Fatal(e)
				}
				if p.OK() {
					t.Fatalf("capacity+1 unexpectedly fits %d %s %s", v, ecc, mode)
				}
			}
		}
	}
	bad := []Options{{ECC: "bad"}, {Version: -1}, {MinVersion: 20, MaxVersion: 10}, {Mask: new(-1)}, {ECI: new(1_000_000)}, {GS1: true, ECI: new(0)}, {PrintDPI: math.NaN()}, {PrintDPI: math.Inf(1)}, {Render: RenderOptions{Margin: -1, Scale: 8}}, {Render: RenderOptions{Margin: 4, Scale: 0}}}
	for _, o := range bad {
		_, e := Generate("x", o)
		var typed *Error
		if !errors.As(e, &typed) {
			t.Fatalf("expected typed error: %#v %v", o, e)
		}
	}
	if _, e := Generate("\xff", Options{}); e == nil {
		t.Fatal("invalid UTF8 accepted")
	}
	if _, e := GenerateBytes([]byte{255, 0, 128}, Options{}); e != nil {
		t.Fatal(e)
	}
	p, e := Estimate(strings.Repeat("9", 8000), Options{})
	if e != nil || p.OK() || p.Version() != 0 || p.OverflowBits() <= 0 {
		t.Fatalf("overflow plan: %v %v", p, e)
	}
}
func TestFNC1PercentSafety(t *testing.T) {
	indicator := "12"
	for _, o := range []Options{{FNC1Second: &indicator}, {GS1: true}} {
		text := "ABC%DEF"
		if o.GS1 {
			text = "10ABC%DEF"
		}
		q, e := Generate(text, o)
		if e != nil {
			t.Fatal(e)
		}
		if q.Segments()[1].Mode() != Byte || q.Segments()[1].Text() != text {
			t.Fatal("percent changed")
		}
		o.Mode = Alphanumeric
		if _, e := Generate(text, o); e == nil {
			t.Fatal("unsafe explicit alphanumeric accepted")
		}
	}
}
func TestRenderRoundTripLimits(t *testing.T) {
	q, e := Generate("PNG 日本語", Options{})
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultRenderOptions()
	o.Foreground = "#1234"
	o.Background = "transparent"
	p, e := ToPixels(q.Matrix(), o)
	if e != nil {
		t.Fatal(e)
	}
	b, e := ToPNG(q.Matrix(), o)
	if e != nil {
		t.Fatal(e)
	}
	im, e := png.Decode(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	pix := p.Bytes()
	for y := 0; y < p.Height(); y++ {
		for x := 0; x < p.Width(); x++ {
			r, g, bb, a := im.At(x, y).RGBA()
			i := (y*p.Width() + x) * 4
			alpha := uint32(pix[i+3])
			if a != alpha*257 || r != uint32(pix[i])*257*alpha/255 || g != uint32(pix[i+1])*257*alpha/255 || bb != uint32(pix[i+2])*257*alpha/255 {
				t.Fatalf("RGBA mismatch %d,%d", x, y)
			}
		}
	}
	pix[0] ^= 1
	if bytes.Equal(pix, p.Bytes()) {
		t.Fatal("pixels not copied")
	}
	url, e := q.ToPNGDataURL()
	if e != nil {
		t.Fatal(e)
	}
	raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = png.Decode(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	for _, ro := range []RenderOptions{{Margin: 4, Scale: math.MaxInt}, {Margin: math.MaxInt, Scale: 8}, {Margin: 4, Scale: -1}, {Margin: -1, Scale: 8}} {
		if _, e := ToPixels(q.Matrix(), ro); e == nil {
			t.Fatal("overflow geometry accepted")
		}
	}
	// SVG uses int64 geometry: either MaxInt field alone can remain valid on
	// 32-bit hosts. Combining them exceeds the 53-bit limit on both widths.
	for _, ro := range []RenderOptions{{Margin: math.MaxInt, Scale: math.MaxInt}, {Margin: 4, Scale: -1}, {Margin: -1, Scale: 8}} {
		if _, e := ToSVG(q.Matrix(), ro); e == nil {
			t.Fatal("overflow SVG accepted")
		}
	}
	wideSVG, e := ToSVG([][]bool{{true}}, RenderOptions{Margin: 1 << 30, Scale: 8})
	if e != nil || !strings.Contains(wideSVG, `width="17179869192"`) || !strings.Contains(wideSVG, "M8589934592,8589934592h8v8h-8z") {
		t.Fatal("valid int64 SVG geometry rejected or truncated")
	}
	if _, e := ToPNG([][]bool{{true, false}}, o); e == nil {
		t.Fatal("ragged matrix accepted")
	}
	o.Foreground = "\" onload=\"alert(1)"
	svg, e := ToSVG(q.Matrix(), o)
	if e != nil || strings.Contains(svg, "fill=\"\" onload=") {
		t.Fatal("SVG escaping failed")
	}
	o.Foreground = "\x00"
	if _, e = ToSVG(q.Matrix(), o); e == nil {
		t.Fatal("invalid XML accepted")
	}
}
func FuzzPublicAPINoPanic(f *testing.F) {
	for _, s := range []string{"", "abc", "日本語", "10A%B", "\xff"} {
		f.Add(s, 1, 0)
	}
	f.Fuzz(func(t *testing.T, s string, v, mask int) {
		if len(s) > 256 {
			t.Skip()
		}
		o := DefaultOptions()
		o.Version = v
		o.Mask = &mask
		q, e := Generate(s, o)
		if e != nil {
			var typed *Error
			if !errors.As(e, &typed) {
				t.Fatalf("untyped %T", e)
			}
			return
		}
		if q.Size() != q.Version()*4+17 {
			t.Fatal("size")
		}
		if _, e = q.ToPNG(); e != nil {
			t.Fatal(e)
		}
	})
}
