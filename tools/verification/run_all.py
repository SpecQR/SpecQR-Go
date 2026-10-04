#!/usr/bin/env python3
"""Run all seven strict verifier lanes; fail if any lane or source snapshot fails."""
import argparse
import os
from pathlib import Path
import subprocess
import sys

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[1]

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--candidate',type=Path,default=ROOT);p.add_argument('--baseline',type=Path,required=True)
 p.add_argument('--go',default=os.environ.get('SPECQR_GO','go'));p.add_argument('--binary',type=Path)
 p.add_argument('--node',default='node');p.add_argument('--node-24-19',required=True);p.add_argument('--node-24-21',required=True)
 p.add_argument('--cpp-dependency-dir',type=Path,required=True);p.add_argument('--java-dependency-dir',type=Path,required=True)
 p.add_argument('--reports',type=Path,required=True);p.add_argument('--summary',type=Path,required=True)
 p.add_argument('--no-install',action='store_true')
 a=p.parse_args();a.reports.mkdir(parents=True,exist_ok=True)
 base=['--candidate',str(a.candidate.resolve()),'--go',a.go]
 if a.binary:base+=['--binary',str(a.binary.resolve())]
 lanes=[('conformance','verify_conformance.py',['--baseline',str(a.baseline),'--node',a.node]),
  ('structured-append','verify_structured_append.py',['--baseline',str(a.baseline),'--node',a.node]),
  ('jsqr','verify_jsqr.py',['--node',a.node]),
  ('zxing-cpp','verify_decoders.py',['--decoder','cpp','--dependency-dir',str(a.cpp_dependency_dir)]),
  ('zxing-java','verify_decoders.py',['--decoder','java','--dependency-dir',str(a.java_dependency_dir)]),
  ('gs1-24.19','verify_gs1.py',['--baseline',str(a.baseline),'--node',a.node_24_19,'--node-profile','24.19.0']),
  ('gs1-24.21','verify_gs1.py',['--baseline',str(a.baseline),'--node',a.node_24_21,'--node-profile','24.21.0'])]
 for name,script,extra in lanes:
  command=[sys.executable,str(HERE/script),*base,*extra,'--output',str(a.reports/(name+'.json'))]
  if a.no_install and script=='verify_decoders.py':command+=['--no-install']
  print('Running '+name,flush=True)
  with (a.reports/(name+'.log')).open('w') as log:
   subprocess.run(command,stdout=log,stderr=subprocess.STDOUT,check=True)
 subprocess.run([sys.executable,str(HERE/'summarize_reports.py'),'--candidate',str(a.candidate),'--reports',str(a.reports),'--output',str(a.summary)],check=True)

if __name__=='__main__':main()
