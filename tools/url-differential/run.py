#!/usr/bin/env python3
"""Reproduce the stdlib URL adapter's seeded differential lane.

Example: python3 tools/url-differential/run.py --node /path/to/node --go /path/to/go
Requires one of the documented pinned Node and Go development toolchains.
No reference implementation is part of the library or ordinary test suite.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--node", required=True, help="Node 24.19.0 or 24.21.0 binary")
parser.add_argument("--go", required=True, help="Go 1.26.8 or 1.27.1 binary")
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
go_version = subprocess.check_output([args.go, "version"], text=True).strip()
if not any(f"go{version} " in go_version for version in ("1.26.8", "1.27.1")):
    raise SystemExit(f"Unexpected development toolchain: {go_version}")
raw = subprocess.check_output([args.node, str(Path(__file__).with_name("generate.mjs"))], text=True)
reference = json.loads(raw)
with tempfile.TemporaryDirectory(prefix="specqr-url-reference-") as directory:
    path = Path(directory) / "reference.json"
    path.write_text(raw, encoding="utf-8")
    env = dict(os.environ, SPECQR_URL_REFERENCE=str(path), GOTOOLCHAIN="local")
    subprocess.run([args.go, "test", "-count=1", "-run", "^TestGS1URLDifferential$", "-v", "."], cwd=root, env=env, check=True)
print(json.dumps({"lane": "url-differential", "goVersion": go_version, "nodeVersion": reference["nodeVersion"], "runtimeVersions": reference["runtimeVersions"], "cases": reference["caseCount"], "uniqueInputs": reference["uniqueInputs"], "mismatches": 0}))
