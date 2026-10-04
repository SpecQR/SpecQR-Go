# Structured Append

`GenerateStructuredAppend(text, StructuredAppendOptions)`、`GenerateBytesStructuredAppend(data, options)`、`GenerateSegmentsStructuredAppend(segments, options)` は 2..16 symbols の immutable `SAResult` を返します。

```go
o := specqr.StructuredAppendOptions{
    Options: specqr.Options{Version: 2, ECC: specqr.M},
    MaxSymbols: 16,
    SymbolDiagnostics: true,
}
set, err := specqr.GenerateStructuredAppend(strings.Repeat("ABC123", 40), o)
```

各 version で最大収容 prefix を greedy に選び、指定範囲の最小 fitting version を採用します。1 symbol に収まる入力を無理に2分割しません。全 symbols は同じ version/ECC を使い、mask は個別に選択します。

text の境界は Unicode scalar、raw bytes は byte 境界です。手動 segment は元の mode を維持し、byte mode だけ分割できます。numeric / alphanumeric / Kanji の segment は分割不可です。1つの不可分 segment が小さい version に入らなければ、より大きい version が必要です。

`MaxSymbols` のゼロ値は16、明示範囲は2..16。`FullSplitUnits` は手動 source の unit 詳細、`SymbolDiagnostics` は decoder metadata 互換性の警告を含めます。いずれも encoded bytes を変えません。ECI / FNC1 / GS1 / ECC boost / 既存 SA header は同時指定できません。manual route の mode強制・最適化無効指定も拒否します。

`Symbols()` / `Total()` / `Parity()` / `InputLength()` / `ByteLength()` / `Diagnostics()` を利用します。diagnostics の `splitStrategy` は `greedy-largest-fitting` または `segment-boundary-byte-chunk`。byte offsets、scalar/source-segment offsets、capacity、各 symbol の mask を返します。

## Parity と結合

`CalculateStructuredAppendParity(string)`、`CalculateStructuredAppendBytesParity([]byte)`、`CalculateStructuredAppendSegmentsParity([]Segment)` は元 UTF-8 / raw bytes の XOR を返します。Kanji codeword や encode 後の bytes の XOR ではありません。

`SAPart{Index,Total,Parity,Text,Bytes,Binary}` の Index は **1-based**。Binary=false のとき Text は nonnil、Binary=true のとき Bytes を使い Text は nil にします。空 bytes は許されます。Text と Bytes を同時指定できません。

`MergeStructuredAppendParts(parts)` は順不同の完全セットを検証し、index順に結合します。範囲、total/parity の統一、重複・欠落、text/binary の混在、UTF-8、aggregate size、実 XOR を検査します。欠落セットの暫定結合は提供しません。MergeResult の `Text()` / `Bytes()` / `Binary()` / `Parts()` / `PartMetadata()` / `Diagnostics()` はコピーまたは immutable 値を返します。

Parity は一部の誤りを検出するだけで、真正性・暗号学的な完全性を保証しません。decoder は GS1/ECI/SA metadata を異なる形で公開するため、reader側でも結合規則を確認してください。
