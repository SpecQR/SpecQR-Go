"""Development-only process bridge to compiled, dependency-free Go.

Python only orchestrates and verifies results. Candidate symbols always come
from an actual Go executable compiled from selected sources (or an explicitly
selected exact executable), never from a reference encoder.
"""
import hashlib
import json
import os
import shutil
from pathlib import Path
import subprocess
import tempfile

GO = os.environ.get('SPECQR_GO', 'go')
BINARY = None
DRIVER = Path(__file__).resolve().parents[1] / "conformance/main.go"
_COMPILED = {}
_TEMP = []


def configure(go=None, binary=None):
    global GO, BINARY
    command = go or os.environ.get('SPECQR_GO', 'go')
    resolved = shutil.which(command)
    if not resolved:
        raise RuntimeError('Selected Go compiler was not found: ' + command)
    GO, BINARY = str(Path(resolved).resolve()), Path(binary).resolve() if binary else None


def candidate_files(root):
    return sorted(p for p in root.rglob('*.go') if not any(part in ('.git', 'node_modules', 'artifacts', '.tools') for part in p.relative_to(root).parts)) + [root / 'go.mod']


def _binary(candidate):
    root = Path(candidate).resolve()
    if BINARY:
        if not BINARY.is_file():
            raise RuntimeError('Selected Go binary does not exist: ' + str(BINARY))
        return BINARY
    files = candidate_files(root)
    if not files or any(not p.is_file() for p in files):
        raise RuntimeError('Candidate Go sources/go.mod missing: ' + str(root))
    fingerprint = tuple((str(p.relative_to(root)), hashlib.sha256(p.read_bytes()).hexdigest()) for p in files)
    version = subprocess.check_output([GO, 'version'], text=True).strip()
    key = (GO, version, fingerprint)
    if key not in _COMPILED:
        env = os.environ.copy()
        env.update(GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GOWORK='off')
        metadata = json.loads(subprocess.check_output([GO, 'list', '-m', '-json'], cwd=root, env=env, text=True))
        modules = subprocess.check_output([GO, 'list', '-m', 'all'], cwd=root, env=env, text=True).splitlines()
        if metadata.get('Path') != 'github.com/SpecQR/SpecQR-Go' or modules != ['github.com/SpecQR/SpecQR-Go']:
            raise RuntimeError('Candidate must be the SpecQR-Go module with zero module dependencies')
        dependencies = subprocess.check_output([GO, 'list', '-buildvcs=false', '-deps', '-f', '{{if not .Standard}}{{.ImportPath}}{{end}}', './...'], cwd=root, env=env, text=True).splitlines()
        if any(p and p != 'github.com/SpecQR/SpecQR-Go' and not p.startswith('github.com/SpecQR/SpecQR-Go/') for p in dependencies):
            raise RuntimeError('Candidate contains a non-standard-library external package dependency')
        temporary = tempfile.TemporaryDirectory(prefix='specqr-go-conformance-')
        _TEMP.append(temporary)
        binary = Path(temporary.name) / ('conformance.exe' if os.name == 'nt' else 'conformance')
        command = [GO, 'build', '-trimpath', '-buildvcs=false', '-o', str(binary), './tools/conformance']
        process = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True, timeout=240)
        if process.returncode:
            raise RuntimeError('Go candidate compilation failed:\n' + process.stdout + process.stderr)
        if not binary.is_file():
            raise RuntimeError('Go did not produce expected conformance binary')
        _COMPILED[key] = binary.resolve()
    return _COMPILED[key]


def generate(candidate, requests, fault=None):
    binary = _binary(candidate)
    env = os.environ.copy()
    for name in ('NODE_OPTIONS', 'NODE_PATH', 'PYTHONPATH', 'CLASSPATH', 'LD_PRELOAD', 'DYLD_INSERT_LIBRARIES', 'SPECQR_TEST_FAULT'):
        env.pop(name, None)
    if fault:
        env['SPECQR_TEST_FAULT'] = fault
    process = subprocess.run([str(binary), '--json-lines'], input=''.join(json.dumps(r, ensure_ascii=True) + '\n' for r in requests),
                             capture_output=True, text=True, encoding='utf-8', env=env, timeout=900)
    if process.returncode:
        raise RuntimeError(f'Go candidate process failed ({process.returncode}): {process.stderr[-4000:]}')
    try:
        results = [json.loads(line) for line in process.stdout.splitlines()]
    except json.JSONDecodeError as error:
        raise RuntimeError(f'Invalid Go JSON-lines response: {process.stdout[:1000]!r}; stderr={process.stderr[-1000:]!r}') from error
    if len(results) != len(requests):
        raise RuntimeError(f'Go response count mismatch: {len(results)} != {len(requests)}')
    return results


def matrix_hash(matrix):
    return hashlib.sha256(''.join(matrix).encode('ascii')).hexdigest()


def verify_identity(candidate, identity, nonce):
    binary = _binary(candidate)
    fingerprint = hashlib.sha256(binary.read_bytes()).hexdigest()
    build = json.loads(subprocess.check_output([GO, 'version', '-m', '-json', str(binary)], text=True))
    modules = []
    for dep in build.get('Deps', []):
        module = {'path': dep['Path'], 'version': dep.get('Version', ''), 'sum': dep.get('Sum', '')}
        if dep.get('Replace'):
            module.update(replacePath=dep['Replace']['Path'], replaceVersion=dep['Replace'].get('Version', ''))
        modules.append(module)
    external = [dep['Path'] for dep in build.get('Deps', []) if dep['Path'] != 'github.com/SpecQR/SpecQR-Go']
    if external or identity.get('goModules') != modules or identity.get('goVersion') != build.get('GoVersion'):
        raise RuntimeError('Candidate identity does not match independently parsed Go build metadata')
    if (identity.get('nonce') != nonce or identity.get('pid') == os.getpid()
            or not isinstance(identity.get('pid'), int)
            or identity.get('language') != 'Go'
            or Path(identity.get('executable', '')).resolve() != binary
            or identity.get('binarySha256') != fingerprint
            or identity.get('runtimeDependencies') != []
            or identity.get('packageFilesSha256') != {binary.name: fingerprint}):
        raise RuntimeError(f'Candidate Go process/executable identity check failed: {identity}')
