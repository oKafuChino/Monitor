// Local, in-memory UI fixture. No production nodes or filesystem writes.
// Run after npm run build: node script/ui-fixture.mjs
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import crypto from 'node:crypto';
const root=fileURLToPath(new URL('../dist/',import.meta.url));
const manifest=JSON.parse(fs.readFileSync(new URL('../komari-theme.json',import.meta.url),'utf8'));
const settings=Object.fromEntries(manifest.configuration.data.filter(f=>f.key).map(f=>[f.key,f.default]));
settings._komari_onboarding_v1={seen:['install','workbench','notifications'],workbenchOpened:true};
settings._komari_dashboard_v1=[];
const clients=[{uuid:'fixture-linux',name:'Linux fixture',os:'Ubuntu',cpu_cores:2,mem_total:4294967296,disk_total:10737418240,swap_total:0,tags:''},{uuid:'fixture-windows',name:'Windows fixture',os:'Windows',mem_total:8589934592,disk_total:21474836480}];
const file=(name,is_dir=false)=>({name,path:'/'+name,is_dir,is_symlink:false,size:is_dir?0:29,mode:'-rw-r--r--',mode_octal:'0644',owner:'fixture',group:'fixture',modified_at:'2026-10-01T00:00:00Z'});
const files=[file('sample.txt'),file('notes.md'),file('folder',true)];
const contents=new Map([['/sample.txt','Hello from isolated fixture.\n'],['/notes.md','# Fixture notes\n']]);
const uploads=new Map();
const calls=[];
function rpc(req){
 calls.push(req.method);
 let result={};
 const p=req.params||{};
 switch(req.method){
 case 'common:getVersion': result={version:'fixture',hash:'ui-check'};break;
 case 'common:getNodes':result=Object.fromEntries(clients.map(c=>[c.uuid,c]));break;
 case 'common:getNodesLatestStatus':result=Object.fromEntries(clients.map(c=>[c.uuid,{online:true,cpu:12,ram:1073741824,disk:2147483648,net_in:512,net_out:128}]));break;
 case 'admin:listClients':result=clients;break;
 case 'admin:fileList':result=files;break;
 case 'admin:fileListRoots':result=[{...file('',true),name:p.uuid==='fixture-windows'?'C:':'/',path:p.uuid==='fixture-windows'?'C:/':'/'}];break;
 case 'admin:fileStat':result=files.find(f=>f.path===p.path)||file('sample.txt');break;
 case 'admin:fileSearch':result={matches:[{path:'/sample.txt',line:1,text:'Hello from isolated fixture.',is_dir:false}],limited:false};break;
 case 'admin:listNotificationChannels':result=[{id:'webhook',configuration:{type:'managed',name:'Webhook',data:[]}}];break;
 case 'admin:getNotificationChannelConfiguration':result={configuration:{type:'managed',data:[]},data:{}};break;
 default:if(req.method.toLowerCase().includes('ping')||req.method.includes('Tasks'))result=[];else if(req.method.includes('queryMetrics'))result={series:[]};
 }
 return {jsonrpc:'2.0',id:req.id,result};
}
const server=http.createServer(async(req,res)=>{
 const u=new URL(req.url,'http://localhost');
 const json=(data,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(data));};
 const chunks=[];for await(const chunk of req)chunks.push(chunk);const raw=Buffer.concat(chunks);let body={};try{body=JSON.parse(raw)}catch{}
 if(u.pathname==='/__fixture'){return json({settings,calls,files:Object.fromEntries(contents)});}
 if(u.pathname==='/api/me')return json({logged_in:true,uuid:'fixture-admin',username:'fixture',sso_id:'','2fa_enabled':false});
 if(u.pathname==='/api/public')return json({status:'success',data:{sitename:'Local verification',theme:'default',theme_settings:settings,oauth_enable:false}});
 if(u.pathname.replace(/\/$/,'')==='/api/admin/settings')return json({status:'success',data:{eula_accepted:true,theme:'default',notification_method:'legacy-plugin',notification_enabled:true}});
 if(u.pathname==='/api/admin/client/list')return json(clients);
 if(u.pathname==='/api/admin/ui/settings'){if(req.method==='PATCH'){Object.assign(settings,body);calls.push({patch:body});}return json({status:'success',data:settings});}
 if(u.pathname==='/api/rpc2')return json(Array.isArray(body)?body.map(rpc):rpc(body));
 if(u.pathname.endsWith('/file/download')){const text=contents.get(u.searchParams.get('path'))||'';res.writeHead(200,{'Content-Type':'application/octet-stream','Content-Length':Buffer.byteLength(text)});return res.end(text);}
 if(u.pathname.endsWith('/file/upload')){
  const op=u.searchParams.get('operation');calls.push('file.upload.'+op);
  if(op==='init'){const id=crypto.randomUUID();uploads.set(id,{path:body.path,content:''});return json({status:'success',data:{upload_id:id,chunk_size:26214400,chunk_count:1}});}
  const id=u.searchParams.get('upload_id')||body.upload_id;
  if(op==='chunk'&&uploads.has(id))uploads.get(id).content+=raw.toString('utf8');
  if(op==='merge'&&uploads.has(id)){const entry=uploads.get(id);contents.set(entry.path,entry.content);}
  return json({status:'success',data:{}});
 }
 if(u.pathname.startsWith('/api/'))return json({status:'success',data:[]});
 const candidate=path.resolve(root,'.'+decodeURIComponent(u.pathname));
 if(!candidate.startsWith(root))return json({error:'not found'},404);
 const target=fs.existsSync(candidate)&&fs.statSync(candidate).isFile()?candidate:path.join(root,'index.html');
 const types={'.js':'text/javascript','.css':'text/css','.html':'text/html','.json':'application/json','.webp':'image/webp','.png':'image/png','.svg':'image/svg+xml','.woff2':'font/woff2','.ttf':'font/ttf'};
 res.writeHead(200,{'Content-Type':types[path.extname(target)]||'application/octet-stream','Cache-Control':'no-store'});fs.createReadStream(target).pipe(res);
});
// Minimal fixture-only WebSocket framing for the app's JSON-RPC transport.
server.on('upgrade',(req,socket)=>{
 if(req.url!=='/api/rpc2'){socket.end('HTTP/1.1 404 Not Found\r\n\r\n');return;}
 const accept=crypto.createHash('sha1').update(req.headers['sec-websocket-key']+'258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
 socket.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: '+accept+'\r\n\r\n');
 let pending=Buffer.alloc(0);
 const send=(data,op=1)=>{const bytes=Buffer.from(data);const header=Buffer.alloc(bytes.length<126?2:4);header[0]=128|op;header[1]=bytes.length<126?bytes.length:126;if(bytes.length>=126)header.writeUInt16BE(bytes.length,2);socket.write(Buffer.concat([header,bytes]));};
 socket.on('error',()=>{});
 socket.on('data',data=>{pending=Buffer.concat([pending,data]);while(pending.length>=2){const op=pending[0]&15;let length=pending[1]&127;let offset=2;if(length===127){socket.destroy();return;}if(length===126){if(pending.length<4)return;length=pending.readUInt16BE(2);offset=4;}const masked=Boolean(pending[1]&128);if(pending.length<offset+(masked?4:0)+length)return;const mask=masked?pending.subarray(offset,offset+4):null;if(masked)offset+=4;const payload=Buffer.from(pending.subarray(offset,offset+length));if(mask)for(let i=0;i<payload.length;i++)payload[i]^=mask[i%4];pending=pending.subarray(offset+length);if(op===8){socket.end();return;}if(op===9){send(payload,10);continue;}if(op!==1)continue;try{const request=JSON.parse(payload.toString());send(JSON.stringify(rpc(request)));}catch{send(payload);}}});
});
server.listen(4174,'127.0.0.1',()=>console.log('Isolated UI fixture: http://127.0.0.1:4174'));
