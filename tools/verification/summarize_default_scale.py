#!/usr/bin/env python3
"""Publish a sanitized failed Java scale-8 diagnostic without masking failures.

A failed compatibility lane remains failed. This only verifies that its evidence
is complete, source-consistent, and isolated from seven separately strict lanes.
Unknown failures, mutated sources, changed counts, or non-identical controls fail
this summarizer rather than being documented away as expected limitations.
"""
import argparse
import hashlib
import json
from pathlib import Path
from verify_conformance import snapshot,tool_snapshot

EXPECTED={'alphanumeric-L-0':(4,'NotFoundException'),'alphanumeric-L-7':(4,'NotFoundException'),
          'utf8-H-0':(4,'NotFoundException'),'sa-2-15':(2,'ChecksumException')}

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--candidate',type=Path,required=True)
 p.add_argument('--report',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
 r=json.loads(a.report.read_text());corpus=r.get('javaDefaultScaleCorpus',{})
 if r.get('status')!='failed' or r.get('pngScale')!=8 or not r.get('scopeCompleted'):raise AssertionError('Diagnostic was not a completed failed default-scale8 run')
 if r.get('sourceSha256')!=snapshot(a.candidate,'all') or r.get('testToolSha256')!=tool_snapshot():raise AssertionError('Diagnostic source or tools are stale')
 if (corpus.get('status'),corpus.get('attempted'),corpus.get('successes'),corpus.get('failures'))!=('failed',446,442,4):raise AssertionError('Default-scale8 outcomes changed; investigate before documenting')
 if r['counts'].get('matrixDecodes')!=446 or r['counts'].get('pngDecodes')!=442:raise AssertionError('Decoder counts disagree with diagnostic')
 if len(r.get('failures',[]))!=5:raise AssertionError('Unexpected failures outside four detection misses and strict coverage mismatch')
 coverage=[f for f in r['failures'] if f.get('error')=='Strict decoder coverage count mismatch']
 if len(coverage)!=1:raise AssertionError('Missing strict-failure record')
 mismatch=coverage[0];actual=dict(mismatch['actualCounts']);actual['pngDecodes']=446
 if actual!=mismatch['expectedCounts']:raise AssertionError('A strict count other than PNG detections changed')
 misses={f['case']:f for f in r['failures'] if 'case' in f}
 if set(misses)!=set(EXPECTED):raise AssertionError('Different detector failures')
 controls=corpus.get('failureControls',[])
 if len(controls)!=4 or set(c['case'] for c in controls)!=set(EXPECTED):raise AssertionError('Missing independent PNG controls')
 for c in controls:
  version,error=EXPECTED[c['case']]
  if c['version']!=version or c['candidateOutcome']!={'error':error} or c['controlOutcome']!={'error':error}:raise AssertionError('Default-scale8 error changed')
  if not c.get('independentControlPixelsIdentical') or not c.get('matchingOutcomes') or c.get('countedAsStrictSuccess') is not False:raise AssertionError('Invalid independent control')
  if misses[c['case']].get('route')!='png' or misses[c['case']]['actual'].get('error')!=error:raise AssertionError('Failure record disagrees')
 if not corpus.get('allFailureControlsPixelIdentical') or not corpus.get('allFailureControlOutcomesMatch') or corpus.get('countedAsStrictScale3Success') is not False:raise AssertionError('Missing control guards')
 identity=r.get('goProcessIdentity',{})
 if identity.get('language')!='Go' or identity.get('runtimeDependencies')!=[]:raise AssertionError('Missing real Go identity')
 output={'status':'failed','scope':'Complete ZXing Java PNG detection at the Go renderer default scale 8; not part of the separately passing scale-3 lane.',
  'reportSha256':hashlib.sha256(a.report.read_bytes()).hexdigest(),'candidateBinarySha256':identity['binarySha256'],
  'sourceSha256':r['sourceSha256'],'testToolSha256':r['testToolSha256'],'decoder':r['decoder'],'jarSha256':r['jarSha256'],
  'javaRuntime':r['javaRuntime'],'javaExecutableSha256':r['javaExecutableSha256'],'pngScale':8,'pureBarcodeHint':False,
  'counts':r['counts'],'decodedVersions':r['decodedVersions'],'javaDefaultScaleCorpus':corpus,'failures':r['failures'],
  'elapsedSeconds':r['elapsedSeconds'],'conclusion':'Four actual Go PNG detection failures are independently reproduced by pixel-identical separately encoded controls. These failures remain failures; no matrix fallback, exclusion or scale substitution counts them as PNG successes.'}
 text=json.dumps(output,indent=2)+'\n'
 if any(x in text for x in ('/workspace/','/tmp/','/home/','/root/')):raise AssertionError('Unsanitized path')
 a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(text);print(a.output)
if __name__=='__main__':main()
