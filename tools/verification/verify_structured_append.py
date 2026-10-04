#!/usr/bin/env python3
"""Full Structured Append metadata/codewords and independently-oracled merge."""
import argparse
import copy
import functools
import hashlib
import json
import operator
from pathlib import Path
import secrets
import subprocess
import time
from protocol import configure, generate, verify_identity
from verify_conformance import snapshot, tool_snapshot, assert_typed_rejection

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[1]

def parity(data):
    return functools.reduce(operator.xor,data,0)

def compare(wanted,actual):
    if wanted!=actual:
        raise AssertionError(json.dumps({'expected':wanted,'actual':actual},ensure_ascii=True)[:3000])

def projection(actual):
    return {**{key:actual.get(key) for key in ('total','parity','inputLength','byteLength','diagnostics')},
            'symbols':[{'matrix':s.get('matrix'),'data_codewords':s.get('data'),'codewords':s.get('codewords')} for s in actual.get('symbols',[])]}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--candidate',type=Path,default=ROOT);p.add_argument('--baseline',type=Path,required=True)
    p.add_argument('--go');p.add_argument('--binary',type=Path);p.add_argument('--node',default='node')
    p.add_argument('--output',type=Path,default=ROOT/'artifacts/structured-append.json')
    args=p.parse_args();configure(args.go,args.binary);args.output.parent.mkdir(parents=True,exist_ok=True)
    before=snapshot(args.candidate,'all');tools=tool_snapshot();nonce=secrets.token_hex(24)
    identity=generate(args.candidate,[{'command':'identity','nonce':nonce}])[0];verify_identity(args.candidate,identity,nonce)
    reference=args.output.parent/'structured-append-reference.json'
    started=time.monotonic();report={'status':'running','sourceSha256':before,'testToolSha256':tools,'goProcessIdentity':identity,'counts':{'sets':0,'symbols':0,'validMerges':0,'rejectedMerges':0},'negativeControls':[]}
    try:
        subprocess.run([args.node,str(HERE/'structured-append-golden.mjs'),str(args.baseline),str(HERE/'structured-append-cases.json'),str(reference)],check=True)
        records=json.loads(reference.read_text())['cases'];requests=[{**row['request'],'includeMatrix':True,'includeDiagnostics':True} for row in records]
        actual=generate(args.candidate,requests)
        for record,result in zip(records,actual):
            compare(record['expected'],projection(result));report['counts']['sets']+=1;report['counts']['symbols']+=len(result['symbols'])
        if report['counts']['sets']!=22 or report['counts']['symbols']!=112:raise AssertionError('Truncated SA corpus')
        corrupted=generate(args.candidate,[requests[0]],fault='sa-diagnostics')[0]
        try:compare(records[0]['expected'],projection(corrupted))
        except AssertionError:report['negativeControls'].append({'fault':'go-sa-diagnostics','detected':True})
        else:raise AssertionError('SA diagnostics mutation escaped detection')
        valid=[]
        for total in range(2,17):
            for binary in (False,True):
                values=[bytes([0,index,255,128,total]) if binary else f'{index}:e\u0301🙂漢字' for index in range(total)]
                check=parity(b''.join(v if binary else v.encode() for v in values))
                parts=[{'index':i+1,'total':total,'parity':check,('bytes' if binary else 'text'):list(v) if binary else v} for i,v in enumerate(values)]
                valid.append({'command':'merge','parts':parts[::-1]})
        invalid=[]
        base=valid[0]
        invalid.append({'command':'merge','parts':[]})
        for mode in ('missing','duplicate','total','parity','payload','mixed','index0','index17','total1','parity256'):
            r=copy.deepcopy(base)
            if mode=='missing':r['parts'].pop()
            elif mode=='duplicate':r['parts'][1]=copy.deepcopy(r['parts'][0])
            elif mode=='total':r['parts'][0]['total']=3
            elif mode=='parity':r['parts'][0]['parity']^=1
            elif mode=='payload':r['parts'][0]['text']+='x'
            elif mode=='mixed':r['parts'][0]['bytes']=list(r['parts'][0].pop('text').encode())
            elif mode=='index0':r['parts'][0]['index']=0
            elif mode=='index17':r['parts'][0]['index']=17
            elif mode=='total1':r['parts'][0]['total']=1
            elif mode=='parity256':r['parts'][0]['parity']=256
            invalid.append(r)
        all_requests=valid+invalid
        expected=list(map(json.loads,subprocess.check_output([args.node,str(HERE/'merge-oracle.mjs'),str(args.baseline)],input=''.join(json.dumps(r,ensure_ascii=True)+'\n' for r in all_requests),text=True).splitlines()))
        if len(expected)!=len(all_requests):raise AssertionError('Merge oracle response truncated')
        output=generate(args.candidate,all_requests)
        for i,(want,got) in enumerate(zip(expected,output)):
            if i<len(valid):compare(want,got);report['counts']['validMerges']+=1
            else:
                if 'error' not in want:raise AssertionError('Malformed merge unexpectedly accepted by baseline')
                assert_typed_rejection(got);report['counts']['rejectedMerges']+=1
        try:compare(expected[0],generate(args.candidate,[valid[0]],fault='merge-data')[0])
        except AssertionError:report['negativeControls'].append({'fault':'go-merged-payload','detected':True})
        else:raise AssertionError('Merge mutation escaped detection')
        if report['counts']!={'sets':22,'symbols':112,'validMerges':30,'rejectedMerges':11}:raise AssertionError('Wrong strict SA coverage')
        if before!=snapshot(args.candidate,'all') or tools!=tool_snapshot():raise AssertionError('Sources/tools changed during SA verification')
        end=generate(args.candidate,[{'command':'identity','nonce':nonce}])[0];verify_identity(args.candidate,end,nonce)
        if end['binarySha256']!=identity['binarySha256']:raise AssertionError('Go executable changed')
        report.update(status='passed',referenceSha256=hashlib.sha256(reference.read_bytes()).hexdigest(),mergeOracleSha256=hashlib.sha256(json.dumps(expected,sort_keys=True).encode()).hexdigest())
    except Exception as error:report.update(status='failed',error=f'{type(error).__name__}: {error}');raise
    finally:
        report['elapsedSeconds']=round(time.monotonic()-started,3);args.output.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))

if __name__=='__main__':main()
