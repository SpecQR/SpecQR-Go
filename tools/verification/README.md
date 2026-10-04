# Development-only verification

The subject is a separately compiled, dependency-free Go executable. Python
orchestrates comparisons, Node runs pinned JavaScript/Nayuki references, and
three independent decoders inspect the subject's real matrices and PNGs. None
of these tools or packages generates candidate QR symbols. The development adapter lives in `tools/conformance`; it adds no dependency
and is not part of the production API or CLI.

## Reproduce

Use Go 1.26.8 or newer (also tested on Go 1.27.1), Python 3.11+, Node, and Java 17+ (for the Java decoder).
The GS1 audits require the exact official Node 24.19.0 and 24.21.0 runtimes; their
Ada/ICU/Unicode versions are checked, not just the major version.

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

Each candidate process is built offline with `go build -trimpath -buildvcs=false
-o <temporary> ./tools/conformance` into temporary storage. `--candidate`
selects a source tree, `--go` selects Go, and `--binary` selects an exact
prebuilt development adapter. That last option does not substitute a source
build for the supplied artifact. For a module-consumer lane, unpack a source archive
into a clean directory and select it with `--candidate`.

`SPECQR_GO` sets the default Go command. `SPECQR_DEV_NODE_MODULES` may point
to a separate development package directory containing `package.json` and
`node_modules`. ZXing-C++ official PyPI wheel hashes and ZXing Java JAR SHA-256
are checked before use. `--no-install` requires existing verified decoder files.
The Go module has no external module dependencies. Node/Python/Java packages
are isolated development-only independent references/decoders.
Reports default to `artifacts/`; `--output` may select an external location.

## Exact coverage

- 3,028 live pinned-JS public matrices with complete padded data and interleaved
  data/ECC codewords, version/ECC/mask, and SHA-256 of every matrix module.
  Includes all 1,280 version 1–40 × ECC × mask combinations, mixed/manual modes,
  Unicode, Kanji, binary, automatic optimization/masks, ECI boundaries, both
  FNC1 positions, GS1, boost and deterministic fuzz.
- 2,400 independently checked Nayuki matrices, including every fixed
  version/ECC/mask combination. Independent segmentation/mask choices are not
  silently treated as equivalent.
- 4,320 raw-pattern matrices with all mask scores, 65,536 GF products and
  all Reed–Solomon generator/remainder degrees 1–255.
- 640 exact capacities, 1,920 boundary estimates, 22 Structured Append sets and
  112 member matrices, 256 shared-process concurrent matrices, and 18 typed
  malformed rejections. The separate SA lane compares complete diagnostics,
  split offsets/units and codewords, then checks 30 shuffled text/binary merges
  across all totals 2–16 and 11 malformed merges against live JavaScript.
- jsQR: 232 matrix plus 232 real-PNG detections, every version, and 64 ECI headers.
  jsQR does not support FNC1/SA; no success is claimed for them in this lane.
- ZXing-C++: 706 matrix plus 750 PNG detections, 135 SA headers, all 152 legal
  FNC1-second indicators on both manual and high-level routes, ECI bytes/text,
  five complete high-level sets (44 symbols), and independent reconstruction.
- ZXing Java: 446 matrix plus 446 PNG detections, every version, 135 SA headers,
  five complete sets reconstructed on both routes, and 32 damaged-symbol tests
  that require exactly three corrected codewords each.

Every decoded RGBA pixel and quiet-zone pixel of the actual Go PNG is checked before
independent detection. ZXing Java strict PNG detection uses scale 3 without
`PURE_BARCODE` or fallback to a matrix. Its scale-8 behavior is a separate complete 446-symbol diagnostic: 442 succeed
and four fail, with every failure reproduced by a same-pixel independent PNG
control. The scale-8 report remains failed. `--png-scale 8` reproduces that
separate diagnostic, and `summarize_default_scale.py` refuses stale results or
any unexplained failure. Failed detection
never increments success counts. C++ checks default scale 8. All three decoders
must reject real blank images.

## Exact GS1 profiles

Each Node profile runs all 5,610 cases / 15,690 operations against live pinned
JavaScript, including the complete bounded AI catalogue, diagnostics, malformed
percent escapes, IPv4/IPv6, authority/path/query handling and URL creation.
`gs1/reference-url-outcomes.json` pins all 54 IDNA/dot cases for both exact Node
runtimes. `gs1/go-url-outcomes.json` records 55 independently constructed Go
expected outcomes (the same 54 cases plus one UTF-16-only input). Repeated dot-creation
cases preserve the original corpus multiplicity. No earlier-language candidate outcomes
are imported as Go expectations; `gs1/go_profile.py` derives the finite
expectations from explicit rules and Python standard-library RFC3492, and no broad IDNA/dot category is exempted.
Unknown differences, changed values with unchanged totals, and stale fixtures
fail the audit.

The Go URL implementation is bounded and dependency-free: pinned Unicode
15.0.0 lowercase/rejection tables, fullwidth ASCII and alternate-dot mapping,
soft-hyphen removal, RFC3492 Punycode and strict ACE roundtrip validation.
Unsupported combining/format/unassigned or compatibility-unstable scalars and
Hangul Jamo are rejected. Conservative RTL checks reject mixed letter/digit
conventions. A separate 39-host-variant corpus checks 78 requests / 156 operations
against these explicit policies and also records live Node differences.
It does not claim complete UTS46, NFC, or browser URL parity for all Unicode.
It deliberately preserves GS1 dot-only values and emits them as query values.
The strict Go JSON
boundary rejects the single such corpus input instead of replacing it. The
report lists this input-domain difference explicitly. Node 24.21's acceptance
of malformed ASCII ACE labels is not inherited by Go.

## Subject identity and mutation controls

Reports include source/test hashes, the exact Go process PID/nonce/executable,
independently verified executable SHA-256, zero-dependency Go module and native executable metadata,
reference commit/source/corpus hashes, and decoder/package fingerprints. Source,
executable, dependency or tool changes invalidate a run. Strict response counts
prevent truncation from becoming a pass. A scoped `--suite public` or `internal`
result is explicitly marked incomplete.

Six adapter faults first execute real Go then corrupt a matrix hash, data
byte, or interleaved codeword; exit, omit a response, or emit an error. Three
more faults prove untyped failures cannot count as typed validation. SA and
merge faults corrupt metadata and reconstructed payloads. GS1 controls corrupt
actual Go catalogue output, reference URL data, and a Go dot URL while
preserving its aggregate difference class. These controls are confined to the
development-only adapter, never the production library or ordinary CLI.

These are finite synthetic checks, not ISO/GS1 certification, print/camera tests,
or guarantees for arbitrary damage or inputs. Packaging, compiler/OS matrices
and clean offline consumers are separate checks.

## Provenance

The development harness is adapted from MIT-licensed SpecQR-Rust and SpecQR-Kotlin, whose
workflow derives from SpecQR-Java `1f2bf277a582f0eb4e31c9975443c1a76552aa23`,
SpecQR-Python `1deb5cf83ed7a93eeb44138e5980a963a00cdffb`, SpecQR-CPP
`e91cd8fe4434fd6ef10d1126bb2fd17b0b19321f`, and SpecQR-CSharp
`057c4b3f25e52c4786a8f94c744ff884eedcecfa`. The Java decoder retains its original
SpecQR-Swift attribution. These are test workflow ancestors; the Go subject
links no earlier-language implementation, encoder or decoder.

## Offline module consumer

`build_consumer.py --candidate <unpacked-module> --consumer-dir <empty-directory>
--output <receipt.json>` creates an independent module whose copied development
adapter imports only the public SpecQR-Go API. The receipt records Go build-info,
exact replacement path, compiler, adapter/source hashes and executable identity.
Run all seven strict lanes against that exact artifact with `--binary`; no source
rebuild is substituted. This local offline-consumer check is distinct from a
post-publication module download and does not claim a public fetch happened.

`run_all.py` runs the seven strict lanes and rejects any failed, scoped, stale or
source-inconsistent report before producing a summary. Compiler and decoder
executables are fingerprinted; external packages are pinned and checked. Do not
change source or verification files while a release-evidence run is active.
