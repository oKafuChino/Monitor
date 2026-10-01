// Fixture browser checks exercise the real share bundle, not backend security.
// PLAYWRIGHT_MODULE may point to a bundled Playwright installation.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import {pathToFileURL,fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE ? pathToFileURL(process.env.PLAYWRIGHT_MODULE).href : 'playwright');

test('share bundle renders desktop/mobile and languages, clears offline/revoked data, and requests only share routes',async()=>{
 const requests=[],errors=[];let revoked=false;
 const node={name:'Fixture Node',region:'Hong Kong',cpu_name:'AMD EPYC',cpu_cores:8,arch:'amd64',virtualization:'KVM',os:'Ubuntu 24.04',kernel_version:'6.8',gpu_name:'NVIDIA L4',mem_total:8*1024**3,swap_total:1024**3,disk_total:100*1024**3};
 const latest={cpu:{usage:12.5},ram:{used:1024**3,total:8*1024**3},swap:{used:0,total:1024**3},disk:{used:20*1024**3,total:100*1024**3},disk_io:{status:'ok',read_bytes_per_sec:0,write_bytes_per_sec:2048,sample_interval_ms:2000},network:{up:2048,down:4096,totalUp:1024**3,totalDown:2*1024**3},connections:{tcp:12,udp:2},process:42,uptime:86401,load:{load1:1,load5:1,load15:1},updated_at:new Date().toISOString()};
 const server=http.createServer(async(req,res)=>{
  const url=new URL(req.url,'http://localhost');requests.push(url.pathname);
  res.setHeader('Cache-Control','no-store');res.setHeader('Referrer-Policy','no-referrer');
  res.setHeader('Content-Security-Policy',"default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; form-action 'none'");
  if(url.pathname==='/api/share/rpc'){
   if(revoked){res.writeHead(404).end();return;}
   let body='';for await(const c of req)body+=c;const rpc=JSON.parse(body);
   const expiry=new Date(Date.now()+3600000).toISOString();let result;
   if(rpc.method==='share:getNode')result={node,share_expires_at:expiry,session_expires_at:expiry,definitions:[],ping_tasks:[]};
   else if(rpc.method==='share:getLatestStatus')result={online:true,latest,recent:[],share_expires_at:expiry,session_expires_at:expiry};
   else if(rpc.method==='share:queryMetrics')result={series:rpc.params.metric_keys.map(key=>({metric_key:key,id:'node',name:key,unit:key==='cpu.usage'?'%':'',points:Array.from({length:12},(_,i)=>({time:new Date(Date.now()-(12-i)*60000).toISOString(),value:key==='disk.io.read.rate'?0:i*2}))}))};
   else{res.writeHead(400).end();return;}
   res.setHeader('Content-Type','application/json');res.end(JSON.stringify({jsonrpc:'2.0',id:rpc.id,result}));return;
  }
  const name=url.pathname==='/node'?'index.html':url.pathname.slice(1);
  if(name!=='index.html'&&!/^assets\/[^/]+\.(js|css)$/.test(name)){res.writeHead(404).end();return;}
  res.setHeader('Content-Type',name.endsWith('.html')?'text/html':name.endsWith('.js')?'text/javascript':'text/css');res.end(fs.readFileSync(path.join(root,'komari-web/dist-share',name)));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 let browser;
 try{
  browser=await chromium.launch({headless:true,...(!fs.existsSync(chromium.executablePath())?{channel:'msedge'}:{})});
  const context=await browser.newContext({viewport:{width:1200,height:900},locale:'en-US'});const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/node`);
  await page.getByRole('heading',{name:'Fixture Node'}).waitFor();
  assert.match(await page.locator('.stats').innerText(),/Read 0 B\/s/);
  fs.mkdirSync(path.join(root,'dist/share-qa'),{recursive:true});
  await page.screenshot({path:path.join(root,'dist/share-qa/desktop.png')});
  for(const [locale,label] of [['zh-CN','临时分享节点'],['zh-TW','臨時分享節點'],['ja','共有ノード'],['id','Node yang dibagikan'],['en','Shared node']]){await page.locator('header select').selectOption(locale);await page.getByText(label,{exact:true}).waitFor();}
  await page.setViewportSize({width:390,height:844});await page.emulateMedia({colorScheme:'dark'});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),true);
  await page.screenshot({path:path.join(root,'dist/share-qa/mobile-dark.png')});
  await context.setOffline(true);await page.getByRole('heading',{name:'Unable to retrieve data'}).waitFor();assert.equal(await page.getByRole('heading',{name:'Fixture Node'}).count(),0);assert.equal(await page.locator('.charts').count(),0);
  await context.setOffline(false);await page.getByRole('heading',{name:'Fixture Node'}).waitFor();
  revoked=true;await page.getByRole('heading',{name:'Share unavailable or expired'}).waitFor({timeout:8000});assert.equal(await page.locator('.charts').count(),0);
  assert.deepEqual(errors,[]);
  assert.ok(requests.every(p=>p==='/node'||p==='/api/share/rpc'||/^\/assets\//.test(p)),JSON.stringify(requests));
 }finally{await browser?.close();await new Promise(resolve=>server.close(resolve));}
});
