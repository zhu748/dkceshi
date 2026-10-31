'use strict';
function parseArgs(argv) {
    const opts = { reset: false, profile: '', timeout: 120, debug: false, selftest: false, browser: false, anchor: '' };
    for (let i = 0; i < argv.length; i++) {
        const a = argv[i];
        if (a === '--reset') opts.reset = true;
        else if (a === '--anchor') opts.anchor = String(argv[++i] || '');
        else if (a === '--profile') opts.profile = String(argv[++i] || '');
        else if (a === '--timeout') opts.timeout = Number(argv[++i]);
        else if (a === '--debug') opts.debug = true;
        else if (a === '--check-assets') opts.checkAssets = true;
        else if (a === '--capture') opts.capture = true;
        else if (a === '--selftest') opts.selftest = true;
        else if (a === '--browser') opts.browser = true;
        else if (a === '--via-chromium') opts.viaChromium = true;
        else if (a === '--dump-payload') { const f = String(argv[++i] || ''); if (f) { opts.dumpPayload = f; } }
        else if (a === '--help' || a === '-h') {
            process.stdout.write('usage: node devid-generate.cjs [--anchor <file>] [--reset] [--profile <file>] [--timeout <sec>] [--debug] [--dump-payload <file>] [--selftest] [--browser]\n');
            process.exit(0);
        } else throw new Error("Unknown option: " + a);
    }
    if (!Number.isFinite(opts.timeout) || opts.timeout <= 0 || opts.timeout > 600) throw new Error('--timeout must be between 0 and 600 seconds');
    if ((opts.selftest || opts.capture || opts.checkAssets) && (opts.browser || opts.viaChromium)) throw new Error('--selftest / --capture require the offline VM path');
    return opts;
}


module.exports = { parseArgs };
