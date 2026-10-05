# URL serialization compatibility regression

固定した SpecQR `16efc6c0a8e397c9df3d051d20fce6c1eebdfad7` の独立 TS 出力を使います。現在の候補出力から期待値を作成しません。元の 1,411 request と全 options、80 positive targets、49 shared operations、39 adversarial probes を保持します。公開版から保持する native residual は入力・完全な出力・公開 commit を固定し、差の件数だけでは合格にしません。元の fixture、102 FNC1 vectors と 4 manual vectors も変更しません。

```sh
python3 tools/url-serialization/verify.py
```

Java/Kotlin は `SPECQR_JAVA`、Kotlin は `KOTLIN_HOME`、Go は `SPECQR_GO`、Rust は `SPECQR_CARGO`、C++ は `CXX` でローカル compiler を選べます。Java/Kotlin の `--jar` は source の代わりに指定した配布 JAR を使用します。Go/Rust は独立 module/crate adapter をビルドします。全 API 呼出しはその言語の実装で行います。C++ は全 request を型付き式へ生成し、文字列の byte length と present-empty options を保持します。

JSON duplicate/nonfinite、型、追加 field、警告件数、exit/stderr、response cardinality、source 変更を fail-closed で検査します。診断は既存の共通比較契約と同じ code/reason/count を照合し、人間向け prose、null/absent と catalog の isVariable 補助 field は比較外です。payload、metadata、順序は完全一致です。

この追加 gate は既存の unit/package、matrix/data/ECC、独立 PNG decoder、各 OS/compiler CI を置換しません。未実行環境の成功を先取りしません。

作成時だけ base URL の空 `#` を保持します。正規化は従来どおり空 fragment を除き、非空 fragment は拒否します。dot-only GS1 値は安全に query へ保持し、decoded query NUL と既存 IDNA 方針を変えません。

これは限定した URL 出力互換性の拡張です。通常の QR、FNC1、segment、matrix、codeword、PNG、公開 API と runtime dependency は変更しません。

102 個すべての元 FNC1 vector は native byte fallback / typed forced-alpha refusal の既存契約で再実行します。独立 TS explicit-byte 参照と data/ECC/matrix を比較し、追加の 68 容量制限なし controls と 4 manual vectors は実 PNG を ZXing-C++ でデコードして全 payload byte を検査します。`python tools/url-serialization/verify_fnc1.py --dependency-dir .tools/zxing-cpp`（C++ は `--adapter build/specqr-conformance` を追加）。
