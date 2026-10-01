import test from 'node:test';
import assert from 'node:assert/strict';
import { readUISettings, saveUISettings } from '../src/utils/uiSettings.ts';
Object.defineProperty(globalThis,'navigator',{value:{},configurable:true});
test('object patches send only changed keys and preserve explicit empty values',async()=>{
 const calls=[];globalThis.fetch=async(url,options)=>{calls.push({url,...options});return {ok:true};};
 await saveUISettings({showIpTagsInCard:false,mainContentWidth:0,backgroundImageUrlDesktop:'',_komari_dashboard_v1:[]});
 assert.equal(calls.length,1);assert.equal(calls[0].url,'/api/admin/ui/settings');assert.equal(calls[0].method,'PATCH');
 assert.deepEqual(JSON.parse(calls[0].body),{showIpTagsInCard:false,mainContentWidth:0,backgroundImageUrlDesktop:'',_komari_dashboard_v1:[]});
});
test('functional guide update reads current settings but writes only its own key',async()=>{
 const writes=[];globalThis.fetch=async(_url,options)=>options.method==='PATCH'?(writes.push(JSON.parse(options.body)),{ok:true}):{ok:true,json:async()=>({data:{customFooterHtml:'preserve',_komari_onboarding_v1:{seen:['install']}}})};
 await saveUISettings(current=>({_komari_onboarding_v1:{seen:[...current._komari_onboarding_v1.seen,'notifications']}}));
 assert.deepEqual(writes,[{_komari_onboarding_v1:{seen:['install','notifications']}}]);
});
test('failed writes report errors without poisoning later saves',async()=>{
 globalThis.fetch=async()=>({ok:false,status:500,json:async()=>({message:'database failure'})});
 await assert.rejects(saveUISettings({mainContentWidth:80}),/database failure/);
 globalThis.fetch=async()=>({ok:true});await saveUISettings({mainContentWidth:75});
});
test('malformed settings responses do not become silent empty settings',async()=>{
 globalThis.fetch=async()=>({ok:true,json:async()=>({data:[]})});await assert.rejects(readUISettings(),/Invalid UI settings/);
});
