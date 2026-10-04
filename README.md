# SpecQR Go

A from-scratch, standard-library-only QR Code Model 2 encoder for Go. No external modules, CGO, native QR bindings, or runtime downloads.

## 概要

SpecQR の Go 版です。最小 Go **1.26.8**、検証対象 Go **1.26.8 / 1.27.1**。両系統は [Go のサポート方針](https://go.dev/doc/devel/release)に基づき選定しています。Windows / Linux / macOS、32-bit Linux を CI の対象にします。ライブラリは標準 Go だけで動作し、CLI も同じ公開 API を使います。

**0.1.0-rc.1 相当の初期プレリリース**です。両 Go 版で Linux amd64 と Windows amd64 の native test・race・CLI・package gate が通過しました。macOS arm64 も本体コードが同一の候補で通過し、その後の変更はレビュー済みのテスト期待値修正だけです。Linux 386 は QEMU による通常実行が通過しましたが、標準ライブラリも含めた unoptimized debug の root suite は両版で失敗しています。この初期公開では、32-bit で標準ライブラリ・runtime まで最適化を無効にする構成を既知の制限として開示します。全面的な検証完了や native Linux386 の成功を主張しません。cross-compile やエミュレーションを native pass と表示しません。安定版タグ・リリース・別レジストリへの公開は行っていません。詳細は [検証範囲と既知の制限](docs/verification.md) を参照してください。

- QR Code Model 2、version 1–40、ECC L/M/Q/H、mask 0–7 と自動選択
- 数字、英数字、UTF-8、任意のバイト列、漢字モード
- 最適混在セグメント、明示的セグメント、ECI、FNC1 第1/第2位置
- 容量、生成前の計画・見積り、選択理由と読取り上の診断
- 限定 GS1 AI カタログ、チェックデジット、element string / Digital Link
- Structured Append の parity、2–16 分割、完全セットの検証・結合
- PNG、SVG、RGBA pixels、data URL

## インストールと最小例

Go module と CLI を直接取得できます。再現性が必要な場合は `@main` ではなく利用するコミットを指定してください。source archive 内では `go test ./...` / `go install ./cmd/specqr` を使えます。

```sh
go get github.com/SpecQR/SpecQR-Go@main
go install github.com/SpecQR/SpecQR-Go/cmd/specqr@main
```

```go
package main

import (
    "os"
    specqr "github.com/SpecQR/SpecQR-Go"
)

func main() {
    q, err := specqr.Generate("Hello, 日本語", specqr.DefaultOptions())
    if err != nil { panic(err) }
    png, err := q.ToPNG()
    if err != nil { panic(err) }
    if err := os.WriteFile("qr.png", png, 0644); err != nil { panic(err) }
}
```

利用アプリではエラーを処理してください。例中の `panic` は呼出側を短くするためのもので、ライブラリが不正入力に panic を返す契約ではありません。

```go
options := specqr.DefaultOptions()
options.ECC = specqr.Q
options.Version = 5
mask := 3
options.Mask = &mask // nil は自動、0 も明示指定可能
plan, err := specqr.Estimate("12345日本語", options)
if err != nil { /* 入力・設定エラー */ }
if plan.OK() {
    q, err := specqr.Generate("12345日本語", options)
    _, _ = q, err
}
```

`Options{}` は M / 自動 / 最適化 / 4-module quiet zone / scale 8 を選びます。`DefaultOptions()` から変更する形を推奨します。`RenderOptions{}` 全体は既定値ですが、非ゼロの設定では `Scale` は正数が必要です。`Margin: 0` は、既定値から変更するか正の Scale を併記して明示できます。

## CLI

Go 標準 `flag` の規則により、フラグは位置引数より前に書きます。`-text` と位置引数は同時指定できません。

```sh
specqr -text 'Hello 日本語' -format png -output qr.png
specqr -ecc Q -version 5 -mask 3 -format svg -output qr.svg '12345'
specqr -input payload.bin -binary -format png -output bytes.png
specqr -segments examples/segments.json -format json -output diagnostics.json
specqr -text '10LOT%ABC' -gs1 -format png -output gs1.png
specqr -help
```

出力は `svg` / `png` / `json` / `matrix` / `svg-data-url` / `png-data-url`。入力・出力の `-` は stdin / stdout です。Structured Append は `-structured-append -format json` を使い、各シンボルの matrix と診断を取得します。JSON の数値範囲、不正 UTF-8、孤立 surrogate、null、重複キー、モードと合わないフィールドを拒否します。入力・生成エラーは stderr の JSON と終了コード 2 です。Go flag parser の構文エラー（不明フラグや整数の形式違反など）は標準のテキストエラーを返します。

## 安全性と決定性

- `*specqr.Error` と `ErrorCode` で入力・容量・資源上限エラーを区別
- 入力文字列は厳密な UTF-8。任意の不正 UTF-8 バイト列には `GenerateBytes` を使用
- 結果の matrix / codewords / pixels / diagnostics / options は防御的コピー。生成済み結果は並行読取り可能
- 呼出中の入力 slice や Options の pointer を別 goroutine から変更しないこと
- 単一データ入力 1,000,000 units、手動 16,384 segments、raster 4 Mi-pixels、SVG 8 MiB、data URL 32 MiB の上限
- 最適化対象は 7,089 scalars 以下。収容不能な巨大入力は行列を作らず、見積りまたは型付きエラーで扱う
- 同じ入力・設定の matrix / codewords / pixels は決定的。PNG 圧縮バイトの異なる Go バージョン間での一致は保証しない

## 重要な境界

高レベル GS1 / FNC1 で literal `%` を含む文字列は byte モードで保ちます。英数字モードを強制すると拒否します。低レベルの手動 FNC1 英数字 segment は QR の escaping をそのまま扱い、`%` は区切り、`%%` は literal `%` です。

GS1 は 50 AI の限定カタログです。完全な GS1 規格、全 AI、認証を主張しません。Digital Link の `.` / `..` だけの値は消さず、query value として保ちます。URL hostname は固定 Unicode 15 の明示的な範囲を持ち、完全な UTS #46 / NFC / WHATWG 実装ではありません。これは一般の Unicode QR payload の制限ではありません。

ZXing Java の厳密な PNG lane は scale 3 で 446/446 です。既定 scale 8 の別診断は **442/446** で、4 件は同じ画素の独立 PNG control でも失敗します。失敗を成功数に含めません。ZXing-C++ は既定 scale 8 の 750/750 を検証します。読取り機器ごとの互換性と印刷条件を確認してください。

Windows 386 の unoptimized debug 実行には未解明の不安定性を記録しています。Intel Windows 上の候補の normal/debug suites は両 Go 版で通過しましたが、別の標準ライブラリ `compress/flate` debug control は両版で失敗しました。また ARM64 Windows の x86 エミュレーションでは Go 1.26.8 の候補の PNG 圧縮経路で失敗しています。原因や修正済みを断定せず、これらの失敗を保持します。PNG は標準ライブラリの BestCompression を使用するため、影響がないとも断定しません。

## 文書と検証

- [API](docs/api.md)
- [GS1 と URL profile](docs/gs1.md)
- [Structured Append](docs/structured-append.md)
- [検証の方法・範囲](docs/verification.md)
- [現在のプラットフォーム検証記録](docs/platform-status.json)
- [機械可読の独立検証](docs/verification-summary.json)
- [独立レビュー](docs/review-summary.json)

```sh
go test ./...
go test -race ./...
go vet ./...
python tools/verify_package.py
```

外部 QR / decoder は `tools/verification` の開発用 oracle だけです。Go module に runtime / test 用の外部 Go module はありません。所有者の JS baseline は `15ad15e5c770ea0e39072f8f88b2733018f02ffd` に固定し、matrix・全 data/ECC codeword・容量・GS1/SA 診断を比較します。Nayuki と実 decoder は独立 lane で実行します。

## ライセンス

原著 Go コードは MIT。生成 Unicode データは Unicode License v3 を保持します。[LICENSE](LICENSE)、[LICENSE-UNICODE](LICENSE-UNICODE)、[NOTICE](NOTICE) を参照してください。第三者 QR 実装のソースを runtime にコピーしていません。
