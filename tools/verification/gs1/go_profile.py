"""Independent finite expected Go URL profile, constructed without a candidate.

Expected outputs are derived from the documented bounded host/GS1 contracts.
Python's standard-library RFC3492 codec independently produces Unicode-host
ACE labels; no previous-language candidate response is read or imported.
"""
import json
from pathlib import Path
from urllib.parse import unquote, urlsplit
try:
 from .corpus import build_cases
except ImportError:
 from corpus import build_cases

BASE='https://example.com/01/04912345678904'
PRIMARY=['01','04912345678904']
REJECTED_ACE={
 'xn--a':'ACE decodes to U+0080 control.',
 'xn--':'Empty ACE payload.',
 'xn--abc':'ACE decodes to forbidden control scalars.',
 'xn--abc-':'Noncanonical all-ASCII ACE result.',
 'xn--a-ecp.ru':'ACE includes U+2488, rejected by the frozen compatibility-unstable profile.',
 'xn--0.pt':'Truncated RFC3492 digit sequence.',
 'xn--a.test':'ACE decodes to U+0080 control.',
 'xn--a_.test':'Underscore is not an RFC3492 encoded digit.',
 'xn--%61.test':'Percent-decoded ACE decodes to U+0080 control.',
}
REJECTED_UNICODE={'a\u200cb.test','a\u200db.test','\u0600.test'}

def invalid():
 return {'parse':{'ok':False,'code':'INVALID_GS1'},'normalize':{'ok':False,'code':'INVALID_GS1'},'validate':{'ok':False,'errors':[{'code':'GS1_DIGITAL_LINK_INVALID_URI','reason':'invalid-uri','ai':None,'value':None,'key':None,'offset':None,'elementIndex':None,'expected':'absolute http or https URL','count':None}],'warnings':[]}}

def accepted(uri, path=None, query=None):
 path=path if path is not None else [PRIMARY]
 query=query if query is not None else []
 return {'parse':{'ok':True,'elements':path+query,'path':path,'query':query,'unknown':[]},'normalize':{'ok':True,'value':uri},'validate':{'ok':True,'errors':[],'warnings':[]}}

def host_expected(host):
 if host in REJECTED_ACE:return invalid(),REJECTED_ACE[host]+' Bounded Go profile rejects this independently reviewed case.'
 if host in REJECTED_UNICODE:return invalid(),'Forbidden joiner or format scalar is rejected by the bounded Unicode 15.0 profile.'
 mapped=host.translate({0x3002:46,0xff0e:46,0xff61:46,0x00ad:None})
 mapped=''.join(chr(ord(c)-0xfee0) if 0xff01<=ord(c)<=0xff5e else c for c in mapped).lower()
 labels=[]
 for label in mapped.split('.'):
  labels.append('xn--'+label.encode('punycode').decode('ascii') if not label.isascii() else label)
 return accepted('https://'+'.'.join(labels)+'/01/04912345678904'),'Explicit dot/fullwidth/soft-hyphen mapping, Unicode 15 lowercase and independent Python stdlib RFC3492 encoding. Existing canonical ACE labels are unchanged.'

def build():
 cases,_=build_cases();rows=[]
 for case in cases:
  category=case['category'];inp=case.get('input','')
  if category=='idna':
   host=inp[len('https://'):].split('/')[0];wanted,reason=host_expected(host)
  elif category=='dot':
   reason='Lossless GS1 dot-only values remain data elements; canonical creation emits them in the query to avoid URL path traversal normalization.'
   if case['command']=='create':wanted={'create':{'ok':True,'value':BASE+'?10='+case['elements'][1]['value']}}
   else:
    suffix=inp[len(BASE):]
    if suffix.startswith('?10='):
     value=unquote(suffix[4:]);wanted=accepted(BASE+'?10='+value,[PRIMARY],[['10',value]])
    else:
     pieces=suffix.split('/');value=unquote(pieces[2]);path=[PRIMARY,['10',value]]
     extra='/21/S' if suffix.endswith('/21/S') else ''
     if extra:path.append(['21','S'])
     wanted=accepted(BASE+extra+'?10='+value,path,[])
  elif '\ud800' in inp:
   wanted={'error':'SpecQrError','message':'Unpaired UTF-16 surrogate','isSpecQRError':True,'code':'INVALID_INPUT'}
   reason='Strict development JSON boundary rejects unpaired UTF-16 instead of encoding/json replacement. Go string API accepts only valid UTF-8.'
  else:continue
  rows.append({'case':case,'go':wanted,'reason':reason})
 assert len(rows)==55
 return {'schemaVersion':1,'reviewStatus':'reviewed','policy':'Finite independently constructed expected Go outcomes: explicit conservative Unicode15 host profile, Python standard-library RFC3492 calculation, canonical ACE rejection reasons, lossless GS1 dot data and strict Unicode input domain. No candidate output or earlier-language subject is read. All other corpus outcomes must equal the live pinned JavaScript reference.','cases':rows}

if __name__=='__main__':
 output=Path(__file__).with_name('go-url-outcomes.json');output.write_text(json.dumps(build(),indent=2,ensure_ascii=True)+'\n');print(output)
