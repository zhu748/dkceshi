'use strict';

// A terminal candidate is immutable until regeneration. Cache the promise too:
// repeated finalization must not issue another paid repair, even after failure.
function createFinalValidation() {
  let pending;
  return {
    run(action) { return pending || (pending = Promise.resolve().then(action)); },
    reset() { pending = undefined; },
  };
}

module.exports = { createFinalValidation };
