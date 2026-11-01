'use strict';

const { isContentSnapshot } = require('./sse_snapshot');
const { shouldSkipPath, isStatusPath, isToolFragmentType, isFragmentScopedPath } = require('./sse_paths');

// Keep source fields separate. APPEND occurrences, even identical ones, are
// never deduplicated; SETs only extend or replay their own source field.
class SnapshotDecoder {
  constructor() { this.fields = new Map(); this.aliases = new Map(); this.last = new Map(); this.nextFragment = 0; this.error = ''; }
  normalize(chunk, kind, thinkingEnabled) {
    this.cursor = kind; this.thinkingEnabled = thinkingEnabled;
    const first = this.fields.size === 0;
    const out = this.patch(chunk, '', kind, false);
    if (!first || !isContentSnapshot(chunk)) out.o = 'APPEND';
    return out;
  }
  path(parent, child) {
    child = typeof child === 'string' ? child.replace(/^\//, '') : '';
    if (child === 'response' || child.startsWith('response/')) return child;
    return parent ? parent.replace(/\/$/, '') + '/' + child : 'response/' + child;
  }
  patch(node, parent, kind, inherited) {
    const out = { ...node }, path = typeof node.p === 'string' ? node.p : '';
    const full = this.path(parent, path);
    if (shouldSkipPath(path) || isStatusPath(path)) return out;
    const op = typeof node.o === 'string' ? node.o.toUpperCase() : '';
    let snapshot = inherited || op === 'SET' || isContentSnapshot(node);
    if (op === 'APPEND') snapshot = false;
    if (full.includes('thinking')) { kind = 'thinking'; if (!(this.thinkingEnabled && this.cursor === 'text')) this.cursor = kind; }
    if (isFragmentScopedPath(parent) && ['content', 'response/content'].includes(path)) kind = 'thinking';
    else if (path === 'response/content') kind = this.cursor = 'text';
    if (typeof node.v === 'string') out.v = this.delta(this.contentKey(full, kind), kind, node.v, snapshot);
    else if (Array.isArray(node.v)) out.v = this.array(node.v, full, snapshot);
    else if (node.v && typeof node.v === 'object') out.v = this.object(node.v, full, kind, snapshot);
    return out;
  }
  contentKey(path, kind) {
    if (this.aliases.has(path)) return this.aliases.get(path);
    if (path.includes('/fragments/')) {
      const match = /\/fragments\/(-\d+)\//.exec(path);
      if (match) {
        const alias = 'response/fragments/' + (this.nextFragment + Number(match[1])) + '/content';
        if (this.aliases.has(alias)) return this.aliases.get(alias);
        if (this.last.has(kind)) return this.last.get(kind);
      }
      return path;
    }
    return this.last.get(kind) || 'channel:' + kind;
  }
  delta(key, kind, text, snapshot) {
    if (!text) return '';
    let previous = this.fields.get(key);
    if (previous === undefined && snapshot && this.last.get(kind)?.startsWith('channel:')) previous = this.fields.get(this.last.get(kind));
    previous ||= '';
    if (snapshot) {
      if (text.startsWith(previous)) text = text.slice(previous.length);
      else if (previous.startsWith(text)) text = '';
      else { this.error = 'upstream snapshot rewrote an already delivered content field'; text = ''; }
    }
    this.fields.set(key, previous + text);
    this.last.set(kind, key);
    return text;
  }
  array(values, path, snapshot) {
    const fragments = path.endsWith('/fragments');
    const base = fragments && !snapshot ? this.nextFragment : 0;
    const out = values.map((node, i) => {
      const kind = this.cursor;
      if (!node || typeof node !== 'object' || Array.isArray(node)) return node;
      return node.type !== undefined && node.content !== undefined
        ? this.fragment(node, base + i, kind, snapshot) : this.patch(node, path, kind, snapshot);
    });
    if (fragments) this.nextFragment = Math.max(this.nextFragment, base + values.length);
    return out;
  }
  object(value, path, kind, snapshot) {
    const out = { ...value };
    if (value.response && typeof value.response === 'object') out.response = this.object(value.response, 'response', kind, snapshot);
    else if (Array.isArray(value.fragments)) out.fragments = this.array(value.fragments, 'response/fragments', snapshot);
    else {
      for (const field of ['text', 'content']) {
        if (typeof value[field] === 'string') { out[field] = this.delta(this.contentKey(path, kind), kind, value[field], snapshot); break; }
      }
    }
    return out;
  }
  fragment(node, index, kind, snapshot) {
    const typ = String(node.type || '').toUpperCase();
    if (typ === 'THINK' || typ === 'THINKING') { kind = 'thinking'; if (!(this.thinkingEnabled && this.cursor === 'text')) this.cursor = kind; }
    else if (typ === 'RESPONSE') kind = this.cursor = 'text';
    else if (isToolFragmentType(typ)) kind = 'thinking';
    const alias = 'response/fragments/' + index + '/content';
    const key = ['string', 'number'].includes(typeof node.id) ? 'fragment:' + JSON.stringify(node.id) : alias;
    this.aliases.set(alias, key);
    return { ...node, content: typeof node.content === 'string' ? this.delta(key, kind, node.content, snapshot) : node.content };
  }
}

function coalesceSnapshotParts(parts) {
  const out = [];
  for (const part of parts) {
    const last = out[out.length - 1];
    if (last && last.type === part.type) last.text += part.text;
    else out.push({ ...part });
  }
  return out;
}

module.exports = { SnapshotDecoder, coalesceSnapshotParts };
