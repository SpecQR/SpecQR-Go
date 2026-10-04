# Go API

## 生成・計画

`Generate(text string, Options)`、`GenerateBytes([]byte, Options)`、`GenerateSegments([]Segment, Options)` は `(*QRCode, error)`。対応する `Estimate` / `EstimateBytes` / `AnalyzeSegments` は `(*Plan, error)` を返します。計画は ECC・行列・描画を作りません。

`Plan.OK()` は指定範囲で収容できるかを表します。自動選択が失敗した場合 `Version()` は 0、`CapacityVersion()` は見積りに使った最大 version です。`RequiredBits()`、`CapacityBits()`、`RemainingBits()`、`OverflowBits()`、`Segments()`、`Diagnostics()` で結果を調べます。固定 version の範囲優先順位は `Version` が `MinVersion` / `MaxVersion` に優先します。

`QRCode` は `Version()` / `Size()` / `ECC()` / `Mask()` / `Matrix()` / `Module(x,y)` / `DataCodewords()` / `Codewords()` / `ErrorCorrectionCodewords()` / `Segments()` / `Options()` / `Diagnostics()` を提供します。`DataCodewords()` は interleave 前の pad 済みデータ、`Codewords()` は data block と ECC block を interleave した列です。`ErrorCorrectionCodewords()` は最終列の ECC 部分です。座標は 0-based、範囲外は型付きエラーです。

`GetCapacity(version, ecc, mode, controlBits)` は単一 segment の容量です。Auto / 空 mode は mode 固有値を持たない総容量を返します。`Maximum()` / `MaxBytes()` / `MaxCharacters()` は `*int`、該当しない値は nil です。byte の容量は文字数でなく符号化済みバイト数です。controlBits は非負のプラットフォーム int、最大 2^53−1 に制限されます。32-bit では int の範囲がさらに狭くなります。

## Options

- ECC: `L`, `M`, `Q`, `H`。空は M
- Mode: `Auto`, `Numeric`, `Alphanumeric`, `Byte`, `Kanji`。空は Auto
- Version: 0 は自動、1..40 は固定
- MinVersion / MaxVersion: 0 はそれぞれ 1 / 40
- Mask: nil は自動、pointer の値は 0..7
- DisableOptimization: true で混在最適化を無効化
- BoostECC: version を増やさず収容可能な最大 ECC を選択
- ECI: nil または 0..999999 の pointer。ECI は raw bytes の意味を宣言し、変換はしない
- GS1: raw element string を検証し FNC1 第1位置を付加
- FNC1Second: 2 桁数字または ASCII Latin 1 文字の pointer
- StructuredAppend: 正しい header segment の pointer
- Render / PrintDPI: 描画と印刷診断。PrintDPI 0 は未指定、正の有限値のみ

高レベル ECI / GS1 / FNC1 second / SA header は同時指定できません。ECI 付きの自動最適化は Kanji を選びません。手動 ECI 変更は、その mode の低レベル規則に従って明示できます。

## Segment

各 constructor は `(Segment, error)`。

`NumericSegment(string)`、`AlphanumericSegment(string)`、`UTF8Segment(string)`、`BytesSegment([]byte)`、`KanjiSegment(string)`、`ECISegment(int)`、`FNC1Segment()`、`FNC1SecondSegment(string)`、`StructuredAppendSegment(index,total,parity)`。

Segment のゼロ値は無効です。内部は immutable。raw bytes は constructor 時に所有するコピーへ変換します。`LogicalBytes()` は UTF-8 または元の raw bytes で、Kanji の Shift_JIS codeword ではありません。`Text()` は textual data、`Data()` は detached bytes、`IsBinary()` で区別できます。`Count()` は QR count field の単位、`CharacterCount()` と `ByteCount()` は診断用です。

`CreateSegments` / `CreateSegmentsWithKanji`、`BitLength`、`SegmentsBits`、`CountBits`、`PayloadBitLength`、`ValidateSegments` も公開しています。計画用 `BitLength` は大きい count でも算術長を返し、実ビット生成は count field の overflow を拒否します。

## 描画

QRCode の `ToPNG()` / `ToSVG()` / `ToPixels()` / `ToPNGDataURL()` / `ToSVGDataURL()` は保存した RenderOptions を使います。関数版は `(matrix [][]bool, options RenderOptions)` を受け、1..177 の正方形行列を検証します。

Raster 色は `#RGB` / `#RGBA` / `#RRGGBB` / `#RRGGBBAA` / black / white / transparent。SVG は XML 属性を escape した CSS 色も出力します。外部由来の CSS 色の意味まで検証する sanitizer ではありません。

Pixels は非 premultiplied RGBA の行優先配列です。`Width()` / `Height()` / `Bytes()` を使います。PNG は stdlib `image/png` の BestCompression。異なる Go release 間で圧縮結果の一致を保証せず、画素一致を検証します。SVG data URL も base64 です。

## エラー・並行利用

```go
var e *specqr.Error
if errors.As(err, &e) {
    fmt.Println(e.Code, e.Message)
}
```

ErrorCode は INVALID_INPUT / INVALID_MODE / INVALID_VERSION / INVALID_ECC / INVALID_ECI / INVALID_GS1 / INVALID_STRUCTURED_APPEND / INVALID_COLOR / DATA_TOO_LONG / RESOURCE_LIMIT。無効な入力は回復可能なエラーです。入力を同時変更する race は呼出側の責任です。結果の returned slices/maps を変更しても元の結果は変わりません。
