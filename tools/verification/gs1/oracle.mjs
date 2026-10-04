import readline from 'node:readline';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
const gs1 = await import(pathToFileURL(path.resolve(process.argv[2], 'src/gs1.js')).href);
const e=values=>values.map(x=>[x.ai,x.value]);
const fields=['code','reason','ai','value','key','offset','elementIndex','expected','count'];
const diagnostic=value=>Object.fromEntries(fields.map(field=>[field,value[field]??null]));
for await (const line of readline.createInterface({input:process.stdin})) {
 const r=JSON.parse(line),out={};
 try{
 if(r.command==='catalog'){ out.catalog={ok:true,value:gs1.getSupportedGs1Ais()}; } else if(r.command==='url'){
  try{const p=gs1.parseGs1DigitalLink(r.input,r.options);out.parse={ok:true,elements:e(p.elements),path:e(p.pathElements),query:e(p.queryElements),unknown:p.unknownQuery.map(x=>[x.key,x.value])};}catch(e){out.parse={ok:false,code:e.code};}
  try{out.normalize={ok:true,value:gs1.normalizeGs1DigitalLink(r.input,r.options)};}catch(e){out.normalize={ok:false,code:e.code};}
  const v=gs1.validateGs1DigitalLink(r.input,r.options);out.validate={ok:v.ok,errors:(v.errors??[]).map(diagnostic),warnings:(v.warnings??[]).map(diagnostic)};
 }else if(r.command==='create'){
  try{out.create={ok:true,value:gs1.createGs1DigitalLink(r.elements,{baseUrl:r.baseUrl})};}catch(e){out.create={ok:false,code:e.code};}
 }else if(r.command==='elements'){
  const v=gs1.validateGs1Elements(r.elements);out.validate={ok:v.ok,errors:(v.errors??[]).map(diagnostic)};
  try{out.string={ok:true,value:gs1.createGs1ElementString(r.elements)};}catch(e){out.string={ok:false,code:e.code};}
 }
 }catch(e){out.uncaught=String(e);}
 console.log(JSON.stringify(out));
}
