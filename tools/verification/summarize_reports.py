#!/usr/bin/env python3
"""Create a sanitized, source-consistent public verification summary.

All seven required lanes must have passed against exactly the current candidate
sources and harness. This never converts a partial, failed or stale run to pass.
"""
import argparse
import hashlib
import json
from pathlib import Path
from verify_conformance import snapshot,tool_snapshot

ROOT=Path(__file__).resolve().parents[2]
LANES=['conformance','structured-append','jsqr','zxing-cpp','zxing-java','gs1-24.19','gs1-24.21']

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--candidate',type=Path,default=ROOT);p.add_argument('--reports',type=Path,required=True);p.add_argument('--output',type=Path,required=True);args=p.parse_args()
    source=snapshot(args.candidate,'all');tools=tool_snapshot();lanes={};identities=set()
    for name in LANES:
        path=args.reports/(name+'.json');r=json.loads(path.read_text())
        if r.get('status')!='passed':raise AssertionError(f'{name} is not passed')
        if r.get('sourceSha256')!=source:raise AssertionError(f'{name} is stale or from different source files')
        if r.get('testToolSha256')!=tools:raise AssertionError(f'{name} is stale or from different test tools')
        if name=='conformance' and not r.get('completeConformanceScope'):raise AssertionError('Scoped conformance does not satisfy complete verification')
        identity=r.get('goProcessIdentity',r.get('candidateIdentity'))
        if not identity or identity.get('language')!='Go' or identity.get('runtimeDependencies')!=[]:raise AssertionError('Missing Go subject identity: '+name)
        identities.add((identity['binarySha256'],identity.get('goVersion')))
        entry={'status':'passed','reportSha256':hashlib.sha256(path.read_bytes()).hexdigest(),'elapsedSeconds':r['elapsedSeconds'],'candidateBinarySha256':identity['binarySha256']}
        for key in ('counts','stats','exactOutcomeCheck','negativeControls','malformedNegativeControls','decodedVersions','blankImageNegativeControl','strictSymbols','pngScale','pureBarcodeHint','referenceProfile','referenceSha256','mergeOracleSha256','corpusSha256','eciApiObservation','hostSafetyProfile','limitations'):
            if key in r:entry[key]=r[key]
        if name.startswith('gs1'):
            entry['nodeRuntimeVersions']=r['nodeIdentity']['versions']
            entry['nodeExecutableSha256']=r['nodeIdentity']['executableSha256']
            entry['differences']=[{'case':v['case'],'operation':v['operation'],'kind':v['kind'],'js':v['js'],'go':v['go']} for v in r['differences']]
        if 'defaultScaleDiagnostic' in r:
            entry['defaultScaleDiagnostic']=r['defaultScaleDiagnostic']
        lanes[name]=entry
    if len(identities)!=1:raise AssertionError('Required lanes used different Go binaries or compiler versions')
    candidate_binary,go_version=next(iter(identities))
    output={'status':'passed','goVersion':go_version,'candidateBinarySha256':candidate_binary,'candidateLanguage':'Go','runtimeGoDependencies':0,'baselineCommit':'15ad15e5c770ea0e39072f8f88b2733018f02ffd','sourceSha256':source,'testToolSha256':tools,'lanes':lanes,'limitations':['Finite deterministic tests, not ISO/GS1 or print/camera certification.','Compiler/OS matrices, go package and offline clean-consumer checks require separate evidence.','GS1 URL compatibility is the reviewed bounded Go profile, not universal UTS46/browser parity.','ZXing Java strict PNG coverage uses scale 3. Its full default-scale8 compatibility diagnostic is separately retained as failed, with independently reproduced pixel-identical controls.']}
    text=json.dumps(output,indent=2,ensure_ascii=True)+'\n'
    for forbidden in ('/workspace/','/tmp/','/home/','/root/'):
        if forbidden in text:raise AssertionError('Unsanitized local path in summary: '+forbidden)
    args.output.parent.mkdir(parents=True,exist_ok=True);args.output.write_text(text);print(args.output)

if __name__=='__main__':main()
