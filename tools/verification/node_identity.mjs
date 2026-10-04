// SPDX-License-Identifier: MIT
// Fingerprint the actual independent development packages loaded by the tools.
import { createRequire } from 'node:module';
import { createHash } from 'node:crypto';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
const require=createRequire(join(resolve(process.env.SPECQR_DEV_NODE_MODULES??fileURLToPath(new URL('.',import.meta.url))),'package.json'));
const expected={'nayuki-qr-code-generator':'1.8.0','jsqr':'1.4.0'},packages={};
for(const name of process.argv.slice(2)) {
 const metadata=require.resolve(name+'/package.json'),root=dirname(metadata),version=JSON.parse(readFileSync(metadata)).version;
 if(version!==expected[name])throw new Error(`Unpinned package ${name}@${version}`);
 const files={};
 function walk(directory){for(const e of readdirSync(directory,{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))){
  const path=join(directory,e.name);
  if(e.isDirectory())walk(path);else if(e.isFile())files[relative(root,path)]=createHash('sha256').update(readFileSync(path)).digest('hex');
 }}
 walk(root);packages[name]={version,entry:require.resolve(name),filesSha256:files};
}
console.log(JSON.stringify({node:process.version,nodeVersions:process.versions,executable:process.execPath,executableSha256:createHash('sha256').update(readFileSync(process.execPath)).digest('hex'),packages}));
