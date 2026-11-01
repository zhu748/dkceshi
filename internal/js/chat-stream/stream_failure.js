'use strict';

const { openAIErrorType } = require('./error_shape');

const MODERATION_BLOCKED_THINKING_PHRASE = '你好，这个问题我暂时无法回答，让我们换个话题再聊聊吧';

function upstreamEmptyOutputDetail(contentFilter, _text, thinking) {
  if (contentFilter) {
    return {
      status: 400,
      message: 'Upstream content filtered the response and returned no output.',
      code: 'content_filter',
    };
  }
  if (thinking !== '') {
    if (thinking.includes(MODERATION_BLOCKED_THINKING_PHRASE)) {
      return {
        status: 400,
        message: '输入内容被审核拦截',
        code: 'input_content_blocked',
      };
    }
    return {
      status: 429,
      message: 'Upstream account hit a rate limit and returned reasoning without visible output.',
      code: 'upstream_empty_output',
    };
  }
  return {
    status: 503,
    message: 'Upstream service is unavailable and returned no output.',
    code: 'upstream_unavailable',
  };
}

function sendFailedChunk(res, status, message, code) {
  if (res.writableEnded || res.destroyed) {
    return;
  }
  res.write(`data: ${JSON.stringify({
    status_code: status,
    error: {
      message,
      type: openAIErrorType(status),
      code,
      param: null,
    },
  })}\n\n`);
  if (!res.writableEnded && !res.destroyed) {
    res.write('data: [DONE]\n\n');
  }
  if (typeof res.flush === 'function') {
    res.flush();
  }
}

async function finishStreamFailure(res, releaseLease, detail) {
  sendFailedChunk(res, detail.status, detail.message, detail.code);
  await releaseLease();
  if (!res.writableEnded && !res.destroyed) res.end();
}

module.exports = { sendFailedChunk, upstreamEmptyOutputDetail, finishStreamFailure };
