import { readFile, readdir, mkdir, copyFile, cp, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { resolve } from 'node:path';
const root = resolve(import.meta.dirname, '..');
const source = resolve(root, '../mixora-music-client/app');
const output = resolve(root, 'public');
const html = await readFile(resolve(source, 'index.html'), 'utf8');
const ordered = [...html.matchAll(/href="(\/_next\/static\/css\/[^" ]+\.css)"/g)].map(m => m[1]);
const remaining = (await readdir(resolve(source, '_next/static/css'))).filter(x => x.endsWith('.css')).sort().map(x => `/_next/static/css/${x}`);
const paths = [...new Set([...ordered, ...remaining, '/styles/fonts.css'])];
const files = [];
for (const path of paths) {
  await mkdir(resolve(output, '.'+path, '..'), {recursive:true});
  const data = await readFile(resolve(source, '.'+path));
  await copyFile(resolve(source, '.'+path), resolve(output, '.'+path));
  files.push({path, sha256:createHash('sha256').update(data).digest('hex'), bytes:data.length});
}
for (const path of ['fonts','_next/static/media']) await cp(resolve(source,path), resolve(output,path), {recursive:true});
const indexPath = resolve(root,'index.html');
const index = await readFile(indexPath,'utf8');
const links = '<!-- reference-styles:start -->\n'+paths.map(p => `    <link rel="stylesheet" href="${p}" />`).join('\n')+'\n    <!-- reference-styles:end -->';
await writeFile(indexPath,index.includes('reference-styles:start') ? index.replace(/<!-- reference-styles:start -->[\s\S]*?<!-- reference-styles:end -->/,links) : index.replace('</head>',links+'\n  </head>'));
await writeFile(resolve(root,'docs/reference-styles.json'),JSON.stringify({source:'../mixora-music-client/app',files},null,2)+'\n');
console.log(`Copied ${files.length} original stylesheets byte-for-byte, plus fonts and referenced static media.`);
