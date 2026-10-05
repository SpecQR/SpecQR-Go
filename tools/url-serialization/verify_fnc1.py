#!/usr/bin/env python3
"""All 102 original FNC1 requests plus four manual and 68 real-PNG controls."""
import argparse, importlib.util, json, os, pathlib, subprocess, sys
from verify import ROOT, HERE, LANG, sha, strict, same, source, process

def main():
    p=argparse.ArgumentParser();p.add_argument('--adapter',type=pathlib.Path);p.add_argument('--jar',type=pathlib.Path);p.add_argument('--dependency-dir',type=pathlib.Path,required=True);p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/fnc1-preservation.json');a=p.parse_args()
    before=source();manifest=strict((HERE/'fixtures/manifest.json').read_bytes())
    refpath=HERE/'fixtures/fnc1-native-reference.json';assert sha(refpath.read_bytes())==manifest['sha256'][refpath.name]
    fixture=strict(refpath.read_bytes());rows=fixture['rows'];assert len(rows)==174
    tool=ROOT/('tools' if LANG=='CPP' else 'tools/conformance' if LANG in ('Java','Kotlin') else 'tools/verification')
    sys.path.insert(0,str(tool));import protocol
    if LANG in ('Java','Kotlin'):
        protocol.configure(jar=a.jar);cp=protocol._classpath(ROOT)
        command=[protocol.JAVA,'-XX:ActiveProcessorCount=2','-Xmx512m','-Dfile.encoding=UTF-8','--class-path',cp,'io.specqr.ConformanceDriver']
        binaries={str(f):sha(f.read_bytes()) for part in cp.split(os.pathsep) for f in ([pathlib.Path(part)] if pathlib.Path(part).is_file() else pathlib.Path(part).rglob('*.class'))}
    elif LANG in ('Go','Rust'):
        protocol.configure(binary=a.adapter);binary=protocol._binary(ROOT);command=[str(binary),'--json-lines'];binaries={str(binary):sha(binary.read_bytes())}
    else:
        assert a.adapter and a.adapter.is_file();command=[str(a.adapter.resolve())];binaries={str(a.adapter.resolve()):sha(a.adapter.read_bytes())}
    requests=[r['request'] for r in rows]
    body=('\n'.join(protocol.request_line(r) for r in requests)+'\n').encode() if LANG=='CPP' else b''.join((json.dumps(r,ensure_ascii=True)+'\n').encode() for r in requests)
    run=subprocess.run(command,input=body,capture_output=True,timeout=600);actual=process(run.returncode,run.stderr,run.stdout,len(rows))
    decoderfile=tool/('verify-decode.py' if LANG=='CPP' else 'verify_decoders.py')
    spec=importlib.util.spec_from_file_location('png_verifier',decoderfile);png=importlib.util.module_from_spec(spec);spec.loader.exec_module(png)
    requirements=tool/'zxing-cpp-requirements.txt';assert (a.dependency_dir/'.verified-requirements-sha256').read_text().strip()==sha(requirements.read_bytes())
    sys.path.insert(0,str(a.dependency_dir));import zxingcpp
    counts={'originalVectors':102,'manualVectors':4,'relaxedByteControls':68,'exactSymbols':0,'typedErrors':0,'realPngDecodes':0,'verifiedPixels':0}
    for row,result in zip(rows,actual):
        expected=row['expected'];request=row['request']
        if expected.get('nativeForcedAlpha'):
            code='INVALID_GS1' if LANG=='CPP' and request['options'].get('gs1') else 'INVALID_MODE'
        else:code=expected.get('errorCode')
        if code:
            assert result.get('error' if LANG=='CPP' else 'code')==code,(row['id'],code,result)
            if LANG!='CPP':assert result.get('isSpecQRError') is True,(row['id'],result)
            counts['typedErrors']+=1;continue
        for key,value in expected.items():assert same(result.get(key),value),(row['id'],key,value,result)
        counts['exactSymbols']+=1
        if 'pngScale' in request:
            data,luminance,size=png.verify_png(result['png'],result['matrix'],8)
            decoded=zxingcpp.read_barcode(memoryview(luminance).cast('B',shape=(size,size)),text_mode=zxingcpp.TextMode.Plain)
            second=request['options'].get('fnc1Second')
            for seg in request.get('segments',[]):
                if seg['mode']=='fnc1-second':second=seg['applicationIndicator']
            payload=(second.encode() if second else b'')+bytes.fromhex(row['payloadHex'])
            assert decoded is not None and decoded.valid and decoded.bytes==payload,(row['id'],None if decoded is None else decoded.bytes.hex(),payload.hex())
            assert decoded.symbology_identifier==(']Q5' if second else ']Q3'),row['id']
            counts['realPngDecodes']+=1;counts['verifiedPixels']+=size*size
    blank=bytearray([255])*232*232
    assert zxingcpp.read_barcode(memoryview(blank).cast('B',shape=(232,232))) is None
    assert source()==before,'source changed'
    assert all(sha(pathlib.Path(f).read_bytes())==h for f,h in binaries.items()),'binary changed'
    report={'status':'passed','language':LANG,'counts':counts,'fixtureSha256':sha(refpath.read_bytes()),'referenceCommit':fixture['referenceCommit'],'sourceFiles':before,'binaries':binaries,'stdoutSha256':sha(run.stdout),'exitCode':run.returncode,'stderrBytes':len(run.stderr),'blankControl':True,'sourceStable':True}
    a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({k:v for k,v in report.items()if k not in ('sourceFiles','binaries')}))
if __name__=='__main__':main()
