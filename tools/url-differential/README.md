# Optional URL differential lane

This development-only harness compares 10,000 seeded URL cases against the
WHATWG `URL` implementation in explicitly pinned Node 24.19.0 or 24.21.0.
It runs the production Go adapter through an opt-in test using the real GS1
helpers. Node and Python are not dependencies of the Go package or ordinary
tests. Go 1.26.8 and 1.27.1 are accepted for this verification lane.

```sh
python3 tools/url-differential/run.py \
  --node /absolute/path/to/node \
  --go /absolute/path/to/go
```

The runner validates both toolchain versions plus Node's ADA, ICU, and Unicode
components (ADA 3.4.4/4.0.0 respectively, ICU 78.3, Unicode 17.0), generates a reference JSON file
in a temporary directory, runs `TestGS1URLDifferential`, then removes the file.
No network, third-party module, installed package, or external service is used.
For a retained reference artifact, run `node tools/url-differential/generate.mjs`
and capture stdout; set `SPECQR_URL_REFERENCE` to that file when running the Go
test. The output records the Node version, fixed seed, and case count.

Scope covers ASCII/domain/IPv4/IPv6 hosts, credentials, default and nondefault
ports, path escaping and dot segments, special queries, and special schemes.
It intentionally excludes nonempty fragments (GS1 rejects them) and dot-only
values after a GS1 primary AI (preserved intentionally by SpecQR). This lane
does not claim complete IDNA/UTS #46 conformance: the production host adapter
uses a documented conservative frozen Unicode 15.0.0 profile. The independent
ordinary Go suite includes supported/rejected Unicode examples and checks every
Unicode scalar for stable accepted-host/ACE round trips.
