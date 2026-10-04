# 検証範囲と既知の制限

これは2026-10-04時点の公開前検証記録です。初期公開では通常ビルドの検証結果を採用し、32-bit で標準ライブラリ・runtime まで最適化を無効にした構成の既知の失敗を開示します。失敗した gate を通過に変更せず、native Linux386 や全構成を検証済みとは表示しません。公開 commit の CI と公開 module の取得確認は、この公開前の記録と別の証拠です。

## 状態

| Gate | 状態 |
| --- | --- |
| Linux amd64 Go 1.26.8 / 1.27.1 の unit/API/example/CLI | 修正後候補で通過 |
| Linux amd64 normal / unoptimized debug / race / vet / CGO-free | 修正後候補で通過 |
| 独立 source 7 lanes | 修正後候補、Go 1.26.8 で通過 |
| 独立 detached package/module consumer 各 7 lanes | 修正後候補、Go 1.27.1 で通過 |
| Windows amd64 両 Go の native tests / race / CLI / package | Intel Windows で各 10 gates と独立 review race が通過 |
| Windows 386 両 Go の normal / unoptimized debug suites | Intel Windows の WOW64 で各 suite が通過。別診断の失敗は下記に保持 |
| macOS arm64 両 Go の native tests / race / CLI / package | 元のテスト snapshot で通過。本体・module・tooling は同一、テストだけの変更を別途レビュー |
| Linux 386 の native tests / CLI | **未実行**。cross-compile のみ。現在のホストでは exec format error |
| Linux 386 の QEMU user-mode 実行、両 Go 版 | normal suites・CLI・example・別module consumer・独立 review/geometry tests は通過。root-package unoptimized debug は両版失敗 |
| public GitHub CI / fresh public clone / pinned public module consumer | **未実行**。公開前の必要 gate の後に実施 |

Cross-compilation は build portability の証拠です。OS の runtime / filesystem / shell / package 依存動作の証拠ではありません。エミュレーションを native pass と表示しません。Linux386 は公式 Debian QEMU 10.0.13 の user-mode で実際に実行しました。native Linux386 の確認には32-bit実行をサポートする Linux executor が必要です。

現在の集約記録は [platform-status.json](platform-status.json) です。元の `verification-summary.json`、`local-platform-validation.json`、`review-summary.json` の実行時の hash / 結果は履歴として保持します。Go/mod fingerprint は元の `af5539ed5e47abd6d436960fe887f44d9344f884f467efb479f3314777c221ce` から、`api_test.go` だけの修正で `752ed151c2f95eea6836432e12dd4c72c544ef87bcf235c0f4a74909efb13266` になりました。SVG の int64 境界と raster 上限を分けるテスト修正で、本体・依存・tooling の変更はありません。元の Mac native 実行はそのまま本体の証拠として使いますが、新しいテストの期待値まで Mac で再実行したとは表示しません。文書更新による source archive の変更も、過去の archive hash に遡って反映しません。

## Windows 386 debug の診断

Intel Windows amd64 上の WOW64 では、候補の normal / unoptimized debug を両 Go 版・amd64/386 で実行した全8 suites が通過しました。一方、別の全 `compress/flate` debug control は amd64 で両版通過、386 で両版失敗です。Go 1.26.8 は `TestBestSpeed` 内の `io.ReadAll` で nil pointer、Go 1.27.1 は GC / `sigtramp` の fatal traceback を記録しました。

さらに以前の ARM64 Windows guest の x86 エミュレーションでは、Go 1.26.8 の候補が `ToPNG → image/png (level 9) → zlib → flate → compressor.deflate` で失敗しています。Intel 上の成功でこの失敗を消しません。原因を Go、Windows、WOW64、仮想化、hardware のいずれかと断定せず、ARM エミュレーションだけの問題や新しい Go で修正済みとも主張しません。

本体 PNG の BestCompression 経路は新しい `TestBestSpeed` の失敗箇所そのものではありませんが、同じ標準ライブラリと runtime を使用します。PNG への影響がないという保証にはできません。これらは対象条件を限定した既知の診断結果です。候補の通過結果と分離して開示し、標準ライブラリ全 test の成功を後から別の公開条件として加えるものではありません。

## Linux 386 のエミュレーション検証

公式 Debian `qemu-user` 10.0.13 の package checksum を照合し、作業用 directory に展開しました。ホストへの install、binfmt 登録、security 設定の変更は行っていません。各 target は静的リンクの ELF32 / i386 / CGO 無効を確認し、明示的に `qemu-i386` で実行しました。

両 Go 版で通常の package suites、実 CLI の text / arbitrary bytes と PNG、example、別 module consumer が通過しました。既存の独立6テストと、SVG の正確な整数境界を含む geometry tests は normal / debug とも通過しました。通常の root suite は top-level 47 件通過、外部 Node reference を必要とする opt-in `TestGS1URLDifferential` だけが未設定で skip です。この追加 URL lane は今回の32-bit実行には含めず、別の pinned Node を使った独立7 lanes の結果と混同しません。PNG pixel checks は候補の matrix との整合性確認で、独立 QR decoder の代わりではありません。386 の race detector は非対応であり、native amd64 の別の race 証拠を emulated race pass に数えません。

`-gcflags=all="-N -l"` で本体と標準ライブラリを最適化無効にした root suite は、Go 1.26.8 で並行 PNG 圧縮中に `compress/flate` の nil dereference / `index > windowEnd`、Go 1.27.1 で新規 local struct slice の代入中に nil dereference を記録しました。後者の fault PC は `runtime.bulkBarrierPreWrite` に対応します。これは障害の位置であり原因の断定ではありません。CLI / conformance の debug package tests は通過しましたが、途中で停止した root suite の残りを通過扱いにしません。失敗した候補 suite の再試行や runtime knob の調整は行わず、全結果を保持しています。

続いて SpecQR を import しない固定量の標準ライブラリ control を、両 Go 版 × QEMU386 / native amd64 の計4条件で各1回実行しました。Go 1.26.8 の QEMU386 は PNG/flate で `fillWindow called with stale data` を記録し、後続の struct 代入 phase に到達しませんでした。他の3条件は128回の PNG encode・8,921,088 pixel checks・262,160 struct checksを完了しました。SpecQR がなくても類似の最小版 PNG 障害が起きる証拠ですが、元の障害と同一原因とも、Go 1.27.1 の候補の障害が解消したとも断定しません。control の allocation pressure は明示的な近似です。原因不明のまま圧縮方式や最小 Go 版を変更せず、[全条件の結果](linux386-emulation.json)を保持しています。

## 正しさの独立検証

`tools/verification/README.md` に再現方法を記載しています。Go candidate は source または指定された native executable として動作し、独立 PID / nonce / SHA-256 / Go build-info / module graph に結び付けます。reference の結果を candidate として返しません。

- 3,028 owner JS cases、2,400 独立 Nayuki matrices
- 4,320 raw-pattern matrices、全65,536 GF products、RS degree1..255
- version1..40 × ECC4 × mask8、全data/ECC codeword、mask penalty、capacity/count-width boundary
- GS1 各 pinned Node runtime 5,610 cases / 15,690 operations。具体的な入力・結果の差だけを固定 profile と比較
- SA 22 sets /112 matrices、complete diagnostics、30 shuffled merges、11 malformed merge
- jsQR 232 matrix +232 PNG、ECI metadata
- ZXing-C++ 706 matrix +750 PNG（既定 scale8）、SA/FNC1/ECI metadata
- ZXing Java 446 matrix +446 PNG（strict scale3）、SA metadata、32 correction tests

negative control は実 candidate process の matrix/data/codeword/typed-error/exit/drop を変化させ、検証が正しく失敗することを要求します。GS1 は差の個数だけが同じでも入力や値が変われば失敗します。source/tool/binary/runtime が途中で変わった report は受け入れません。

## 既定 scale8 の Java decoder 診断

`default-scale8-diagnostic.json` は **status=failed** を保持します。全446 matrix は読めますが、実 PNG は442/446。alphanumeric-L-0 (v4)、alphanumeric-L-7 (v4)、utf8-H-0 (v4)、sa-2-15 (v2) の4件は、全pixel一致する独立PNG controlでも同じ種類の失敗です。

この結果は strict scale3 lane と分離し、失敗を成功数へ変換しません。C++ の既定scale8を別に検証します。maskを変更したりpure-barcode fallbackを混ぜてgreenにしません。

## 資源・所有権・並行性

unit tests とレビューは不正UTF-8 / 任意raw bytes、制御の組合せ、count overflow、資源予算、matrix/bytes/options/diagnostic の deep copy、並行読取りを対象にします。stdlib Go fuzzing は core/segments/API/merge/GS1/URL の各公開入力を別campaignで実行します。有限fuzz/差分検証は完全な規格証明ではありません。

## 再現性・依存関係

`tools/verify_package.py` は source ZIP を同一の時刻/順序/権限で2回生成し一致を確認します。archiveを新しいdirectoryへ展開し、tests、CLI install、別Go moduleからのimportを実行します。`-trimpath -buildvcs=false` の同一compilerでbinaryを2回buildして比較します。Go module は1つだけ、go.sum不要、外部module / CGOなしです。

## 限界

ISO / GS1 認証、実カメラ、印刷物、全OS/architecture/readerの検証を主張しません。GS1 catalogとUnicode host profileの境界は `gs1.md` を参照してください。PNG圧縮bytesはGo版間で同一と限りません。公開前/native環境の未実行gateは上記のとおり明示して残します。
