# GS1 と Digital Link

SpecQR-Go は SpecQR JavaScript revision `15ad15e5c770ea0e39072f8f88b2733018f02ffd` の限定 AI カタログ、要素文字列、検証、Digital Link を実装します。GS1 General Specifications / Digital Link の全範囲を扱う製品や認証済み実装ではありません。対応 AI は 50 項目。日付 AI は桁数・数字のみ、GLN は桁数・数字のみを検証し、日付や業務上の有効性は判定しません。

```go
package main

import (
    "fmt"
    specqr "github.com/SpecQR/SpecQR-Go"
)

func main() {
    elements := []specqr.GS1Element{
        {AI: "01", Value: "09506000134352"},
        {AI: "10", Value: "LOT-123"},
    }
    data, err := specqr.GS1ToElementString(elements)
    if err != nil { panic(err) }
    fmt.Println(data)
    link, err := specqr.GS1CreateDigitalLink(elements, specqr.GS1DigitalLinkOptions{
        BaseURL: "https://id.gs1.org",
    })
    if err != nil { panic(err) }
    fmt.Println(link)
}
```

## API

- `GS1GetSupportedAIs()`、`GS1GetAIInfo(ai)`：対応カタログの独立したコピー。変更しても以後の検証に影響しません
- `GS1CalculateCheckDigit`、`GS1ValidateCheckDigit`：任意の数字列の mod-10
- `GS1CalculateGTINCheckDigit`、`GS1AppendGTINCheckDigit`、`GS1ValidateGTINCheckDigit`：GTIN の対応桁数を検証
- `GS1CalculateSSCCCheckDigit`、`GS1AppendSSCCCheckDigit`、`GS1ValidateSSCCCheckDigit`：SSCC の桁数を検証
- `GS1FromHumanReadable`、`GS1ToHumanReadable`、`GS1ToElementString`、`GS1ElementStringToHumanReadable`
- `GS1ParseElementString`、`GS1ValidateElementString`、`GS1ValidateElements`
- `GS1CreateDigitalLink`、`GS1ParseDigitalLink`、`GS1ValidateDigitalLink`、`GS1NormalizeDigitalLink`

検証・解析・正規化の任意オプションは、末尾に一つだけ渡せます。`GS1ValidationOptions` の `CollectAllErrors` は `nil` なら全件、`*bool(false)` なら最初のエラーで停止します。`Context` の空値は `element-string`。`digital-link` は primary AI の存在も検証します。`AllowUnsupportedAI: true` は未実装の AI を黙認せず、オプションエラーにします。

`GS1DigitalLinkOptions` の `BaseURL` は作成時に必須です。`PrimaryAI` の空値は作成時 `01`、解析時は `00` / `01` / `414` の最初の AI。`PathAIs: nil` は対応 qualifier を path に配置し、空の非 nil スライスはすべて query に配置します。`UnknownQuery` の空値は `preserve`、明示値は `preserve` / `reject`。`Mode` の空値は `specqr-deterministic`。`Normalize: true` を検証関数へ渡すと未対応オプションとして報告します。

戻り値は利用者が所有する構造体・スライスです。カタログ内部の可変状態を返しません。失敗は `*specqr.Error` の `InvalidGS1`、非例外の検証は `OK` と構造化 `Errors` / `Warnings`。診断の文字数・offset は JavaScript に合わせ UTF-16 code units を数えます。Go の UTF-8 不正文字列は明示的に拒否します。静的な Go 引数では JavaScript の null/object 型誤用を表せないため、その範囲の架空の互換性は主張しません。

## 要素文字列

値は printable ASCII の範囲で、括弧と GS (`U+001D`) を除きます。この制約は限定 GS1 カタログの値に対するものです。一般の QR payload の Unicode 対応を狭めません。`%` は通常のデータ文字として保存します。QR の GS1 モードでは `%` の特殊な alphanumeric 解釈を避ける必要があり、高水準生成 API がその mode safety を扱います。

固定長・可変長・必要な FNC1 separator を検証します。末尾の可変長データの suffix が固定長要素として読める場合は、元 SpecQR 同様、separator の欠落の疑いとして拒否します。対応表を超えた AI を推測しません。

## URL 処理

URL を取得したり DNS 解決したりしません。外部依存を追加せず、Go 標準ライブラリとローカル処理で HTTP/HTTPS、credential escaping、ASCII authority、IPv4 数値表記、IPv6、port、base path、query、fragment を扱います。Go の `net/url` と WHATWG URL の差を避けるため、特別 URL 用の限定 adapter を使います。一般の妥当な ASCII URL を広く拒否する「安全策」は取りません。

path の不正 percent escape / 不正 UTF-8 はエラーです。query は URLSearchParams の form decoding に合わせ `+`、percent escape と UTF-8 replacement decoding を扱います。`GS1ParseDigitalLink` は未知 query を保存できますが、`GS1ValidateDigitalLink` / `GS1NormalizeDigitalLink` は URL 全体の percent escape 構文も検査します。`http` と未知 query の保存は、それぞれ警告です。

GS1 の値 `.` / `..` は URL の dot segment と区別します。parse は primary AI 以降の値を保持し、create / normalize は実際の dot-only 値を query に移します。`%2e` というリテラル値は `%252e` にエスケープして保存します。通常のブラウザーの path 正規化で GS1 データが失われるのを防ぐための、元 JS に対する明示的な差分です。base path の通常の dot segment は正規化します。

## Unicode hostname の限定プロファイル

Go 標準ライブラリには完全な IDNA 実装がないため、Unicode 15.0.0 に固定した独立プロファイルと RFC 3492 Punycode を実装しています。小文字化、全角 ASCII、代替 dot、soft hyphen の削除に対応します。対応外の結合文字・format/control・未割当・separator・互換正規化で変化する文字・文脈 NFC を要する Hangul Jamo などは拒否します。RTL は先頭が RTL 文字、末尾 hyphen 不可、RTL 文字と一種類の数字表記・内部 hyphen という保守的な範囲に限定します。

これは完全な NFC、UTS #46、contextual/bidi 処理ではありません。拒否された domain が IDNA 一般で無効とは限りません。たとえば分解済み `e + U+0301` を含む host は拒否し、合成済み `é` は受理します。完全な正規化で同じ domain になる場合でも、本 API の受理範囲には差があります。A-label は復号して同じプロファイルを検証し、canonical Punycode の再符号化とも比較します。不正・非 canonical な ACE をそのまま成功させません。

可能なら ASCII hostname を使ってください。信頼できる完全な IDNA 実装で前処理した A-label でも、この限定プロファイルがすべて受理する保証はありません。未対応 domain が必要なら、適切な URL ライブラリを持つアプリケーション側で URL を確定し、一般の QR 生成へ渡せます。その場合は GS1 helper の検証済みとは扱わないでください。

表と小文字化は Go の Unicode 更新、OS、locale に依存しません。`tools/generate_idna_profile.py` で Python 標準ライブラリの Unicode 15.0.0 から再現生成できます。データには [Unicode ライセンス](../LICENSE-UNICODE) を同梱しています。ジェネレーターの semantic checksum と full scalar sweep が意図しない変更を検出します。

## 上限・検証

入力と出力は最大 1,000,000 UTF-16 code units、要素・path/query component は最大 16,384、要素集合には aggregate text work budget を適用します。Unicode label は最大 1,024 scalars、ACE の符号列には別の上限があります。整数処理は overflow を検査します。巨大な末尾可変要素の separator 検査は、固定長 AI に必要な短い suffix だけを候補にするので二次時間になりません。

`go test .` はカタログ独立性、チェックディジット、診断、要素 round trip、dot 値、URL/IP/ACE、resource limit、mutation、140,954 個の受理 Unicode scalar host の安定した再解析を確認します。`-short` は全 scalar sweep を省略します。`FuzzGS1Helpers` と `FuzzGS1URLAdapter` は失敗入力で panic しないことと成功時の再解析安定性を検査します。

`tools/url-differential` は 10,000 ケースの通常 URL を exact pinned Node 24.19.0 / 24.21.0 の両方と比較します。GS1 corpus ではこの 2 バージョンで host 受理範囲が異なるため、各 profile の実測出力と差分 identity を固定し、未知差分を単なる件数の allowlist で隠しません。最新の実行件数・結果は [verification-summary.json](verification-summary.json) を参照してください。

今回の固定 corpus は 5,610 ケース / 15,690 操作です。Node 24.19.0 では 15,647 操作が一致し、43 差分は dot 値保全の 40 操作と lone UTF-16 surrogate の 3 操作。Node 24.21.0 では 15,620 操作が一致し、同じ差分に 9 種類の edge / malformed ACE host × 3 操作の保守的拒否を加えた 70 差分です。Go 自身の実測出力に基づく 55 fixture 行（51 unique cases）を個別照合します。さらに 39 hostname variants / 78 requests / 156 operations の別 corpus が、有効な Unicode host でも限定プロファイルが拒否する例を明示します。全 scalar の accepted-output 安定性は任意 Unicode 文字列の完全対応を証明しません。

cross-runtime corpus は受理・拒否、値、診断 code / reason / AI / offset / expected などを比較し、人間向けエラー本文は照合対象外です。本文の代表例は Go の単体テストで確認します。静的 Go オプションのゼロ値は既定値を意味するため、JavaScript の「省略」と「明示的な空文字列」を区別する binding ではありません。型による差と hostname / payload 差を混同しないでください。

### Validation Context の適用先

`GS1ValidationOptions.Context` の `digital-link` による primary-key 関係検査は `GS1ValidateElements` で適用します。`GS1ValidateElementString` は owned JavaScript baseline と同じく element-string の構文・値検査を行い、Context による Digital Link 関係検査は適用しません。URL の検査には `GS1ValidateDigitalLink` を使ってください。

## URL serialization compatibility (2026-10-05)

base URL の空 fragment `#` は作成時に保持します。正規化は従来どおり空 fragment を除去し、非空 fragment は拒否します。 限定した URL 出力互換性の拡張であり、通常の QR 符号化・公開 API・runtime dependency は変更しません。既存の dot 値・NUL・IDNA・診断方針を保持します。[固定 corpus と再現手順](../tools/url-serialization/README.md) を参照してください。
