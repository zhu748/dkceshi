'use strict';

const { VisibleCleaner } = require('../visible-cleaner');
const { consumePreview } = require('./quoted_preview');

function cleanVisibleEvents(state, events, final = false) {
  if (!state) return events;
  if (state.cleanVisible) state.visibleCleaner ||= new VisibleCleaner(state.stripReferences === true);
  for (const event of events) {
    if (event.preview || !event.text) continue;
    if (state.cleanVisible && !event.deferred) event.text = state.visibleCleaner.write(event.text);
    event.text = consumePreview(state, event.text);
  }
  if (final && state.cleanVisible) {
    const text = consumePreview(state, state.visibleCleaner.flush());
    if (text) events.push({ type: 'text', text });
  }
  return events;
}

module.exports = { cleanVisibleEvents };
