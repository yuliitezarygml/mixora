import http from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../app');
const types={'.js':'application/javascript','.css':'text/css','.html':'text/html; charset=utf-8','.txt':'text/x-component','.json':'application/json','.svg':'image/svg+xml','.png':'image/png','.jpg':'image/jpeg','.woff2':'font/woff2','.woff':'font/woff','.wasm':'application/wasm','.mp4':'video/mp4','.webm':'video/webm'};
http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://localhost');
 if(url.pathname.startsWith('/api/')||url.pathname.startsWith('/compat/')||url.pathname.startsWith('/health/')){
  const proxy=http.request({hostname:'127.0.0.1',port:8080,path:req.url,method:req.method,headers:{...req.headers,host:'localhost:8080'}},response=>{res.writeHead(response.statusCode,response.headers);response.pipe(res)});
  proxy.on('error',()=>{res.writeHead(502,{'Content-Type':'application/json'});res.end('{"error":"Backend unavailable"}')});req.pipe(proxy);return;
 }
 try{
  let name=decodeURIComponent(url.pathname);if(name==='/')name='/index.html';
  if(!path.extname(name))name+=(req.headers.rsc==='1'?'.txt':'.html');
  const file=path.resolve(root,'.'+name);if(!file.startsWith(root+path.sep))throw Error('outside root');
  const data=await fs.readFile(file);res.writeHead(200,{'Content-Type':types[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store'});res.end(data);
 }catch{res.writeHead(404);res.end('Not found')}
}).listen(5173,'127.0.0.1',()=>console.log('Mixora client: http://localhost:5173'));
