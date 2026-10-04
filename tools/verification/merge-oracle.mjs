// SPDX-License-Identifier: MIT
// Live pinned-JS oracle; never linked into the Rust subject.
import readline from 'node:readline';
import {pathToFileURL} from 'node:url';
import {resolve} from 'node:path';
const {mergeStructuredAppendParts}=await import(pathToFileURL(resolve(process.argv[2],'src/structured-append.js')).href);
for await(const line of readline.createInterface({input:process.stdin})){
 const r=JSON.parse(line);
 try{
  const p=r.parts.map(p=>({index:p.index,total:p.total,parity:p.parity,data:p.bytes?Uint8Array.from(p.bytes):p.text}));
  const result=mergeStructuredAppendParts(p);
  console.log(JSON.stringify({...result,data:result.data instanceof Uint8Array?Array.from(result.data):result.data}));
 }catch(error){console.log(JSON.stringify({error:error.name,code:error.code}));}
}
