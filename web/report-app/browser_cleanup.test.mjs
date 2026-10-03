import test from 'node:test';
import assert from 'node:assert/strict';
import {cleanupBrowserFixture,PROFILE_REMOVAL} from './browser_cleanup.mjs';
const deferred=()=>{let resolve;const promise=new Promise(r=>{resolve=r;});return {promise,resolve};};
function harness(){const events=[],close=deferred();return {events,close,options:{directory:'/tmp/owned-profile',closed:close.promise,shutdownMs:5,browser:{kill:signal=>{events.push(['signal',signal]);close.resolve();return true;}},requestClose:()=>{events.push(['protocol','Browser.close']);close.resolve();},closeTransport:()=>events.push(['transport']),closeServer:async()=>events.push(['server']),removeProfile:async(path,options)=>events.push(['remove',path,options])}};}

test('graceful close joins owned process before bounded profile removal',async()=>{const h=harness();await cleanupBrowserFixture(h.options);assert.deepEqual(h.events.map(e=>e[0]),['protocol','transport','server','remove']);assert.equal(h.events.at(-1)[1],'/tmp/owned-profile');assert.deepEqual(h.events.at(-1)[2],{recursive:true,force:true,maxRetries:10,retryDelay:100});assert.equal(PROFILE_REMOVAL.maxRetries*(PROFILE_REMOVAL.maxRetries+1)/2*PROFILE_REMOVAL.retryDelay,5500);});

test('unavailable protocol closes only the owned child with SIGTERM',async()=>{const h=harness();h.options.requestClose=async()=>{throw new Error('closed CDP connection');};await cleanupBrowserFixture(h.options);assert.deepEqual(h.events.map(e=>e[0]),['signal','transport','server','remove']);assert.deepEqual(h.events[0],['signal','SIGTERM']);});

test('failed process join reports failure and never deletes a live profile',async()=>{const h=harness();h.options.requestClose=()=>{};h.options.browser.kill=signal=>{h.events.push(['signal',signal]);return false;};await assert.rejects(cleanupBrowserFixture(h.options),error=>error instanceof AggregateError&&error.errors[0].message.includes('cleanup deadline'));assert.deepEqual(h.events.map(e=>e[0]),['signal','transport','server']);});

test('persistent profile ENOTEMPTY remains a visible failure after configured retries',async()=>{const h=harness(),error=Object.assign(new Error('profile remained nonempty'),{code:'ENOTEMPTY'});h.options.removeProfile=async(path,options)=>{assert.equal(options.maxRetries,10);assert.equal(options.retryDelay,100);throw error;};await assert.rejects(cleanupBrowserFixture(h.options),failure=>failure instanceof AggregateError&&failure.errors.includes(error));});

test('transport cleanup failure does not suppress server or profile cleanup',async()=>{const h=harness(),error=new Error('transport cleanup');h.options.closeTransport=()=>{throw error;};await assert.rejects(cleanupBrowserFixture(h.options),failure=>failure.errors.includes(error));assert.deepEqual(h.events.map(e=>e[0]),['protocol','server','remove']);});
