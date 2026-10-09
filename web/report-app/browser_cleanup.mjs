// Test-harness lifecycle only. Never discovers or signals unrelated processes.
export const PROFILE_REMOVAL = Object.freeze({recursive:true,force:true,maxRetries:10,retryDelay:100});

async function closedWithin(closed, timeoutMs) {
  let timer;
  try {
    return await Promise.race([
      closed.then(()=>true),
      new Promise(resolve=>{timer=setTimeout(()=>resolve(false),timeoutMs);})
    ]);
  } finally { clearTimeout(timer); }
}

export async function cleanupBrowserFixture({browser,closed,requestClose,closeTransport,closeServer,removeProfile,directory,shutdownMs=5000}) {
  const failures=[];
  // Browser.close may close its connection before acknowledging. Its only
  // fallback is a bounded signal to this fixture's own ChildProcess.
  void Promise.resolve().then(requestClose).catch(()=>{});
  let stopped=false;
  try {
    stopped=await closedWithin(closed,shutdownMs);
    if(!stopped){
      browser.kill('SIGTERM');
      stopped=await closedWithin(closed,shutdownMs);
    }
    if(!stopped)throw new Error('Owned Chromium process did not close within the cleanup deadline');
  } catch(error) { failures.push(error); }
  try { closeTransport(); } catch(error) { failures.push(error); }
  try { await closeServer(); } catch(error) { failures.push(error); }
  // A close event also joins the process's stdio. Residual profile writes from
  // Chromium children can still race removal; Node retries only bounded standard
  // transient errors (including ENOTEMPTY). Persistent errors remain failures.
  if(stopped){try{await removeProfile(directory,PROFILE_REMOVAL);}catch(error){failures.push(error);}}
  if(failures.length)throw new AggregateError(failures,'Browser fixture cleanup failed');
}
