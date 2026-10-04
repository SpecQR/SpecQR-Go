#!/usr/bin/env python3
"""Build an offline, independent module consumer of a selected source artifact.

The copied adapter imports the actual SpecQR-Go package through Go module
resolution. It cannot access package-private symbols. Run all seven lanes with
its returned exact executable via --binary; this script never claims conformance.
"""
import argparse
import hashlib
import json
import os
import shutil
from pathlib import Path
import subprocess
from verify_conformance import snapshot

HERE=Path(__file__).resolve().parent

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--candidate',type=Path,required=True);p.add_argument('--consumer-dir',type=Path,required=True)
 p.add_argument('--go',default=os.environ.get('SPECQR_GO','go'));p.add_argument('--output',type=Path,required=True)
 a=p.parse_args();a.go=str(Path(shutil.which(a.go) or a.go).resolve());root=a.candidate.resolve();consumer=a.consumer_dir.resolve()
 if consumer==root or root in consumer.parents:raise ValueError('Consumer must be outside selected candidate tree')
 consumer.mkdir(parents=True,exist_ok=True)
 if any(consumer.iterdir()):raise ValueError('Consumer directory must be empty')
 before=snapshot(root,'all');driver=root/'tools/conformance/main.go'
 (consumer/'main.go').write_bytes(driver.read_bytes())
 (consumer/'go.mod').write_text('module example.com/specqr-offline-conformance-consumer\n\ngo 1.26.8\n\nrequire github.com/SpecQR/SpecQR-Go v0.0.0\n\nreplace github.com/SpecQR/SpecQR-Go => '+json.dumps(str(root))+'\n')
 env=os.environ.copy();env.update(GOPROXY='off',GOSUMDB='off',GOWORK='off',GOTOOLCHAIN='local')
 binary=consumer/('conformance-consumer.exe' if os.name=='nt' else 'conformance-consumer')
 subprocess.run([a.go,'build','-trimpath','-buildvcs=false','-o',str(binary),'.'],cwd=consumer,env=env,check=True)
 modules=subprocess.check_output([a.go,'list','-m','all'],cwd=consumer,env=env,text=True).splitlines()
 if len(modules)!=2 or not modules[1].startswith('github.com/SpecQR/SpecQR-Go v0.0.0 => '):raise AssertionError('Unexpected consumer module graph: '+repr(modules))
 build=subprocess.check_output([a.go,'version','-m',str(binary)],env=env,text=True)
 identity=json.loads(subprocess.check_output([str(binary)],input='{"command":"identity","nonce":"offline-module-consumer"}\n',text=True))
 if identity.get('runtimeDependencies')!=[] or len(identity.get('goModules',[]))!=1:raise AssertionError('Unexpected executable module graph')
 dep=identity['goModules'][0]
 if dep.get('path')!='github.com/SpecQR/SpecQR-Go' or Path(dep.get('replacePath','')).resolve()!=root:raise AssertionError('Wrong linked candidate')
 if snapshot(root,'all')!=before:raise AssertionError('Sources changed during consumer build')
 receipt={'status':'built-not-yet-conformance-tested','candidate':str(root),'sourceSha256':before,'consumer':str(consumer),'binary':str(binary),'binarySha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'compiler':subprocess.check_output([a.go,'version'],text=True).strip(),'modules':modules,'buildInfo':build,'identity':identity,'adapterSha256':hashlib.sha256(driver.read_bytes()).hexdigest()}
 a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(receipt,indent=2)+'\n');print(binary)

if __name__=='__main__':main()
