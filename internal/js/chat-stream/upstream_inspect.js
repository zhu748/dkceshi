'use strict';

const { asString } = require('./http_internal');

// 检测 DeepSeek 是否返回了封号/停用 JSON 响应（HTTP 200 但 body 是 JSON 而非 SSE）。
// 与 Go 侧 detectMutedCompletion / isUserBannedResponse 对齐：先 peek 第一个字节，只有 JSON 才读取全部 body。
// 返回 { mutedUntil, banned, response }：
//   - mutedUntil > 0 时表示检测到禁言，response 为 null；
//   - banned 为 true 时表示检测到 USER_IS_BANNED，response 为 null；
//   - 均未检测到时，response 为可能经过恢复的 Response（SSE body 未被消耗）。
async function detectMutedOrBannedResponse(res) {
  if (!res || !res.ok || !res.body) {
    return { mutedUntil: 0, banned: false, response: res };
  }
  const reader = res.body.getReader();
  let firstChunk;
  try {
    const result = await reader.read();
    if (result.done || !result.value || result.value.length === 0) {
      reader.releaseLock();
      return { mutedUntil: 0, banned: false, response: res };
    }
    firstChunk = result.value;
  } catch (_err) {
    try { reader.releaseLock(); } catch (_e) {}
    return { mutedUntil: 0, banned: false, response: res };
  }
  if (firstChunk[0] !== 0x7b) { // '{'
    let cancelled = false;
    let released = false;
    const releaseSource = () => {
      if (!released) { released = true; reader.releaseLock(); }
    };
    const newBody = new ReadableStream({
      start(controller) { controller.enqueue(firstChunk); },
      async pull(controller) {
        try {
          const { done, value } = await reader.read();
          if (cancelled) return;
          if (done) { controller.close(); releaseSource(); }
          else controller.enqueue(value);
        } catch (err) {
          if (!cancelled) { controller.error(err); releaseSource(); }
        }
      },
      async cancel(reason) {
        cancelled = true;
        try { if (!released) await reader.cancel(reason); }
        finally { releaseSource(); }
      },
    });
    return { mutedUntil: 0, banned: false, response: new Response(newBody, res) };
  }
  // JSON body: read it in full (bounded), then either report the mute/ban
  // verdict or hand back a replayable response. Returning response:null with
  // the original res — whose body reader is now locked and never released —
  // made the downstream getReader() call throw "ReadableStream is locked"
  // after headers were already flushed, silently truncating the client stream.
  const chunks = [firstChunk];
  let total = firstChunk.length;
  try {
    while (true) {
      // The first read can itself exceed the cap. Do not wait for another
      // chunk before handing its buffered prefix back to the normal path.
      if (total > MAX_MUTED_DETECT_JSON_BYTES) {
        return { mutedUntil: 0, banned: false, response: replayJSONBodyResponse(res, chunks) };
      }
      const { done, value } = await reader.read();
      if (done) break;
      if (value) { chunks.push(value); total += value.length; }
    }
    try {
      const data = JSON.parse(Buffer.concat(chunks).toString('utf-8'));
      const mutedUntil = extractMuteUntil(data);
      const banned = isBannedJSONResponse(data);
      if (mutedUntil > 0 || banned) return { mutedUntil, banned, response: null };
    } catch (_error) { /* Truncated JSON is replayed unchanged. */ }
    return { mutedUntil: 0, banned: false, response: replayJSONBodyResponse(res, chunks) };
  } finally {
    try { await reader.cancel(); }
    catch (error) { console.warn('[vercel_stream_inspect] reader cancel failed', error.message); }
    reader.releaseLock();
  }
}

// MAX_MUTED_DETECT_JSON_BYTES matches the Go-side mute-detection read cap
// (1 MiB) in internal/deepseek/client/client_completion_inspect.go.
const MAX_MUTED_DETECT_JSON_BYTES = 1 << 20;

// replayJSONBodyResponse rebuilds a Response whose body replays the buffered
// chunks; the original res body is fully consumed by now, so the replacement
// is what downstream consumers read from.
function replayJSONBodyResponse(res, chunks) {
  const body = Buffer.concat(chunks);
  return new Response(body, res);
}

// 判断 DeepSeek JSON 响应是否表示 USER_IS_BANNED。
// 匹配规则与 Go 侧 isUserBannedResponse 对齐：data.biz_code == 10 或 data.biz_msg 包含 user_is_banned。
function isBannedJSONResponse(data) {
  if (!data || typeof data !== 'object') {
    return false;
  }
  const d = data.data;
  if (!d || typeof d !== 'object') {
    return false;
  }
  const bizMsg = asString(d.biz_msg).toLowerCase();
  const bizCode = Number(d.biz_code) || 0;
  return bizCode === 10 || bizMsg.includes('user_is_banned');
}

// 从 DeepSeek 响应中解析 mute_until。
// 匹配规则与 Go 侧 detectMutedCompletion/isMutedJSONResponse 对齐：
// - data.biz_code == 5 或 14，或 data.biz_msg 包含 "muted"，视为封号；
// - 优先取 data.biz_data.mute_until 或 data.mute_until。
function extractMuteUntil(data) {
  if (!data || typeof data !== 'object') {
    return 0;
  }
  const d = data.data;
  if (!d || typeof d !== 'object') {
    return 0;
  }
  const bizMsg = asString(d.biz_msg).toLowerCase();
  const bizCode = Number(d.biz_code) || 0;
  if (bizCode !== 5 && bizCode !== 14 && !bizMsg.includes('muted')) {
    return 0;
  }
  const topMuteUntil = Number(d.mute_until);
  if (Number.isFinite(topMuteUntil) && topMuteUntil > 0) {
    return topMuteUntil;
  }
  const bizData = d.biz_data;
  if (!bizData || typeof bizData !== 'object') {
    return 0;
  }
  const muteUntil = Number(bizData.mute_until);
  return Number.isFinite(muteUntil) && muteUntil > 0 ? muteUntil : 0;
}

module.exports = { detectMutedOrBannedResponse };
