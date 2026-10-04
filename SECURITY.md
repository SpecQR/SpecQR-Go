# セキュリティ

入力・出力 allocation と算術に上限を設け、不正入力は型付きエラーで扱います。QR は秘密情報の暗号化手段ではありません。GS1 チェックデジットや Structured Append parity は認証ではありません。

SVG の属性 escape は CSS の意味全体を制限する仕組みではありません。信頼できない色指定をそのままブラウザへ渡さないでください。URL helper は QR payload 構築用で、ネットワークアクセス可否や SSRF の policy 判定には使わないでください。

問題報告には公開可能な最小再現入力、Go version、OS/architecture、commit、期待結果を添えてください。実際の秘密・credential・個人データを公開 issue に貼らないでください。
