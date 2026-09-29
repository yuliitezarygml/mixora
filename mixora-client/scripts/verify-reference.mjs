import {readFile,readdir} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {resolve} from 'node:path';
const root=resolve(import.meta.dirname,'..');
const manifest=JSON.parse(await readFile(resolve(root,'docs/reference-styles.json'),'utf8'));
let css='';
for(const file of manifest.files) {
 const data=await readFile(resolve(root,'public','.'+file.path));
 if(createHash('sha256').update(data).digest('hex')!==file.sha256)throw new Error(`Original CSS modified: ${file.path}`);
 css+=data.toString();
}
const selectors=new Set([...css.matchAll(/\.([a-zA-Z][\w-]*__[\w-]+)/g)].map(m=>m[1]));
async function check(dir) {
 for (const item of await readdir(dir,{withFileTypes:true})) {
  const path=resolve(dir,item.name);
  if(item.isDirectory())await check(path);
  else if(item.name.endsWith('.jsx')){
   const source=await readFile(path,'utf8');
   for(const match of source.matchAll(/\b([A-Z][A-Za-z]+_[\w]+__[\w]+)/g))if(!selectors.has(match[1]))throw new Error(`Missing original selector ${match[1]} in ${path}`);
  }
 }
}
await check(resolve(root,'src'));
console.log(`${manifest.files.length} original stylesheets verified by SHA-256; all migrated component classes exist.`);
