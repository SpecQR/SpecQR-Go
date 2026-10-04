#!/usr/bin/env python3
"""Exact bounded GS1 audit, including separately reviewed Go URL outcomes.

The live reference is pinned JavaScript, never Kotlin. Profile fixtures contain
all reviewed Node 24.19/24.21 outcomes, plus independently justified Go-specific
URL expectations. An unknown mismatch always fails; categories are not waivers.
"""
import argparse
import collections
import copy
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time
from protocol import configure, generate, verify_identity
from verify_conformance import snapshot, tool_snapshot
from gs1.corpus import build_cases
from gs1.go_profile import build as build_go_profile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
PIN = '15ad15e5c770ea0e39072f8f88b2733018f02ffd'
REF_FIXTURE = HERE / 'gs1/reference-url-outcomes.json'
GO_FIXTURE = HERE / 'gs1/go-url-outcomes.json'


def sha(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()).hexdigest()


def node_identity(node):
    result = json.loads(subprocess.check_output([node, '-p', 'JSON.stringify({executable:process.execPath,versions:process.versions,platform:process.platform,architecture:process.arch})'], text=True))
    result['executableSha256'] = hashlib.sha256(Path(result['executable']).read_bytes()).hexdigest()
    return result


def comparison_kind(operation, expected, actual):
    if expected == actual:
        return None
    if not isinstance(actual, dict) or expected.get('ok') != actual.get('ok'):
        return 'acceptance'
    return 'value' if expected.get('ok') and operation != 'validate' else 'diagnostic'


def exact_check(cases, references, actual, reference_fixture, go_fixture, version):
    if len(cases) != 5610 or len(references) != len(cases) or len(actual) != len(cases):
        raise AssertionError('GS1 case/response count is not exactly 5610')
    known = [(case, expected) for case, expected in zip(cases, references) if case['category'] in ('idna', 'dot')]
    if len(known) != 54:
        raise AssertionError('Missing IDNA/dot reference outcomes')
    for (case, expected), row in zip(known, reference_fixture['cases']):
        if case != row['case'] or expected != row['reference'][version]:
            raise AssertionError('Reviewed Node reference outcome changed: ' + repr(case))
    overrides = {sha(row['case']): row for row in go_fixture['cases']}
    if go_fixture.get('reviewStatus') != 'reviewed' or not go_fixture.get('policy'):
        raise AssertionError('Go URL profile has not been independently reviewed')
    seen, operations = set(), 0
    for case, expected, got in zip(cases, references, actual):
        key = sha(case)
        if key in overrides:
            override = overrides[key]
            if not override.get('reason'):
                raise AssertionError('Go override lacks a source-policy reason')
            wanted = override['go']
            seen.add(key)
        else:
            wanted = expected
        operations += len(expected)
        if got != wanted:
            raise AssertionError('Unexpected Go GS1 outcome: ' + json.dumps({'case': case, 'expected': wanted, 'actual': got}, ensure_ascii=True)[:2500])
    if seen != set(overrides):
        raise AssertionError('Go profile has stale cases outside this exact corpus')
    if operations != 15690:
        raise AssertionError(f'GS1 operation count changed: {operations}')
    return {'passed': True, 'cases': len(cases), 'operations': operations, 'referenceProfile': version,
            'exactReferenceUrlCases': len(known), 'explicitGoOverrideCases': len(overrides),
            'goOutcomesSha256': sha(actual), 'referenceOutcomesSha256': sha(references)}


def verify_host_safety(candidate, node, baseline):
    fixture = json.loads((HERE / 'gs1/host-safety-cases.json').read_text())
    rejected = {'parse': {'ok': False, 'code': 'INVALID_GS1'}, 'normalize': {'ok': False, 'code': 'INVALID_GS1'},
                'validate': {'ok': False, 'errors': [{'code': 'GS1_DIGITAL_LINK_INVALID_URI', 'reason': 'invalid-uri',
                    'ai': None, 'value': None, 'key': None, 'offset': None, 'elementIndex': None,
                    'expected': 'absolute http or https URL', 'count': None}], 'warnings': []}}
    requests, expected = [], []
    for host in fixture['reject']:
        requests.extend([{'command': 'url', 'input': 'https://' + host + '/01/04912345678904'},
                         {'command': 'create', 'elements': [{'ai': '01', 'value': '04912345678904'}], 'baseUrl': 'https://' + host}])
        expected.extend([copy.deepcopy(rejected), {'create': {'ok': False, 'code': 'INVALID_GS1'}}])
    for entry in fixture['accept']:
        for host in (entry['raw'], entry['ascii']):
            uri = 'https://' + host + '/01/04912345678904'
            canonical = 'https://' + entry['ascii'] + '/01/04912345678904'
            requests.extend([{'command': 'url', 'input': uri}, {'command': 'create', 'elements': [{'ai': '01', 'value': '04912345678904'}], 'baseUrl': 'https://' + host}])
            expected.extend([{'parse': {'ok': True, 'elements': [['01', '04912345678904']], 'path': [['01', '04912345678904']], 'query': [], 'unknown': []},
                              'normalize': {'ok': True, 'value': canonical}, 'validate': {'ok': True, 'errors': [], 'warnings': []}},
                             {'create': {'ok': True, 'value': canonical}}])
    actual = generate(candidate, requests)
    for request, wanted, got in zip(requests, expected, actual):
        if got != wanted:
            raise AssertionError('Conservative host profile failed: ' + json.dumps({'request': request, 'expected': wanted, 'actual': got}, ensure_ascii=True))
    body = ''.join(json.dumps(r, ensure_ascii=True) + '\n' for r in requests)
    references = list(map(json.loads, subprocess.check_output([node, str(HERE / 'gs1/oracle.mjs'), str(baseline)], input=body, text=True).splitlines()))
    if len(requests) != 78 or len(references) != 78 or sum(map(len, expected)) != 156:
        raise AssertionError('Conservative host coverage changed')
    differences = [{'request': request, 'reference': reference, 'go': got} for request, reference, got in zip(requests, references, actual) if reference != got]
    return {'passed': True, 'hostVariants': 39, 'requests': 78, 'operations': 156,
            'outcomesSha256': sha(actual), 'referenceOutcomesSha256': sha(references), 'referenceDifferenceCases': len(differences),
            'differences': differences}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', type=Path, default=ROOT)
    parser.add_argument('--baseline', type=Path, required=True)
    parser.add_argument('--go', default=None)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--node', default=os.environ.get('NODE', 'node'))
    parser.add_argument('--node-profile', choices=['24.19.0', '24.21.0'])
    parser.add_argument('--output', type=Path, default=ROOT / 'artifacts/gs1.json')
    args = parser.parse_args()
    configure(args.go, args.binary)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    baseline = args.baseline.resolve()
    started = time.monotonic()
    before, tools_before = snapshot(args.candidate, 'all'), tool_snapshot()
    reference_fixture = json.loads(REF_FIXTURE.read_text())
    go_fixture = json.loads(GO_FIXTURE.read_text()) if GO_FIXTURE.exists() else {'reviewStatus': 'unreviewed', 'cases': []}
    if go_fixture != build_go_profile():
        raise AssertionError('Go finite expected profile differs from independent policy construction')
    node_before = node_identity(args.node)
    version = node_before['versions']['node']
    profile = reference_fixture['profiles'].get(version)
    if not profile or (args.node_profile and args.node_profile != version):
        raise AssertionError('Unreviewed or unexpected exact Node runtime: ' + version)
    for name, value in profile['runtimeVersions'].items():
        if node_before['versions'].get(name) != value:
            raise AssertionError('Node reference runtime component changed: ' + name)
    revision = subprocess.check_output(['git', '-C', str(baseline), 'rev-parse', 'HEAD'], text=True).strip()
    if revision != PIN:
        raise AssertionError('Wrong reference commit: ' + revision)
    subprocess.run(['git', '-C', str(baseline), 'diff', '--quiet', 'HEAD', '--', 'src', 'package.json'], check=True)
    baseline_hashes = {str(p.relative_to(baseline)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((baseline / 'src').rglob('*.js'))}
    cases, body = build_cases()
    corpus_sha = hashlib.sha256(body.encode()).hexdigest()
    if corpus_sha != reference_fixture['corpusSha256']:
        raise AssertionError('GS1 corpus changed')
    nonce = secrets.token_hex(24)
    identity = generate(args.candidate, [{'command': 'identity', 'nonce': nonce}])[0]
    verify_identity(args.candidate, identity, nonce)
    report = {'status': 'running', 'candidateLanguage': 'Go', 'candidateIdentity': identity,
              'baselineCommit': PIN, 'baselineSourceSha256': baseline_hashes,
              'sourceSha256': before, 'testToolSha256': tools_before, 'nodeIdentity': node_before,
              'referenceProfile': version, 'corpusSha256': corpus_sha, 'limitations': ['Finite bounded corpus, not GS1 certification. Human-readable diagnostic message prose is excluded.']}
    try:
        process = subprocess.run([args.node, str(HERE / 'gs1/oracle.mjs'), str(baseline)], input=body, capture_output=True, text=True, check=True, timeout=180)
        references = [json.loads(line) for line in process.stdout.splitlines()]
        requests = [{k:v for k,v in case.items() if k != 'category'} for case in cases]
        actual = []
        for offset in range(0, len(requests), 256):
            actual.extend(generate(args.candidate, requests[offset:offset+256]))
        stats, differences = collections.Counter(), []
        for case, expected, got in zip(cases, references, actual):
            stats['cases'] += 1
            stats['cases_' + case['category']] += 1
            for operation, result in expected.items():
                stats['operations'] += 1
                stats['operations_' + case['category']] += 1
                kind = comparison_kind(operation, result, got.get(operation, {}))
                if kind is None:
                    stats['matched'] += 1
                else:
                    stats['difference_' + kind] += 1
                    stats['difference_' + case['category'] + '_' + kind] += 1
                    differences.append({'case': case, 'operation': operation, 'kind': kind, 'js': result, 'go': got.get(operation)})
        report.update(stats=dict(stats), differences=differences, cases=cases, reference=references, go=actual)
        report['exactOutcomeCheck'] = exact_check(cases, references, actual, reference_fixture, go_fixture, version)
        controls = []
        for label, side in [('candidate-catalog-value', 'go'), ('reference-known-url-value', 'reference'), ('same-count-candidate-dot-uri', 'go-dot')]:
            bad_go, bad_ref = copy.deepcopy(actual), copy.deepcopy(references)
            if side == 'go':
                # The process must run the Go API before its test-only corruption.
                bad_go[0] = generate(args.candidate, [requests[0]], fault='gs1-catalog')[0]
            elif side == 'go-dot':
                index = next(i for i, case in enumerate(cases) if case['category'] == 'dot' and case['command'] == 'url')
                bad_go[index]['normalize'] = {'ok': True, 'value': 'https://wrong-candidate.example/01/04912345678904'}
                old_kind = comparison_kind('normalize', references[index]['normalize'], actual[index]['normalize'])
                new_kind = comparison_kind('normalize', references[index]['normalize'], bad_go[index]['normalize'])
                if old_kind != new_kind:
                    raise AssertionError('Same-count mutation must preserve the existing difference kind')
            else:
                index = next(i for i, case in enumerate(cases) if case['category'] == 'idna')
                bad_ref[index]['normalize'] = {'ok': True, 'value': 'https://wrong.example/01/04912345678904'}
            try:
                exact_check(cases, bad_ref, bad_go, reference_fixture, go_fixture, version)
            except AssertionError as error:
                controls.append({'fault': label, 'detected': True, 'detail': str(error)[:250]})
            else:
                raise AssertionError('GS1 mutation negative control was accepted: ' + label)
        report['negativeControls'] = controls
        report['hostSafetyProfile'] = verify_host_safety(args.candidate, args.node, baseline)
        if snapshot(args.candidate, 'all') != before or tool_snapshot() != tools_before:
            raise AssertionError('Candidate or harness sources changed while GS1 ran')
        end_identity = generate(args.candidate, [{'command': 'identity', 'nonce': nonce}])[0]
        verify_identity(args.candidate, end_identity, nonce)
        if identity['binarySha256'] != end_identity['binarySha256'] or node_identity(args.node) != node_before:
            raise AssertionError('Go or Node executable changed while GS1 ran')
        for name, wanted in baseline_hashes.items():
            if hashlib.sha256((baseline / name).read_bytes()).hexdigest() != wanted:
                raise AssertionError('Reference changed while GS1 ran: ' + name)
        report['status'] = 'passed'
    except Exception as error:
        report.update(status='failed', error=f'{type(error).__name__}: {error}')
        raise
    finally:
        report['elapsedSeconds'] = round(time.monotonic() - started, 3)
        args.output.write_text(json.dumps(report, ensure_ascii=True, indent=2) + '\n')
        print(json.dumps({k:v for k,v in report.items() if k not in ('cases','reference','go','differences','sourceSha256','testToolSha256')}, indent=2))

if __name__ == '__main__':
    main()
