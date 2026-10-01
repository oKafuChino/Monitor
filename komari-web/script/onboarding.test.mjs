import test from 'node:test';
import assert from 'node:assert/strict';
import { parseGuideState, selectAdminGuide } from '../src/components/onboarding/guideState.ts';
test('unknown guides are ignored and state is normalized', () => {
 assert.deepEqual(parseGuideState({seen:['install','bogus'],workbenchOpened:true}), {seen:['install'],workbenchOpened:true});
 assert.deepEqual(parseGuideState(null), {seen:[],workbenchOpened:false});
});
test('new installations and existing nodes get appropriate guidance', () => {
 assert.equal(selectAdminGuide(0,parseGuideState(null)), 'install');
 assert.equal(selectAdminGuide(1,parseGuideState(null)), 'workbench');
 assert.equal(selectAdminGuide(1,parseGuideState({workbenchOpened:true})), 'notifications');
});

test('retired guides cannot block onboarding', () => {
 const state = parseGuideState({seen:['markets','terminal','install','workbench','notifications'],workbenchOpened:true});
 assert.deepEqual(state.seen,['install','workbench','notifications']);
 assert.equal(selectAdminGuide(2,state),null);
});
