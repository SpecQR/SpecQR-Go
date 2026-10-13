# 公開前の実機検証

これは公開前に使った native 検証手順です。Windows amd64 の両 Go 版の native gate は通過済みです。macOS arm64 も本体・module・tooling が同一の候補で通過し、その後は `api_test.go` のレビュー済み修正だけです。最終テスト snapshot を Mac で再実行したとは表示しません。Linux386 は QEMU の通常実行が通過、root-package unoptimized debug は両版で失敗、native 実行は未実行です。初期公開では32-bitで標準ライブラリ・runtimeまで最適化を無効にする構成を既知の制限として扱い、失敗を通過に変更しません。現在の結果と診断は [verification.md](verification.md) / [platform-status.json](platform-status.json) を参照してください。以下は native 検証の再現用手順です。cross-compilation やエミュレーションを native pass と表示しません。

## 必要環境

- Windows **amd64** の実機または動作中の native Windows VM。Mac は Intel / Apple Silicon の native OS
- Go **1.26.8** と **1.27.1** を別ディレクトリへ展開した公式 toolchain
- Python **3.12**（固定 Unicode 15 generator の再現にも使用）
- Windows の race test は mingw-w64 runtime **v8+** を含む C compiler。`gcc --print-file-name libsynchronization.a` が実ファイルへのフルパスを返すこと
- macOS の race detector は darwin/amd64 と darwin/arm64 をサポート

根拠: [Go race detector requirements](https://go.dev/doc/articles/race_detector#Requirements)、[公式インストール](https://go.dev/doc/install)、[公式ダウンロード](https://go.dev/dl/)。archive URL と SHA-256 は `toolchain-downloads.json` に固定しています。取得したファイルの hash を確認してから展開してください。runner 自身は software install / network / publication を行いません。

SpecQR の runtime/production CLI は CGO を使いません。Go の race instrumentation 自体だけは CGO を有効にする必要があり、別に `CGO_ENABLED=0` の通常 test も実行します。

## Windows PowerShell

source ZIP を展開し、`SpecQR-Go` directory で実行します。パスは実際の toolchain 場所へ置き換えます。

```powershell
py -3.12 tools/verify_native_platform.py --attest-native-execution --expected-os windows --go-min C:\tools\go1.26.8\go\bin\go.exe --go-current C:\tools\go1.27.1\go\bin\go.exe --output-dir ..\windows-validation
```

## macOS

```sh
python3.12 tools/verify_native_platform.py --attest-native-execution --expected-os darwin --go-min "$HOME/tools/go1.26.8/go/bin/go" --go-current "$HOME/tools/go1.27.1/go/bin/go" --output-dir ../macos-validation
```

## Linux 386

現cloud hostでは386 binaryの実行がexec-format errorで阻止されるため未実行です。32 bit 実行を許す native Linux amd64 executor（32 bit 実行対応）で、次を実行します。architecture/security 制限を変更して迂回する手順ではありません。

```sh
python3.12 tools/verify_native_platform.py --attest-native-execution --expected-os linux --go-min /path/go1.26.8/go/bin/go --go-current /path/go1.27.1/go/bin/go --execute-linux-386 --output-dir ../linux386-validation
```

## 結果の扱い

runner は native OS/Go host architecture、正確な Go version、compiler executable hash、source Go/mod hash を保存します。normal / unoptimized debug / race / vet / CGO-free / gofmt / go doc / Unicode generator / source ZIP 再現性 / installed CLI /別 module consumer / example 実行を両版で確認します。Linux386 フラグを付けた場合は本当に test と CLI を実行し、compile だけで成功にしません。

output directory は source 外の新規または空 directory を指定してください。既存の receipt/log は上書きしません。

`native-validation.json` の status が passed であり、source hashes が対象 candidate と一致することを確認してください。失敗または環境不足なら failed-or-blocked と個別 log を残します。未実行 gate を省いて passed にしないでください。log 内の個人の local paths は公開時に sanitize してください。変更後は差分が影響する matrix/oracle/review/platform 証拠を再検証します。test だけの変更では、元の実行 snapshot と本体の byte 一致を明示して証拠を引き継げますが、新しい test をその OS で実行したとは主張しません。

必要な実機 gate と明示した公開範囲のレビュー後、正確な公開 commit で public CI・fresh clone・pinned public module consumer を別途確認します。未解決の32-bit debug失敗やnative Linux386未実行は、この公開範囲の下でも証拠として保持します。

## Native attestation

`--attest-native-execution` は、実行者が実際の native OS/architecture、または同 architecture の hardware-virtualized VM であり、instruction-set/full-system emulation ではないと確認したときだけ指定します。不明な場合は確認するまで止めてください。macOS は `sysctl -n sysctl.proc_translated` で Rosetta を拒否し、Windows は `IsWow64Process2` または `GetNativeSystemInfo`/`IsWow64Process` で native AMD64 process/OS を確認します。これらの検査でも任意の full-system emulator が存在しないことまでは証明できないため、attestation を receipt に明示します。Linux386 追加 gate は先に amd64 の race を含む全 suite を実行するため、32 bit native Python/Go host だけではこの runner の全要件を満たしません。
