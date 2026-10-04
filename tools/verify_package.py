#!/usr/bin/env python3
"""Verify a reproducible source archive, zero-dependency package and clean consumer."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

ROOT=Path(__file__).resolve().parents[1]
EXCLUDE={'.git','.tools','.baseline','artifacts','node_modules','__pycache__'}

def source_files(root):
 return sorted(p for p in root.rglob('*') if p.is_file() and not any(x in EXCLUDE for x in p.relative_to(root).parts) and p.suffix not in ('.pyc','.test','.out') and p.name not in ('specqr','specqr.exe','hello.png'))

def archive(root,path):
 with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=9) as z:
  for p in source_files(root):
   rel=p.relative_to(root).as_posix();info=zipfile.ZipInfo('SpecQR-Go/'+rel,(2026,1,1,0,0,0));info.compress_type=zipfile.ZIP_DEFLATED;info.external_attr=0o100644<<16;z.writestr(info,p.read_bytes())

def run(args,cwd,env):
 p=subprocess.run(args,cwd=cwd,env=env,text=True,capture_output=True,encoding='utf-8')
 if p.returncode:raise RuntimeError(repr(args)+'\n'+p.stdout+p.stderr)
 return p.stdout

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--go',default=os.environ.get('SPECQR_GO','go'));p.add_argument('--output',type=Path);p.add_argument('--archive',type=Path);a=p.parse_args()
 env=os.environ.copy();env.update(GOTOOLCHAIN='local',GOPROXY='off',GOSUMDB='off',GOWORK='off',GOFLAGS='-buildvcs=false');go=str(Path(shutil.which(a.go) or a.go).resolve())
 assert run([go,'list','-m','all'],ROOT,env).splitlines()==['github.com/SpecQR/SpecQR-Go']
 external=run([go,'list','-deps','-f','{{if not .Standard}}{{.ImportPath}}{{end}}','./...'],ROOT,env).splitlines()
 assert all(not s or s.startswith('github.com/SpecQR/SpecQR-Go') for s in external)
 assert run([go,'list','-f','{{.CgoFiles}}','./...'],ROOT,env).splitlines()==['[]']*len(run([go,'list','./...'],ROOT,env).splitlines())
 assert not (ROOT/'go.sum').exists();assert not run([go,'mod','tidy','-diff'],ROOT,env).strip()
 with tempfile.TemporaryDirectory(prefix='specqr-go-package-') as tmp:
  t=Path(tmp);one=t/'one.zip';two=t/'two.zip';archive(ROOT,one);archive(ROOT,two);assert one.read_bytes()==two.read_bytes()
  with zipfile.ZipFile(one) as z:
   for name in z.namelist():assert not Path(name).is_absolute() and '..' not in Path(name).parts
   z.extractall(t)
  source=t/'SpecQR-Go';run([go,'test','./...'],source,env)
  exe='specqr.exe' if os.name=='nt' else 'specqr';a1=t/('a-'+exe);a2=t/('b-'+exe)
  for target in (a1,a2):run([go,'build','-trimpath','-buildvcs=false','-o',str(target),'./cmd/specqr'],source,env)
  assert a1.read_bytes()==a2.read_bytes(), 'Same-toolchain builds must reproduce'
  env['GOBIN']=str(t/'installed');run([go,'install','-trimpath','-buildvcs=false','./cmd/specqr'],source,env);installed=t/'installed'/exe
  assert run([str(installed),'-version-info'],t,env).strip()=='0.1.0-rc.1'
  assert json.loads(run([str(installed),'-text','Detached 日本語 consumer','-format','json'],t,env))['matrix']
  (t/'bytes.bin').write_bytes(bytes([0,255,128]));run([str(installed),'-input','bytes.bin','-binary','-format','png','-output','bytes.png'],t,env);assert (t/'bytes.png').read_bytes().startswith(b'\x89PNG\r\n\x1a\n')
  c=t/'consumer';c.mkdir();(c/'go.mod').write_text('module example.com/clean-consumer\n\ngo 1.26.8\n\nrequire github.com/SpecQR/SpecQR-Go v0.0.0\nreplace github.com/SpecQR/SpecQR-Go => '+json.dumps(str(source))+'\n',encoding='utf-8')
  (c/'main.go').write_text('''package main
import("fmt";q "github.com/SpecQR/SpecQR-Go")
func main(){s,e:=q.Generate("Detached 日本語",q.Options{});if e!=nil{panic(e)};b,e:=s.ToPNG();if e!=nil||len(b)<8{panic(e)};raw,e:=q.GenerateBytes([]byte{0,255,128},q.Options{});if e!=nil||raw.Segments()[0].Count()!=3{panic(e)};fmt.Println(s.Size())}
''',encoding='utf-8')
  assert int(run([go,'run','.'],c,env).strip())>=21
  receipt={'status':'passed','compiler':run([go,'version'],t,env).strip(),'module':'github.com/SpecQR/SpecQR-Go','externalDependencies':[],'cgoFiles':[],'sourceArchiveSha256':hashlib.sha256(one.read_bytes()).hexdigest(),'fileCount':len(source_files(ROOT)),'sourceFilesSha256':{p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in source_files(ROOT)},'reproducibleArchive':True,'reproducibleBinary':True,'detachedTests':True,'installedCLI':True,'separateModuleConsumer':True}
  if a.archive:a.archive.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(one,a.archive)
 if a.output:a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({k:v for k,v in receipt.items() if k!='sourceFilesSha256'}))
if __name__=='__main__':main()
