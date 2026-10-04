# 開発

Go 1.26.8 と 1.27.1 で `go test ./...`、`go test -race ./...`、`go vet ./...`、`gofmt` を実行してください。標準ライブラリ以外の Go module や CGO は追加しない方針です。

変更時は `tools/verification/README.md` の独立検証を source と detached module consumer の両方で実行します。oracle との差を成功扱いにしたり、失敗から fixture を無条件に更新しないでください。URL profile 差は具体的な input/outcome と pinned Node identity で説明します。PNG 圧縮バイトでなく全画素を検証します。

`tools/verify_package.py` は reproducible source ZIP、zero dependency、CGO-free、インストールした CLI、clean consumer、同一 toolchain の再ビルド一致を確認します。開発 oracle は Go API にリンクされません。
