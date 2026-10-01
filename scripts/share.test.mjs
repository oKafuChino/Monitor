import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {packShare,readShare} from './share-archive.mjs';
const root=fileURLToPath(new URL('../',import.meta.url));
test('share archive exactly matches independent build and excludes main routes',()=>{
 const archive=fs.readFileSync(path.join(root,'web/public/defaultTheme/share.zip'));
 const entries=readShare(archive);
 for(const [name,bytes] of entries){
  assert.match(name,/^(index\.html|assets\/[^/]+\.(js|css))$/);
  assert.deepEqual(bytes,fs.readFileSync(path.join(root,'komari-web/dist-share',name)));
  if(name.endsWith('.js'))assert.doesNotMatch(bytes.toString(),/admin:|public:|\/api\/rpc2|\/api\/nodes|\/api\/clients|serviceWorker|temp_key|localStorage/);
 }
 assert.match(entries.get('index.html').toString(),/no-referrer/);
 const temporary=path.join(root,'web/public/defaultTheme/share-test.zip');
 try{packShare(path.join(root,'komari-web/dist-share'),temporary);assert.deepEqual(fs.readFileSync(temporary),archive);}finally{fs.rmSync(temporary,{force:true});}
});
test('five languages provide every share key',()=>{
 const en=JSON.parse(fs.readFileSync(path.join(root,'komari-web/src/i18n/locales/en.json'))).share;
 for(const locale of ['zh_CN','zh_TW','ja_JP','id_ID']){
  const translated=JSON.parse(fs.readFileSync(path.join(root,`komari-web/src/i18n/locales/${locale}.json`))).share;
  assert.deepEqual(Object.keys(translated).sort(),Object.keys(en).sort());
  assert.deepEqual(Object.keys(translated.metric).sort(),Object.keys(en.metric).sort());
 }
});
