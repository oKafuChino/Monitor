// Updated browser regression against the isolated fixture (never a production API).
// Start: node script/ui-fixture.mjs; run with Playwright installed in the test environment.
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:4174';
const browser=await chromium.launch({headless:true,...(process.env.BROWSER_PATH?{executablePath:process.env.BROWSER_PATH}:{})});
const errors=[];
try{
 const context=await browser.newContext({locale:'zh-CN',viewport:{width:1440,height:960},serviceWorkers:'block'});
 const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message));
 await page.goto(base+'/admin/files');await page.getByRole('heading',{name:'文件管理',exact:true}).waitFor();
 assert.equal(await page.locator('a[href="/terminal"]').count(),0);
 assert.equal(await page.locator('a[href*="/admin/plugins"],a[href*="/admin/market"]').count(),0);
 await page.goto(base+'/terminal?uuid=fixture-linux&request_id=retired');
 await page.waitForURL('**/admin/files?uuid=fixture-linux');
 await page.getByText('sample.txt',{exact:true}).dblclick();
 await page.getByRole('dialog',{name:'/sample.txt',exact:true}).waitFor();
 await page.getByRole('button',{name:'视图',exact:true}).click();
 assert.doesNotMatch(await page.getByRole('menu').innerText(),/终端|Terminal/);
 await page.keyboard.press('Escape');
 await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click();
 await page.goto(base+'/admin/settings/appearance');
 await page.getByRole('heading',{name:'界面设置',exact:true}).waitFor();
 await page.getByRole('switch').first().setChecked(false);
 const save=page.getByRole('button',{name:'保存',exact:true});
 if(await save.isEnabled()){await Promise.all([page.waitForResponse(response=>response.url().endsWith('/api/admin/ui/settings')&&response.request().method()==='PATCH'),save.click()]);}
 await page.goto(base+'/admin/notification/channels');
 await page.getByText('当前通知渠道未注册。',{exact:true}).waitFor();
 await page.setViewportSize({width:390,height:844});
 await page.goto(base+'/admin/files?uuid=fixture-windows');
 await page.getByRole('heading',{name:'文件管理',exact:true}).waitFor();
 assert.deepEqual(errors,[]);
 console.log('PASS file route, legacy redirect, editor menu, appearance, unavailable channel and mobile smoke');
}finally{await browser.close();}
