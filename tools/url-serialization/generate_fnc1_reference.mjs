import fs from 'node:fs';
import {createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
const base=resolve(process.argv[2]), fixture=JSON.parse(fs.readFileSync(process.argv[3]));
if(execFileSync('git',['-C',base,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!=='16efc6c0a8e397c9df3d051d20fce6c1eebdfad7')throw Error('reference commit');
execFileSync('git',['-C',base,'diff','--quiet','HEAD']);
const load=p=>import(pathToFileURL(join(base,p)).href),api=await load('src/index.js');
const {normalizeOptions}=await load('src/options.js');
const {selectPlanForInput,selectPlanForManualSegments}=await load('src/internal/planning.js');
const {normalizeManualSegments,encodeSegments}=await load('src/encoding/modes.js');
const {buildResultArtifact}=await load('src/internal/build.js');
const hex=x=>Buffer.from(x).toString('hex'), hash=x=>createHash('sha256').update(x).digest('hex');
function expected(r){try{const o=normalizeOptions({...r.options,output:'matrix',diagnostics:true});
 const s=r.segments?normalizeManualSegments(r.segments.map(s=>['numeric','alphanumeric','byte','kanji'].includes(s.mode)?{mode:s.mode,data:s.bytes??s.text}:s)):null;
 const p=s?selectPlanForManualSegments(s,o):selectPlanForInput(r.text,o), b=buildResultArtifact(p,o);
 const q=s?api.generateSegments(r.segments.map(s=>['numeric','alphanumeric','byte','kanji'].includes(s.mode)?{mode:s.mode,data:s.bytes??s.text}:s),o):api.generate(r.text,o);
 return {version:p.version,ecc:p.errorCorrectionLevel,mask:q.maskPattern??q.diagnostics.maskPattern,data:hex(encodeSegments(p.segments,p.version,p.errorCorrectionLevel)),codewords:hex(b.interleaved.codewords),matrixHash:hash(q.matrix.map(r=>r.map(Number).join('')).join(''))};
 }catch(e){return {errorCode:e.code};}}
const rows=[];
for(const v of fixture.vectors){
 const request={text:v.input,options:{...v.options}},ref={text:v.input,options:{...v.options,mode:'byte'}};
 rows.push({id:v.id,request,referenceRequest:ref,expected:v.options.mode==='alphanumeric'?{nativeForcedAlpha:true}:expected(ref),payloadHex:v.expectedPayloadUtf8Hex??Buffer.from(v.input).toString('hex')});
 if(v.options.mode!=='alphanumeric'){
  const request={text:v.input,options:{...v.options,maskPattern:3},pngScale:8,includeMatrix:true};delete request.options.version;
  const ref={...request,options:{...request.options,mode:'byte'}};
  rows.push({id:v.id+'-unbounded-version',request,referenceRequest:ref,expected:expected(ref),payloadHex:v.expectedPayloadUtf8Hex??Buffer.from(v.input).toString('hex')});
 }
}
for(const [i,v] of fixture.manualVectors.entries()){
 const segments=[...v.controls,...v.data.map(s=>({mode:s.mode,...(Array.isArray(s.data)?{bytes:s.data}:{text:s.data})}))];
 const request={segments,options:{errorCorrectionLevel:'L',maskPattern:3},pngScale:8,includeMatrix:true};
 rows.push({id:'manual-'+i,request,referenceRequest:request,expected:expected(request),payloadHex:v.expectedPayloadUtf8Hex});
}
process.stdout.write(JSON.stringify({referenceCommit:'16efc6c0a8e397c9df3d051d20fce6c1eebdfad7',originalVectors:102,manualVectors:4,rows},null,2)+'\n');
