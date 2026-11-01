'use strict';

const { VisibleCleaner } = require('../visible-cleaner');

function cleanVisibleEvents(state, events, final = false) {
  if (!state?.cleanVisible) return events;
  state.visibleCleaner ||= new VisibleCleaner();
  for (const event of events) {
    if (!event.deferred && event.text) event.text = state.visibleCleaner.write(event.text);
  }
  if (final) {
    const text = state.visibleCleaner.flush();
    if (text) events.push({ type: 'text', text });
  }
  return events;
}

module.exports = { cleanVisibleEvents };
