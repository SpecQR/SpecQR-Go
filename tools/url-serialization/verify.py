#!/usr/bin/env python3
"""Strict, source-bound replay of independent TS and published native contracts."""
import argparse, collections, gzip, hashlib, json, os, pathlib, shutil, subprocess, tempfile
HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
LANG = 'Go'

def sha(data):
    return hashlib.sha256(data).hexdigest()

def strict(data):
    def pairs(rows):
        result = {}
        for key, value in rows:
            if key in result:
                raise ValueError('duplicate JSON key: ' + key)
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=pairs,
                      parse_constant=lambda x: (_ for _ in ()).throw(ValueError(x)))

def contract(value):
    if isinstance(value, list):
        return [contract(x) for x in value]
    if isinstance(value, dict):
        if 'code' in value and 'message' in value:
            return {k: contract(value[k]) for k in ('code', 'reason', 'count') if value.get(k) is not None}
        return {k: contract(x) for k, x in value.items() if x is not None and k != 'isVariable'
                and not (k == 'errors' and value.get('ok'))}
    return value

def same(a, b):
    if type(a) is not type(b):
        return False
    if isinstance(a, dict):
        return a.keys() == b.keys() and all(same(a[k], b[k]) for k in a)
    if isinstance(a, list):
        return len(a) == len(b) and all(same(x, y) for x, y in zip(a, b))
    return a == b

def source():
    # Include complete owned source/test/tool files, including newly added files.
    excluded = {'.git', '.tools', '.baseline', 'node_modules', '__pycache__', 'artifacts',
                'build', 'dist', 'target', 'baseline'}
    result = {}
    for path in ROOT.rglob('*'):
        parts = path.relative_to(ROOT).parts
        if any(p in excluded or p.startswith('build-') for p in parts):
            continue
        if path.is_file():
            if path.is_symlink():
                raise ValueError('source symlink: ' + str(path))
            result[str(path.relative_to(ROOT))] = sha(path.read_bytes())
    return result

def process(exit_code, stderr, stdout, count):
    if exit_code != 0 or stderr or not stdout.endswith(b'\n'):
        raise ValueError('process exit/stderr/newline contract')
    rows = [strict(line) for line in stdout.splitlines()]
    if len(rows) != count:
        raise ValueError('response cardinality')
    return rows

def negative_controls():
    controls = []
    def reject(name, fn):
        try:
            fn()
        except (ValueError, AssertionError):
            controls.append(name)
        else:
            raise AssertionError('negative control accepted: ' + name)
    for name, value in [('duplicate-key', b'{"x":1,"x":2}'), ('nonfinite', b'NaN')]:
        reject(name, lambda value=value: strict(value))
    for name, ex, err, out, n in [('exit', 1, b'', b'1\n', 1), ('stderr', 0, b'error', b'1\n', 1),
                                 ('drop', 0, b'', b'1\n', 2), ('extra', 0, b'', b'1\n2\n', 1),
                                 ('newline', 0, b'', b'1', 1)]:
        reject(name, lambda ex=ex, err=err, out=out, n=n: process(ex, err, out, n))
    for name, a, b in [('bool-number', True, 1), ('integer-float', 1, 1.0), ('string-number', '1', 1),
                       ('container', [], {}), ('added-field', {'ok': True}, {'ok': True, 'extra': 1}),
                       ('warning-count', {'warnings': [{'code':'X','count':1}]}, {'warnings':[{'code':'X','count':2}]}),
                       ('payload', 'https://example.com/a%5Eb', 'https://example.com/a^b')]:
        if same(a, b):
            raise AssertionError(name)
        controls.append(name)
    old = {'source': 'old'}
    if same(old, {'source': 'changed'}):
        raise AssertionError('source-change')
    controls.append('source-change')
    return controls

def cpp_source(requests):
    def st(s):
        b = s.encode('utf-8')
        return 'std::string("' + ''.join('\\%03o' % c for c in b) + '",' + str(len(b)) + ')'
    def seq(xs):
        return 'std::vector<std::string>{' + ','.join(st(s) for s in xs) + '}'
    def elem(es):
        return 'std::vector<Element>{' + ','.join('{' + st(e['ai']) + ',' + st(e['value']) + '}' for e in es) + '}'
    funcs = {'dictionary':'supported_ais()', 'info':'ai_info(s)', 'checkDigit':'calculate_check_digit(s)',
        'validateCheckDigit':'validate_check_digit(s)', 'gtinDigit':'calculate_gtin_check_digit(s)',
        'gtinAppend':'append_gtin_check_digit(s)', 'gtinValidate':'validate_gtin_check_digit(s)',
        'ssccDigit':'calculate_sscc_check_digit(s)', 'ssccAppend':'append_sscc_check_digit(s)',
        'ssccValidate':'validate_sscc_check_digit(s)', 'human':'parse_human_readable(s)',
        'create':'create_element_string(e)', 'raw':'parse_element_string(s)', 'validateElements':'validate_elements(e,v)',
        'validateRaw':'validate_element_string(s,v)', 'linkCreate':'create_digital_link(e,c)',
        'linkParse':'parse_digital_link(s,p)', 'linkValidate':'validate_digital_link(s,vp)', 'linkNormalize':'normalize_digital_link(s,np)'}
    lines = []
    for r in requests:
        lines.append('emit([&](){std::string s=' + st(r.get('input','')) + ';auto e=' + elem(r.get('elements',[])) + ';ValidationOptions v; DigitalLinkOptions c;DigitalLinkParseOptions p;DigitalLinkValidationOptions vp;DigitalLinkNormalizeOptions np;')
        for k, v in r.get('options', {}).items():
            if k == 'context':
                assert v == 'digital-link'
                lines.append('v.context=ValidationContext::DigitalLink;')
            elif k == 'collectAllErrors': lines.append('v.collect_all_errors=' + str(v).lower() + ';')
            elif k == 'allowUnsupportedAi': lines.append('v.allow_unsupported_ai=' + str(v).lower() + ';')
            elif k == 'baseUrl': lines.append('c.base_url=' + st(v) + ';')
            elif k == 'primaryAi': lines.append('c.primary_ai=' + st(v) + ';p.primary_ai=' + st(v) + ';vp.primary_ai=' + st(v) + ';np.primary_ai=' + st(v) + ';')
            elif k == 'pathAis': lines.append('c.path_ais=' + seq(v) + ';')
            elif k == 'unknownQuery':
                assert v == 'reject'
                lines.append('p.unknown_query=UnknownQueryPolicy::Reject;vp.unknown_query=UnknownQueryPolicy::Reject;np.unknown_query=UnknownQueryPolicy::Reject;')
            elif k == 'normalize': lines.append('vp.normalize=' + str(v).lower() + ';')
            elif k == 'mode': lines.append('np.mode=' + st(v) + ';')
            else: raise AssertionError(k)
        lines.append('return ' + funcs[r['op']] + ';});')
    return (HERE/'adapter.cpp').read_text() + '\nint main(){\n' + '\n'.join(lines) + '\n}\n'

def build(work, requests, jar):
    logs = []
    env = dict(os.environ)
    def run(cmd, cwd=None):
        p = subprocess.run([str(x) for x in cmd], cwd=cwd, env=env, capture_output=True, timeout=600)
        logs.append({'argv':[str(x) for x in cmd], 'exitCode':p.returncode,
                     'stdout':p.stdout.decode(), 'stderr':p.stderr.decode()})
        if p.returncode:
            raise RuntimeError(json.dumps(logs))
    if LANG in ('Java', 'Kotlin'):
        java = pathlib.Path(shutil.which(os.environ.get('SPECQR_JAVA', 'java'))).resolve()
        javac = java.with_name('javac')
        classes = work/'classes'; classes.mkdir()
        cp = str(classes)
        if LANG == 'Kotlin':
            home = pathlib.Path(os.environ['KOTLIN_HOME']).resolve()
            cp += os.pathsep + str(home/'lib/kotlin-stdlib.jar')
            if not jar:
                run([java, '-Dfile.encoding=UTF-8', '-Xmx1g', '-cp', str(home/'lib/*'), 'org.jetbrains.kotlin.cli.jvm.K2JVMCompiler',
                     '-kotlin-home', home, '-Xjdk-release=17', '-language-version', '2.2', '-api-version', '2.2',
                     '-no-reflect', '-Werror', '-module-name', 'specqr', '-d', classes,
                     *sorted((ROOT/'src/main/kotlin').rglob('*.kt'))])
        if jar:
            cp += os.pathsep + str(jar)
        inputs = sorted((ROOT/'src/main/java').rglob('*.java')) if LANG == 'Java' and not jar else []
        run([javac, '--release', '17', '-d', classes, '-cp', cp, *inputs, HERE/'CorpusAudit.java'])
        command = [str(java), '-Dfile.encoding=UTF-8', '-cp', cp, 'io.specqr.CorpusAudit']
        binary = {str(p.relative_to(work)):sha(p.read_bytes()) for p in classes.rglob('*.class')}
        if jar: binary['jar'] = sha(jar.read_bytes())
    elif LANG == 'Go':
        (work/'go.mod').write_text('module audit\n\ngo 1.26.8\nrequire github.com/SpecQR/SpecQR-Go v0.0.0\nreplace github.com/SpecQR/SpecQR-Go => ' + str(ROOT) + '\n')
        shutil.copyfile(HERE/'adapter.go', work/'main.go')
        go = os.environ.get('SPECQR_GO', 'go')
        env.update(GOTOOLCHAIN='local', GOWORK='off', GOPROXY='off')
        run([go, 'build', '-buildvcs=false', '-o', work/'audit', '.'], work)
        command = [str(work/'audit')]; binary = {'audit':sha((work/'audit').read_bytes())}
    elif LANG == 'Rust':
        (work/'Cargo.toml').write_text('[package]\nname="corpus-audit"\nversion="0.0.0"\nedition="2024"\n[[bin]]\nname="audit"\npath="main.rs"\n[dependencies]\nspecqr={path=' + json.dumps(str(ROOT)) + '}\n')
        shutil.copyfile(HERE/'adapter.rs', work/'main.rs')
        env['CARGO_TARGET_DIR'] = str(work/'target')
        run([os.environ.get('SPECQR_CARGO','cargo'), 'build', '--offline', '--manifest-path', work/'Cargo.toml'])
        command = [str(work/'target/debug/audit')]; binary = {'audit':sha(pathlib.Path(command[0]).read_bytes())}
    else:
        generated = cpp_source(requests)
        (work/'adapter.cpp').write_text(generated)
        run([os.environ.get('CXX','g++'), '-std=c++17', '-O0', '-I', ROOT/'include', work/'adapter.cpp',
             *sorted((ROOT/'src').glob('*.cpp')), '-o', work/'audit'])
        command = [str(work/'audit')]; binary = {'audit':sha((work/'audit').read_bytes()), 'generatedSource':sha(generated.encode())}
    return command, logs, binary

def main():
    p = argparse.ArgumentParser(); p.add_argument('--output', type=pathlib.Path, default=ROOT/'artifacts/url-serialization.json'); p.add_argument('--jar', type=pathlib.Path)
    args = p.parse_args()
    manifest = strict((HERE/'fixtures/manifest.json').read_bytes())
    for name, digest in manifest['sha256'].items():
        assert sha((HERE/'fixtures'/name).read_bytes()) == digest, name
    rows = strict(gzip.decompress((HERE/'fixtures/contracts.json.gz').read_bytes()))
    assert len(rows) == 1499
    historical = strict((HERE/'fixtures/gs1-upstream.json').read_bytes())['cases']
    current = strict(gzip.decompress((HERE/'fixtures/current-ts-gs1-1411.json.gz').read_bytes()))['cases']
    shared = strict((HERE/'fixtures/current-ts-gs1-shared49.json').read_bytes())['cases']
    assert len(historical) == len(current) == 1411 and len(shared) == 49
    for i, original in enumerate(historical):
        request = {k:v for k,v in original.items() if k != 'expected'}
        assert same(rows[i]['request'], request) and same(current[i]['request'], request)
        assert same(contract(rows[i]['tsExpected']), contract(current[i]['expected']))
    for i, row in enumerate(shared):
        assert same(rows[1411+i]['request'], row['request'])
        assert same(contract(rows[1411+i]['tsExpected']), contract(row['expected']))
    positives = strict((HERE/'fixtures/approved-restorations80.json').read_bytes())['cases']
    assert len(positives) == 80
    controls = negative_controls()
    before = source()
    requests = [r['request'] for r in rows]
    body = b''.join((json.dumps(r,ensure_ascii=True,separators=(',',':'))+'\n').encode() for r in requests)
    assert sha(body) == manifest['requestSha256']
    with tempfile.TemporaryDirectory(prefix='specqr-url-') as tmp:
        command, builds, binary = build(pathlib.Path(tmp), requests, args.jar)
        result = subprocess.run(command, input=body, capture_output=True, timeout=600)
        actual = process(result.returncode, result.stderr, result.stdout, 1499)
    mismatches = []
    for index, (r, a) in enumerate(zip(rows, actual)):
        expected = r.get('nativeExpected', r['tsExpected'])
        if not same(contract(expected), contract(a)):
            mismatches.append({'index':index, 'request':r['request'], 'expected':expected, 'actual':a})
    assert not mismatches, json.dumps(mismatches,ensure_ascii=True)
    for r in positives:
        i = r['caseId']
        assert 'nativeExpected' not in rows[i]
        assert same(rows[i]['request'], r['request'])
        assert same(contract(rows[i]['tsExpected']), contract(r['expected']))
        assert same(contract(rows[i]['tsExpected']), contract(actual[i])), i
    after = source(); assert before == after, 'source changed during execution'
    report = {'status':'passed','language':LANG, 'sourceFiles':before,
        'sourceSha256':sha(json.dumps(before,sort_keys=True,separators=(',',':')).encode()),
        'referenceCommit':manifest['referenceCommit'], 'baselineCommit':manifest['baselineCommit'],
        'requestSha256':sha(body), 'stdoutSha256':sha(result.stdout),'exitCode':result.returncode,'stderrBytes':len(result.stderr),
        'actualCount':len(actual),'positive80':80,'original1411':1411,'shared49':49,'adversarial39':39,
        'explicitNativeResiduals':[i for i,r in enumerate(rows) if 'nativeExpected' in r],
        'negativeControls':controls,'build':builds,'binary':binary,'sourceStable':True}
    args.output.parent.mkdir(parents=True,exist_ok=True); args.output.write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({k:v for k,v in report.items() if k not in ('sourceFiles','build','binary')}))
if __name__ == '__main__': main()
