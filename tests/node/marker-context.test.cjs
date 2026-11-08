'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { createMarkerNormalizer } = require('../../internal/js/chat-stream/marker');
const { parseToolCalls } = require('../../internal/js/helpers/stream-tool-sieve');

test('caller marker normalization preserves every literal context and nested JSON argument', () => {
  const literal = '<|Q7ZK3M|invoke>';
  const payload = JSON.stringify({ snippet: literal, open: '<![CDATA[', escaped: '"' + literal });
  const call = `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="payload">${payload}</|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`;
  const normalizedCall = call.replace(/<([/]?)\|Q7ZK3M\|/g, (tag, slash, at) => {
    const body = call.indexOf(payload);
    return at >= body && at < body + payload.length ? tag : `<${slash}|EPSE|`;
  });
  for (const example of [
    '`' + literal + '`', '``' + literal + '``', '```xml\n' + literal + '\n```', '~~~xml\n' + literal + '\n~~~',
    JSON.stringify({ example: literal }), '<!-- ' + literal + ' -->', '<![CDATA[' + literal + ']]>',
  ]) {
    const input = example + '\n' + call;
    for (let split = 0; split <= input.length; split++) {
      const n = createMarkerNormalizer('Q7ZK3M');
      const output = n.write(input.slice(0, split)) + n.write(input.slice(split)) + n.flush();
      assert.equal(output, example + '\n' + normalizedCall, `split=${split}`);
      assert.deepEqual(parseToolCalls(output, []), [{ name: 'write', input: { payload: JSON.parse(payload) } }]);
    }
  }
});
