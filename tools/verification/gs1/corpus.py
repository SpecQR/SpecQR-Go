"""Pinned deterministic 5610-case GS1 corpus; references run separately."""
import json
import random
import string

def build_cases():
    root='https://example.com/01/04912345678904'
    cases=[dict(command='catalog',category='catalog')]
    def url(s,category='ordinary'):cases.append(dict(command='url',input=s,category=category))
    def create(value,base='https://example.com',category='ordinary'):cases.append(dict(command='create',elements=[dict(ai='01',value='04912345678904'),dict(ai='10',value=value)],baseUrl=base,category=category))
    for options in [dict(unknownQuery='reject'),dict(unknownQuery='oops'),dict(primaryAi='10'),dict(primaryAi='00'),dict(normalize=True)]:
     for suffix in ['', '?x=1','?quote%22key=1','?back%5Ckey=1','?line%0Akey=1','?%00=1','?%F0%9F%98%80=1']:
      cases.append(dict(command='url',input=root+suffix,options=options,category='ordinary'))
    # Host, authority, prefix, escape and all ASCII punctuation boundaries.
    hosts=['example.com','EXAMPLE.COM.','foo..bar','foo_bar.test','-x.test','x-.test','0','0x','0x7f000001','0177.1','127.1','4294967295','4294967296','1.2.3.256','1.2.3.4.5','09','08','1.2.0x100','1.2.3.','0xffffffff','0X7F.1','1.2.3.0004','1.2.3.','a%2eb','a'*64+'.test','a'*256+'.test','%65xample.com','a%2fb','%ff','%ED%A0%80','[::]','[::1]','[0:0:0:1:0:0:0:1]','[1::]','[::ffff:192.0.2.128]','[::ffff:192.00.2.128]','[::%25eth0]','[:::]','[1:2]','[1:2:3:4:5:6:7:8]','[1:2:3:4:5:6:7:8:9]']
    for h in hosts:
     for scheme in ['https://','https:','http:///','HTTPS:\\\\']:
      url(scheme+h+'/01/04912345678904/10/LOT?17=271031&x=a+b')
    for port in ['','0','000','80','443','65535','65536','999999999999999999999999999','+80','-1','abc',':80','000443']:
     for scheme in ['http','https']:url(scheme+'://example.com:'+port+'/01/04912345678904')
    for c in map(chr,range(128)):
     url('https://a'+c+'b.test/01/04912345678904')
     url('https://u'+c+'ser:p'+c+'ass@example.com/01/04912345678904')
     url('https://example.com/a'+c+'b/01/04912345678904')
     url(root+'/10/A'+c+'B')
     url(root+'?x=A'+c+'B')
     create('A'+c+'B')
    for host in ['例え.テスト','faß.de','βόλος.gr','ς.gr','σ.gr','☃.net','😀.test','a\u200cb.test','a\u200db.test','a\u3002b','é.test','ｅｘａｍｐｌｅ.com','\u00ad.test','ẞ.de','𐐀.test','\u0600.test']:
     url('https://'+host+'/01/04912345678904','idna')
    for ace in ['xn--a','xn--','xn--abc','xn--abc-','xn--a-ecp.ru','xn--fa-hia.de','xn--e28h.test','xn--0.pt','xn--bcher-kva.de','xn--a.test','xn--a_.test','xn--%61.test','xn--ls8h.test','xn--3xa.gr']:
     url('https://'+ace+'/01/04912345678904','idna')
    for s in ['.','..','%2e','%2E.','.%2E','%2e%2e']:
     url(root+'/10/'+s,'dot');url(root+'/10/'+s+'/21/S','dot');url(root+'?10='+s,'dot');create(s if '%' not in s else '.',category='dot')
    for token in ['', '%', '%0','%00','%FF','%ED%A0%80','%E2%82','%E0%80%80','%F0%9F%98%80','%C0%80','%F4%90%80%80','%C2%A0','a+b','a%2Bb','%26x%3D1','%252e','%','x=y=z']:
     for loc in ['/10/','?10=','?x=','?%31%30=','?x=one&&y=two&x=']:
      url(root+loc+token)
    for stem in ['/','//','///','/a//b','/a/./b','/a/../b','/a/%2e/b','/a/%2e%2e/b','/x/%2F/y','/x/%zz/y']:
     url('https://example.com'+stem+'/01/04912345678904');create('LOT','https://example.com'+stem)
    for raw in ['http://','https://?x=1','//example.com/01/04912345678904','/01/04912345678904','ftp://example.com/01/04912345678904','mailto:a@b',' '+root+' ','\0\t'+root+'\r\n','\u00a0'+root,root+'#',root+'#x',root+'?',root+'?&',root+'?17=271031&17=271031',root+'/10/A?10=B',root+'/17/271031']:
     url(raw)
    # Deterministic malformed/valid percent-encoding property samples.
    r=random.Random(701)
    chars=string.ascii_letters+string.digits+' -_~/+%?#&=.:;@[]{}\\\x00\x1f\x7f'
    for i in range(2500):
     value=''.join(r.choice(chars) for _ in range(r.randrange(1,28)))
     url(root+r.choice(['/10/','?10=','?x=','/10/A?x='])+value)
     if i%10==0:create(value)
    # Exercise every UTF-8 octet, continuation-boundary class, and seeded byte string.
    for byte in range(256):
     url(root+'?x=%'+format(byte,'02X'))
    for lead in range(0xc0,0xf8):
     for trail in [0,0x7f,0x80,0x8f,0x90,0x9f,0xa0,0xbf,0xc0,0xff]:
      url(root+'?x=%'+format(lead,'02X')+'%'+format(trail,'02X'))
    for sample in range(512):
     url(root+'?x='+''.join('%'+format(r.randrange(256),'02X') for _ in range(r.randrange(1,8))))
    for token in ['%ＦＦ','%４１','%١١','%E2%28%A1','%C0%AF','%EF%BB%BF','%F4%90%80%80']:
     url(root+'?x='+token)
    url(root+'?x=\ud800')
    # Concrete catalog valid and six invalid element cases per AI.
    fixed={'00':18,'01':14,'02':14,**{x:6 for x in ['11','12','13','15','16','17']},'20':2,**{str(x):13 for x in range(410,416)},**{x:3 for x in ['422','424','425','426']},**{str(x):6 for x in list(range(3100,3106))+list(range(3200,3206))}}
    variable={'10':20,'21':20,'22':20,'30':8,'37':8,'240':30,'241':30,'400':30,'420':20,**{str(x):90 for x in range(91,100)}}
    for ai,limit in (fixed|variable).items():
     numeric=ai in fixed or ai in ['30','37'];valid='0'*limit if ai in fixed else '1' if numeric else 'A'
     for val in [valid,'','0'*(limit+1),'é','A\x1dB','A(B)','A' if numeric else 'A'*limit]:
      cases.append(dict(command='elements',elements=[dict(ai=ai,value=val)],category='catalog'))
    body='\n'.join(json.dumps({k:v for k,v in c.items() if k!='category'},ensure_ascii=True) for c in cases)+'\n'
    return cases, body
