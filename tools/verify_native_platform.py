#!/usr/bin/env python3
"""Execute native prepublication gates; never substitute cross-compilation for a run.

No installer, network request, repository write, or publication is performed.
Logs and receipts go to an explicitly selected directory outside the source.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile

ROOT=Path(__file__).resolve().parents[1]

def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def fingerprint():return {p.relative_to(ROOT).as_posix():sha(p) for p in sorted(ROOT.rglob('*.go')) if not any(part in ('.git','.tools','node_modules') for part in p.relative_to(ROOT).parts)} | {'go.mod':sha(ROOT/'go.mod')}

def native_translation_check(host):
 if host == 'darwin':
  result=subprocess.run(['/usr/sbin/sysctl','-n','sysctl.proc_translated'],text=True,encoding='utf-8',capture_output=True)
  if result.returncode == 0:
   if result.stdout.strip() != '0':raise RuntimeError('Rosetta/translated process is not a native gate')
   return {'method':'sysctl.proc_translated','translated':False}
  if result.returncode == 1 and 'unknown oid' in result.stderr.lower():
   return {'method':'sysctl.proc_translated','supported':False,'reason':'OID absent on this native host; operator attestation remains required'}
  raise RuntimeError('Could not establish macOS translation status')
 if host == 'windows':
  import ctypes
  from ctypes import wintypes
  kernel=ctypes.WinDLL('kernel32',use_last_error=True)
  kernel.GetCurrentProcess.restype=wintypes.HANDLE
  process=kernel.GetCurrentProcess()
  if hasattr(kernel,'IsWow64Process2'):
   native=wintypes.USHORT();guest=wintypes.USHORT()
   fn=kernel.IsWow64Process2;fn.argtypes=[wintypes.HANDLE,ctypes.POINTER(wintypes.USHORT),ctypes.POINTER(wintypes.USHORT)];fn.restype=wintypes.BOOL
   if not fn(process,ctypes.byref(guest),ctypes.byref(native)):raise RuntimeError('IsWow64Process2 failed')
   if native.value != 0x8664 or guest.value != 0:raise RuntimeError('Required Windows gate needs un-translated native AMD64 Python/Go')
   return {'method':'IsWow64Process2','nativeMachine':native.value,'processMachine':guest.value,'translated':False}
  buffer=ctypes.create_string_buffer(64);kernel.GetNativeSystemInfo.argtypes=[ctypes.c_void_p];kernel.GetNativeSystemInfo(ctypes.byref(buffer));native=ctypes.cast(buffer,ctypes.POINTER(wintypes.WORD))[0]
  translated=wintypes.BOOL();fn=kernel.IsWow64Process;fn.argtypes=[wintypes.HANDLE,ctypes.POINTER(wintypes.BOOL)];fn.restype=wintypes.BOOL
  if not fn(process,ctypes.byref(translated)) or native != 9 or translated.value:raise RuntimeError('Required Windows gate needs native AMD64 process and OS')
  return {'method':'GetNativeSystemInfo+IsWow64Process','nativeArchitecture':native,'translated':False}
 return {'method':'matching host/process architecture plus operator attestation','instructionSetTranslationNotIndependentlyProvable':True}

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--expected-os',choices=['windows','darwin','linux'],required=True)
 p.add_argument('--go-min',required=True);p.add_argument('--go-current',required=True)
 p.add_argument('--output-dir',type=Path,required=True)
 p.add_argument('--attest-native-execution',action='store_true',required=True,help='Operator attests real native OS/architecture or same-architecture hardware-virtualized VM, with no instruction-set/full-system emulation')
 p.add_argument('--execute-linux-386',action='store_true',help='Also really execute Linux/386 tests and CLI; requires a compatible native Linux host')
 a=p.parse_args();host={'Windows':'windows','Darwin':'darwin','Linux':'linux'}.get(platform.system())
 if host!=a.expected_os:raise SystemExit('Expected native OS does not match this Python host')
 if a.execute_linux_386 and host!='linux':raise SystemExit('386 execution gate is Linux-specific')
 out=a.output_dir.resolve()
 if out==ROOT or ROOT in out.parents:raise SystemExit('Select an output directory outside source tree')
 if out.exists() and (not out.is_dir() or any(out.iterdir())):raise SystemExit('Output directory must be new or empty; existing evidence is never overwritten')
 out.mkdir(parents=True,exist_ok=True);before=fingerprint();receipt={'status':'running','nativeRunnerSha256':sha(Path(__file__).resolve()),'expectedOS':host,'hostMachine':platform.machine(),'python':platform.python_version(),'sourceGoSha256':before,'versions':[],'crossCompilationCountsAsNative':False,'operatorNativeExecutionAttestation':a.attest_native_execution,'attestationScope':'Actual native OS/architecture or same-architecture hardware-virtualized VM; no instruction-set/full-system emulation. Runtime checks cannot prove absence of arbitrary full-system emulation.'}
 receipt_path=out/'native-validation.json'
 def save():receipt_path.write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf-8')
 save()
 def command(args,env,log,cwd=ROOT):
  r=subprocess.run(args,cwd=cwd,env=env,text=True,encoding='utf-8',capture_output=True)
  log.write_text(r.stdout+r.stderr,encoding='utf-8')
  if r.returncode:raise RuntimeError('Gate failed: '+log.name+' (exit '+str(r.returncode)+')')
  return r.stdout
 try:
  receipt['nativeTranslationCheck']=native_translation_check(host);save()
  for expected,path in [('go1.26.8',a.go_min),('go1.27.1',a.go_current)]:
   go=Path(shutil.which(path) or path).resolve()
   if not go.is_file():raise RuntimeError('Go executable is missing')
   env=os.environ.copy()
   for key in ('GOOS','GOARCH','GOROOT','GOWORK','GOFLAGS'):env.pop(key,None)
   env.update(GOTOOLCHAIN='local',GOWORK='off',GOPROXY='off',GOSUMDB='off',PYTHONUTF8='1',CGO_ENABLED='1',GOFLAGS='-buildvcs=false')
   env['PATH']=str(go.parent)+os.pathsep+env.get('PATH','')
   d=out/expected;d.mkdir(exist_ok=True)
   info=json.loads(command([str(go),'env','-json','GOVERSION','GOOS','GOARCH','GOHOSTOS','GOHOSTARCH'],env,d/'toolchain.log'))
   if info['GOVERSION']!=expected or info['GOOS']!=host or info['GOHOSTOS']!=host or info['GOARCH']!=info['GOHOSTARCH']:raise RuntimeError('Wrong compiler/version/native target: '+repr(info))
   normalized={'x86_64':'amd64','AMD64':'amd64','arm64':'arm64','aarch64':'arm64','i386':'386','i686':'386'}.get(platform.machine(),platform.machine())
   if info['GOHOSTARCH']!=normalized:raise RuntimeError('Go host architecture differs from native Python host; do not label emulation native')
   if host=='windows' and info['GOARCH']!='amd64':raise RuntimeError('Required Windows race gate needs native amd64')
   row={'compiler':info,'goExecutableSha256':sha(go),'gates':{}};receipt['versions'].append(row);save()
   gates=[('normal',[str(go),'test','-count=1','./...']),('debug',[str(go),'test','-count=1','-gcflags=all=-N -l','./...']),('race',[str(go),'test','-race','-count=1','./...']),('vet',[str(go),'vet','./...'])]
   for name,args in gates:
    print(expected+' '+name,flush=True);command(args,env,d/(name+'.log'));row['gates'][name]='passed';save()
   clean=dict(env,CGO_ENABLED='0');command([str(go),'test','-count=1','./...'],clean,d/'no-cgo.log');row['gates']['CGODisabled']='passed'
   files=sorted(str(p) for p in ROOT.rglob('*.go') if not any(x in ('.git','.tools','node_modules') for x in p.relative_to(ROOT).parts))
   gofmt=go.with_name('gofmt.exe' if host=='windows' else 'gofmt');formatted=command([str(gofmt),'-l',*files],env,d/'gofmt.log')
   if formatted.strip():raise RuntimeError('Unformatted Go source')
   row['gates']['gofmt']='passed';command([str(go),'doc','-all','.'],env,d/'godoc.log');row['gates']['packageDocs']='passed'
   command([sys.executable,'tools/generate_idna_profile.py','--check'],env,d/'unicode-generator.log');row['gates']['unicodeGenerator']='passed'
   command([sys.executable,'tools/verify_package.py','--go',str(go),'--output',str(d/'package.json'),'--archive',str(d/'source.zip')],env,d/'package.log');row['gates']['packageCLIConsumerReproducibility']='passed'
   exe=d/('example.exe' if host=='windows' else 'example');command([str(go),'build','-trimpath','-buildvcs=false','-o',str(exe),'./examples/basic'],env,d/'example-build.log');command([str(exe)],env,d/'example-run.log',d)
   if not (d/'hello.png').read_bytes().startswith(b'\x89PNG\r\n\x1a\n'):raise RuntimeError('Example did not generate PNG')
   row['gates']['exampleExecution']='passed'
   if a.execute_linux_386:
    env32=dict(env,GOARCH='386',CGO_ENABLED='0');command([str(go),'test','-count=1','./...'],env32,d/'linux386-test.log');exe32=d/'specqr-linux386';command([str(go),'build','-trimpath','-buildvcs=false','-o',str(exe32),'./cmd/specqr'],env32,d/'linux386-build.log');value=json.loads(command([str(exe32),'-text','native 386 check','-format','json'],env32,d/'linux386-cli.log',d));assert value['matrix'];row['gates']['linux386Execution']='passed'
   row['logSha256']={p.name:sha(p) for p in sorted(d.glob('*.log'))};save()
  if fingerprint()!=before:raise RuntimeError('Source changed during validation')
  if sha(Path(__file__).resolve())!=receipt['nativeRunnerSha256']:raise RuntimeError('Native validation runner changed during execution')
  receipt['status']='passed';save();print('All requested native gates passed: '+str(receipt_path))
 except Exception as error:
  receipt['status']='failed-or-blocked';receipt['blocker']=str(error);save();raise

if __name__=='__main__':main()
