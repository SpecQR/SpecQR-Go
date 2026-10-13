# Development-only verification

The subject is a separately compiled, dependency-free Go executable.

Python が比較処理を進め、Node が固定した JavaScript/Nayuki 参照を実行し、3 つの独立 decoder が検証対象の実際の matrix と PNG を調べます。これらの tool や package は、候補の QR symbol を生成しません。開発用 adapter は `tools/conformance` にあり、依存関係を追加せず、製品用 API や CLI には含まれません。

## Reproduce

Go 1.26.8 以降（Go 1.27.1 でも検証済み）、Python 3.11+、Node、Java 17+（Java decoder 用）を使用してください。
GS1 の監査には、公式 Node 24.19.0 と 24.21.0 の正確な runtime が必要です。major version だけでなく、Ada/ICU/Unicode の版も検査します。

```sh
npm ci --prefix tools/verification --ignore-scripts
git clone https://github.com/SpecQR/SpecQR.git ../SpecQR-baseline
git -C ../SpecQR-baseline checkout 15ad15e5c770ea0e39072f8f88b2733018f02ffd
python3 tools/verification/verify_conformance.py --baseline ../SpecQR-baseline
python3 tools/verification/verify_structured_append.py --baseline ../SpecQR-baseline
python3 tools/verification/verify_jsqr.py
python3 tools/verification/verify_decoders.py --decoder cpp --dependency-dir .tools/zxing-cpp
python3 tools/verification/verify_decoders.py --decoder java --dependency-dir .tools/zxing-java
python3 tools/verification/verify_gs1.py --baseline ../SpecQR-baseline --node /path/node-24.19.0 --node-profile 24.19.0 --output artifacts/gs1-24.19.json
python3 tools/verification/verify_gs1.py --baseline ../SpecQR-baseline --node /path/node-24.21.0 --node-profile 24.21.0 --output artifacts/gs1-24.21.json
```

各候補 process は `go build -trimpath -buildvcs=false
-o <temporary> ./tools/conformance` で offline build し、一時領域へ保存します。`--candidate` は source tree、`--go` は Go、`--binary` は指定したビルド済み開発用 adapter そのものを選びます。最後のオプションでは、指定した成果物を source build に置き換えません。module-consumer lane では、source archive を clean directory に展開し、`--candidate` で選択してください。

`SPECQR_GO` は既定の Go コマンドを設定します。`SPECQR_DEV_NODE_MODULES` は、`package.json` と `node_modules` を含む別の開発用 package directory を指定できます。ZXing-C++ の公式 PyPI wheel の hash と、ZXing Java JAR の SHA-256 を使用前に検査します。`--no-install` には、検証済みの decoder ファイルが既存であることが必要です。
Go module に外部 module 依存はありません。Node/Python/Java package は、分離された開発専用の独立参照・decoder です。
レポートの既定の出力先は `artifacts/` です。`--output` で外部の場所を選択できます。

## Exact coverage

- 固定した JS をその場で実行して比較する 3,028 個の public matrix。pad 済み data と interleave 済み data/ECC codeword の全体、version/ECC/mask、matrix の全 module に対する SHA-256 を含みます。全 1,280 通りの version 1–40 × ECC × mask の組合せ、mixed/manual mode、Unicode、Kanji、binary、自動最適化・mask、ECI 境界、FNC1 の両位置、GS1、boost、決定的な fuzz を対象にします。
- 2,400 個の独立検査した Nayuki matrix。固定した version/ECC/mask の全組合せを含みます。独立した segmentation/mask の選択を、説明なく同等として扱いません。
- 4,320 個の raw-pattern matrix と全 mask score、65,536 個の GF 積、Reed–Solomon generator/remainder の全 degree 1–255。
- 640 個の正確な容量、1,920 個の境界見積り、22 個の Structured Append set と 112 個の構成 matrix、同一 process 内で並行生成する 256 個の matrix、18 個の不正入力に対する型付き拒否。別の SA lane では、diagnostics の全体、split offset/unit、codeword を比較し、総数 2–16 のすべてを対象とする 30 個の順序を入れ替えた text/binary merge と、11 個の不正 merge を、その場で実行する JavaScript と照合します。
- jsQR: 232 個の matrix と 232 個の実 PNG の検出、全 version、64 個の ECI header。jsQR は FNC1/SA に対応しないため、この lane でそれらの成功は主張しません。
- ZXing-C++: 706 個の matrix と 750 個の PNG の検出、135 個の SA header、manual と高レベルの両経路における有効な FNC1-second indicator 全 152 個、ECI bytes/text、5 個の完全な高レベル set（44 symbol）、独立した復元。
- ZXing Java: 446 個の matrix と 446 個の PNG の検出、全 version、135 個の SA header、両経路で復元する 5 個の完全な set、各 case で正確に 3 codeword の訂正を要求する 32 個の破損 symbol テスト。

独立した検出の前に、実際の Go PNG を復号したすべての RGBA pixel と quiet-zone pixel を検査します。ZXing Java の厳密な PNG 検出は scale 3 を使い、`PURE_BARCODE` や matrix への fallback は使いません。scale 8 の挙動は、全 446 symbol を対象にした別の診断です。442 個が成功し、4 個が失敗し、すべての失敗を同じ pixel を持つ独立 PNG control でも再現します。scale 8 のレポートは失敗のまま保持します。`--png-scale 8` でこの別診断を再現でき、`summarize_default_scale.py` は古い結果や説明のない失敗を拒否します。検出失敗を成功数へ加算しません。C++ は既定 scale 8 を検査します。3 つの decoder すべてが、実際の空白画像を拒否する必要があります。

## Exact GS1 profiles

各 Node profile は、全 5,610 case / 15,690 operation を、固定した JavaScript をその場で実行して照合します。限定 AI カタログの全体、diagnostics、不正な percent escape、IPv4/IPv6、authority/path/query の処理、URL 作成を含みます。
`gs1/reference-url-outcomes.json` は、正確な両 Node runtime の全 54 IDNA/dot case を固定します。`gs1/go-url-outcomes.json` は、独立に構築した Go の期待結果 55 個（同じ 54 case と、UTF-16 専用の 1 入力）を記録します。重複する dot 作成 case は、元の corpus における重複数を保持します。先行する他言語の候補結果を Go の期待値として取り込みません。`gs1/go_profile.py` は明示的な規則と Python 標準ライブラリの RFC3492 から有限の期待値を導出し、IDNA/dot の広い分類全体を例外にはしません。
未知の差、合計数が同じでも変化した値、古い fixture は監査失敗とします。

Go の URL 実装は範囲を限定し、外部依存を持ちません。Unicode 15.0.0 に固定した小文字化・拒否テーブル、全角 ASCII と代替 dot の対応付け、soft-hyphen の除去、RFC3492 Punycode、厳密な ACE の往復変換検証を使います。未対応の結合文字・format・未割当文字、互換正規化で変化する scalar、Hangul Jamo は拒否します。保守的な RTL 検査は、mixed letter/digit conventions を拒否します。別の 39 種の host 変異を含む corpus では、これらの明示的な方針に対して 78 request / 156 operation を検査し、その場で実行した Node との差分も記録します。
完全な UTS46、NFC、全 Unicode に対するブラウザーとの URL 同等性は主張しません。GS1 の dot-only 値は意図的に保持し、query の値として出力します。
Go の厳密な JSON 入力境界は、corpus 内の該当する 1 入力を置換せずに拒否します。レポートには、この入力領域の違いを明示します。Node 24.21 が不正な ASCII ACE label を受理する挙動は Go に引き継ぎません。

## Subject identity and mutation controls

レポートには、source/test hash、正確な Go process の PID/nonce/executable、独立に検証した executable の SHA-256、外部依存のない Go module と native executable の metadata、参照の commit/source/corpus hash、decoder/package fingerprint を含めます。source、executable、依存関係、tool が変わると、その run は無効になります。厳密な response 数の確認により、途中で切れた結果を成功扱いにしません。`--suite public` または `internal` に範囲を限定した結果は、不完全と明示します。

6 種の adapter 故障は、最初に実際の Go を実行してから、matrix hash・data byte・interleave 済み codeword の破損、終了、response の欠落、エラー出力を起こします。さらに 3 種の故障で、型のない失敗を型付き検証として数えられないことを確認します。SA と merge の故障は、metadata と復元した payload を破損させます。GS1 control は、実際の Go カタログ出力、参照 URL データ、Go の dot URL を破損させ、最後のものでは集計上の差の分類を保持します。これらの control は開発用 adapter の中だけに置き、製品用ライブラリや通常の CLI には含めません。

これらは有限の合成テストです。ISO/GS1 認証、印刷物・camera のテスト、任意の破損や入力への保証ではありません。packaging、compiler/OS の組合せ、clean な offline consumer は別の検査です。

## Provenance

開発用 harness は、MIT ライセンスの SpecQR-Rust と SpecQR-Kotlin をもとにしています。その workflow は、SpecQR-Java `1f2bf277a582f0eb4e31c9975443c1a76552aa23`、SpecQR-Python `1deb5cf83ed7a93eeb44138e5980a963a00cdffb`、SpecQR-CPP `e91cd8fe4434fd6ef10d1126bb2fd17b0b19321f`、SpecQR-CSharp `057c4b3f25e52c4786a8f94c744ff884eedcecfa` に由来します。Java decoder は元の SpecQR-Swift の帰属表示を保持しています。これらはテスト workflow の派生元です。Go の検証対象は、先行する他言語の実装、encoder、decoder をリンクしません。

## Offline module consumer

`build_consumer.py --candidate <unpacked-module> --consumer-dir <empty-directory>
--output <receipt.json>` は独立 module を作成します。そこへコピーした開発用 adapter は、公開 SpecQR-Go API だけを import します。receipt には Go build-info、正確な replacement path、compiler、adapter/source hash、executable identity を記録します。
`--binary` を使い、その成果物そのものに対して 7 つの strict lane をすべて実行してください。source の再ビルドへ置き換えません。このローカルの offline-consumer 検査は公開後の module download とは別であり、公開元から取得したことを主張しません。

`run_all.py` は 7 つの strict lane を実行し、失敗、範囲限定、古い結果、source 不一致のレポートを拒否してから要約を作成します。compiler と decoder の executable は fingerprint を記録し、外部 package は固定して検査します。release-evidence run の実行中は、source や検証用ファイルを変更しないでください。
