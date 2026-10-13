# Optional URL differential lane

This development-only harness compares 10,000 seeded URL cases against the
WHATWG `URL` implementation in explicitly pinned Node 24.19.0 or 24.21.0.

実際の GS1 helper を使う opt-in テストを通じて、製品用の Go adapter を実行します。Node と Python は Go package や通常のテストの依存関係ではありません。この検証 lane は Go 1.26.8 と 1.27.1 を受け入れます。

```sh
python3 tools/url-differential/run.py \
  --node /absolute/path/to/node \
  --go /absolute/path/to/go
```

runner は両 toolchain の版と、Node の ADA・ICU・Unicode component（ADA はそれぞれ 3.4.4/4.0.0、ICU 78.3、Unicode 17.0）を検証し、一時 directory に参照 JSON ファイルを生成して `TestGS1URLDifferential` を実行した後、そのファイルを削除します。ネットワーク、第三者 module、インストール済み package、外部サービスは使用しません。
参照成果物を保存するには、`node tools/url-differential/generate.mjs` を実行して stdout を保存し、Go テストの実行時に `SPECQR_URL_REFERENCE` でそのファイルを指定します。出力には Node の版、固定 seed、case 数を記録します。

対象範囲は ASCII/domain/IPv4/IPv6 の host、credentials、既定および非既定の port、path escaping と dot segment、special query、special scheme です。非空 fragment（GS1 が拒否するもの）と、GS1 primary AI の後の dot-only 値（SpecQR が意図的に保持するもの）は、意図的に対象から除外します。この lane は完全な IDNA/UTS #46 適合を主張しません。製品用の host adapter は、文書化された保守的な Unicode 15.0.0 の固定 profile を使います。独立した通常の Go suite は、受理・拒否する Unicode の例を含み、すべての Unicode scalar を対象に accepted-host/ACE の往復変換の安定性を検査します。
