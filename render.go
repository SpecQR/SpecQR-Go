package specqr

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Render resource limits apply before output allocation.
const (
	RasterPixelBudget            = 4 * 1024 * 1024
	SVGCharacterBudget           = 8 * 1024 * 1024
	DataURLCharacterBudget       = 32 * 1024 * 1024
	MaxGeometryInteger     int64 = 1<<53 - 1
)

// RenderOptions selects quiet-zone modules, pixel scale, and CSS/hex colors.
// Use DefaultRenderOptions and change fields when explicitly requesting margin 0.
type RenderOptions struct {
	Margin, Scale          int
	Foreground, Background string
}

func DefaultRenderOptions() RenderOptions { return RenderOptions{4, 8, "#000000", "#ffffff"} }
func normalizeRender(o RenderOptions) RenderOptions {
	if o == (RenderOptions{}) {
		return DefaultRenderOptions()
	}
	if o.Foreground == "" {
		o.Foreground = "#000000"
	}
	if o.Background == "" {
		o.Background = "#ffffff"
	}
	return o
}

// Pixels is a detached immutable unpremultiplied RGBA raster.
type Pixels struct {
	width, height int
	data          []byte
}

func (p *Pixels) Width() int    { return p.width }
func (p *Pixels) Height() int   { return p.height }
func (p *Pixels) Bytes() []byte { return append([]byte(nil), p.data...) }
func validMatrix(m [][]bool) error {
	if len(m) < 1 || len(m) > 177 {
		return errCode(InvalidInput, "matrix dimension must be 1..177")
	}
	for _, r := range m {
		if len(r) != len(m) {
			return errCode(InvalidInput, "matrix must be square")
		}
	}
	return nil
}
func copyMatrix(m [][]bool) [][]bool {
	out := make([][]bool, len(m))
	for i, r := range m {
		out[i] = append([]bool(nil), r...)
	}
	return out
}
func renderDimension(m [][]bool, o RenderOptions, raster bool) (int64, error) {
	if e := validMatrix(m); e != nil {
		return 0, e
	}
	if o.Margin < 0 || o.Scale <= 0 {
		return 0, errCode(InvalidInput, "margin must be nonnegative and scale positive")
	}
	margin, scale := int64(o.Margin), int64(o.Scale)
	if margin > (MaxGeometryInteger-int64(len(m)))/2 {
		return 0, errCode(ResourceLimit, "render geometry exceeds exact integer limit")
	}
	modules := int64(len(m)) + 2*margin
	if scale > MaxGeometryInteger/modules {
		return 0, errCode(ResourceLimit, "render geometry exceeds exact integer limit")
	}
	n := modules * scale
	if raster && (n > 2048 || n*n > RasterPixelBudget) {
		return 0, errCode(ResourceLimit, "render exceeds 4 Mi-pixel budget")
	}
	return n, nil
}

// ParseColor accepts hexadecimal RGB/RGBA, black, white, and transparent.
func ParseColor(value string) ([4]byte, error) {
	v := strings.TrimSpace(value)
	switch strings.ToLower(v) {
	case "black":
		return [4]byte{0, 0, 0, 255}, nil
	case "white":
		return [4]byte{255, 255, 255, 255}, nil
	case "transparent":
		return [4]byte{}, nil
	}
	c := [4]byte{0, 0, 0, 255}
	if len(v) < 1 || v[0] != '#' {
		return c, errCode(InvalidColor, "raster color must be hex, black, white or transparent")
	}
	v = v[1:]
	if len(v) != 3 && len(v) != 4 && len(v) != 6 && len(v) != 8 {
		return c, errCode(InvalidColor, "invalid hexadecimal color")
	}
	step := 2
	if len(v) <= 4 {
		step = 1
	}
	for i := 0; i < len(v)/step; i++ {
		a, e := strconv.ParseUint(v[i*step:(i+1)*step], 16, 8)
		if e != nil {
			return c, errCode(InvalidColor, "invalid hexadecimal color")
		}
		if step == 1 {
			a *= 17
		}
		c[i] = byte(a)
	}
	return c, nil
}

// ContrastRatio computes uncomposited RGB contrast; alpha is reported separately.
func ContrastRatio(a, b [4]byte) float64 {
	lum := func(c [4]byte) float64 {
		v := 0.0
		for i, w := range []float64{.2126, .7152, .0722} {
			x := float64(c[i]) / 255
			if x <= .03928 {
				x /= 12.92
			} else {
				x = math.Pow((x+.055)/1.055, 2.4)
			}
			v += w * x
		}
		return v
	}
	x, y := lum(a), lum(b)
	return (math.Max(x, y) + .05) / (math.Min(x, y) + .05)
}

// ToPixels renders a caller-provided square matrix to bounded RGBA.
func ToPixels(m [][]bool, options RenderOptions) (*Pixels, error) {
	o := normalizeRender(options)
	dimension, e := renderDimension(m, o, true)
	if e != nil {
		return nil, e
	}
	fg, e := ParseColor(o.Foreground)
	if e != nil {
		return nil, e
	}
	bg, e := ParseColor(o.Background)
	if e != nil {
		return nil, e
	}
	n := int(dimension)
	data := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		my := y/o.Scale - o.Margin
		for x := 0; x < n; x++ {
			mx := x/o.Scale - o.Margin
			c := bg
			if my >= 0 && mx >= 0 && my < len(m) && mx < len(m) && m[my][mx] {
				c = fg
			}
			copy(data[(y*n+x)*4:], c[:])
		}
	}
	return &Pixels{n, n, data}, nil
}

// ToPNG uses Go's standard image/png encoder. Pixels are stable across toolchains;
// compressed bytes are only promised deterministic for the same Go version.
func ToPNG(m [][]bool, o RenderOptions) ([]byte, error) {
	p, e := ToPixels(m, o)
	if e != nil {
		return nil, e
	}
	im := &image.NRGBA{Pix: p.data, Stride: p.width * 4, Rect: image.Rect(0, 0, p.width, p.height)}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if e = enc.Encode(&b, im); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func escapeXML(v string) (string, error) {
	if !utf8.ValidString(v) {
		return "", errCode(InvalidColor, "SVG color must be UTF-8")
	}
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#x27;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			if r != '\t' && r != '\n' && r != '\r' && (r < 32 || r == 0xfffe || r == 0xffff) {
				return "", errCode(InvalidColor, "invalid XML color character")
			}
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

// ToSVG renders escaped SVG using integer geometry without raster allocation.
func ToSVG(m [][]bool, options RenderOptions) (string, error) {
	o := normalizeRender(options)
	n, e := renderDimension(m, o, false)
	if e != nil {
		return "", e
	}
	if len(o.Foreground) > SVGCharacterBudget/12 || len(o.Background) > SVGCharacterBudget/12 {
		return "", errCode(ResourceLimit, "SVG colors exceed output budget")
	}
	fg, e := escapeXML(o.Foreground)
	if e != nil {
		return "", e
	}
	bg, e := escapeXML(o.Background)
	if e != nil {
		return "", e
	}
	var b strings.Builder
	b.Grow(512 + len(fg) + len(bg) + len(m)*len(m)*30)
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\" role=\"img\"><rect width=\"100%%\" height=\"100%%\" fill=\"%s\"/><path fill=\"%s\" d=\"", n, n, n, n, bg, fg)
	for y, row := range m {
		for x, d := range row {
			if d {
				fmt.Fprintf(&b, "M%d,%dh%dv%dh-%dz", (int64(x)+int64(o.Margin))*int64(o.Scale), (int64(y)+int64(o.Margin))*int64(o.Scale), o.Scale, o.Scale, o.Scale)
			}
		}
	}
	b.WriteString("\"/></svg>")
	if b.Len() > SVGCharacterBudget {
		return "", errCode(ResourceLimit, "SVG exceeds output budget")
	}
	return b.String(), nil
}
func renderDataURL(mime string, data []byte) (string, error) {
	if len(data) > (DataURLCharacterBudget-64)*3/4 {
		return "", errCode(ResourceLimit, "data URL exceeds output budget")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
func ToPNGDataURL(m [][]bool, o RenderOptions) (string, error) {
	b, e := ToPNG(m, o)
	if e != nil {
		return "", e
	}
	return renderDataURL("image/png", b)
}
func ToSVGDataURL(m [][]bool, o RenderOptions) (string, error) {
	s, e := ToSVG(m, o)
	if e != nil {
		return "", e
	}
	return renderDataURL("image/svg+xml", []byte(s))
}
