const assert = require('node:assert/strict');
const fs = require('node:fs');
globalThis.crypto = require('node:crypto').webcrypto;
globalThis.require = require;
globalThis.fs = fs;
require(process.argv[2]);
const go = new Go();
let exitCode, settled = false, lateAfterExit = false, lateGoCalls = 0, finish;
const unhandled = [];
// Keep Node alive when a runtime's go.run Promise never resolves; exit status
// alone must not turn an unexecuted completion assertion into a passing check.
const watchdog = setTimeout(() => {
    console.error('WASM exit/late-settlement watchdog expired');
    process.exit(1);
}, 5000);
go.exit = code => {exitCode = code};
const resume = go._resume;
go._resume = function (...args) {
    if (this.exited) lateGoCalls++;
    return resume.apply(this, args);
};
process.on('unhandledRejection', error => unhandled.push(String(error)));
function pending(value) {
    globalThis.readStarted = true;
    return new Promise((resolve, reject) => {
        finish = () => {
            lateAfterExit = go.exited;
            settled = true;
            if (process.argv[5] === 'reject') reject(new Error('late fixture rejection'));
            else resolve(value);
        };
        globalThis.onReadStarted();
    });
}
globalThis.fetch = (url, options) => {
    let sent = false;
    const response = {ok:true, body:{getReader() {return {
        read() {return Promise.resolve(sent++ ? {done:true} : {done:false, value:new Uint8Array(8)})},
        releaseLock() {}
    }}}};
    options.signal.addEventListener('abort', () => setTimeout(() => finish(), 20), {once:true});
    return process.argv[4] === 'fetch' ? pending(response) : Promise.resolve(response);
};
if (process.argv[4] === 'digest') crypto.subtle.digest = () => pending(new ArrayBuffer(32));
WebAssembly.instantiate(fs.readFileSync(process.argv[3]), go.importObject)
    .then(({instance}) => go.run(instance))
    .then(async code => {
        await new Promise(resolve => setTimeout(resolve, 80));
        assert.equal(exitCode ?? code ?? go.exitCode, 0);
        assert.equal(globalThis.readFinished, true);
        assert.equal(settled, true);
        assert.equal(lateAfterExit, true);
        assert.equal(lateGoCalls, 0);
        assert.deepEqual(unhandled, []);
        console.log('late settlement after WASM exit: PASS');
        clearTimeout(watchdog);
    }).catch(error => {clearTimeout(watchdog);console.error(error);process.exitCode = 1});
