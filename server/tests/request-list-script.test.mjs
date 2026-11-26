import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const scriptPath = new URL('../../frontend/js/request-list.js', import.meta.url);
const scriptSource = await readFile(scriptPath, 'utf8');

function createElement() {
  return {
    textContent: '',
    innerHTML: '',
    title: '',
    listeners: new Map(),
    addEventListener(type, handler) {
      this.listeners.set(type, handler);
    },
    async trigger(type, event) {
      const handler = this.listeners.get(type);
      if (!handler) {
        return;
      }
      return await handler(event);
    }
  };
}

function createHarness(apiImpl, detailImpl) {
  const elements = {
    requestTableBody: createElement(),
    refreshBtn: createElement(),
    requestStatus: createElement(),
    requestDetail: { open: false, showModal() { this.open = true; } },
    requestDetailStatus: createElement(),
    requestUpstreamTitle: createElement(),
    requestUpstream: createElement(),
    requestOutput: createElement()
  };

  const documentListeners = new Map();
  const intervals = new Map();
  let nextIntervalId = 1;

  const document = {
    hidden: false,
    addEventListener(type, handler) {
      documentListeners.set(type, handler);
    },
    getElementById(id) {
      return elements[id];
    },
    createElement() {
      return {
        _textContent: '',
        set textContent(value) {
          this._textContent = value;
        },
        get textContent() {
          return this._textContent;
        },
        get innerHTML() {
          return this._textContent
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
        }
      };
    }
  };

  const window = {
    setInterval(fn, delay) {
      const id = nextIntervalId++;
      intervals.set(id, { fn, delay });
      return id;
    },
    clearInterval(id) {
      intervals.delete(id);
    }
  };

  const context = vm.createContext({
    api: { getRecentLogs: apiImpl, getLogDetail: detailImpl },
    console,
    document,
    window,
    Date,
    URLSearchParams,
    setTimeout,
    clearTimeout
  });

  vm.runInContext(scriptSource, context, { filename: 'request-list.js' });

  async function dispatchDocumentEvent(type) {
    const handler = documentListeners.get(type);
    if (handler) {
      await handler();
    }
  }

  async function flushAsyncWork() {
    await Promise.resolve();
    await Promise.resolve();
  }

  return {
    elements,
    document,
    intervals,
    context,
    dispatchDocumentEvent,
    flushAsyncWork,
    getDocumentListener(type) {
      return documentListeners.get(type);
    }
  };
}

test('auth failure stops polling until user retries manually', async () => {
  let shouldFailAuth = true;
  const paramsHistory = [];
  const harness = createHarness(async params => {
    paramsHistory.push(params);
    if (shouldFailAuth) {
      throw new Error('Authentication required');
    }

    return {
      data: [
        {
          accessed_at: Date.now(),
          lat: 39.9,
          lon: 116.4,
          cached: true,
          cache_key: '39.9,116.4'
        }
      ]
    };
  });

  await harness.dispatchDocumentEvent('DOMContentLoaded');
  await harness.flushAsyncWork();

  assert.equal(harness.intervals.size, 0);
  assert.equal(harness.elements.requestStatus.textContent, '认证失败，请点击立即刷新');
  assert.equal(JSON.stringify(paramsHistory[0]), JSON.stringify({ page: 1, limit: 100 }));

  shouldFailAuth = false;
  await harness.elements.refreshBtn.trigger('click');
  await harness.flushAsyncWork();

  assert.equal(harness.intervals.size, 1);
  assert.match(harness.elements.requestTableBody.innerHTML, /39\.9000, 116\.4000/);
});

test('page polls while visible and pauses when hidden', async () => {
  let callCount = 0;
  const paramsHistory = [];
  const harness = createHarness(async params => {
    paramsHistory.push(params);
    callCount += 1;
    return {
      data: [
        {
          accessed_at: Date.now(),
          lat: 30.2,
          lon: 120.1,
          cached: callCount % 2 === 1,
          cache_key: `key-${callCount}`
        }
      ]
    };
  });

  await harness.dispatchDocumentEvent('DOMContentLoaded');
  await harness.flushAsyncWork();

  assert.equal(callCount, 1);
  assert.equal(harness.intervals.size, 1);
  assert.match(harness.elements.requestStatus.textContent, /自动刷新中/);
  assert.equal(JSON.stringify(paramsHistory[0]), JSON.stringify({ page: 1, limit: 100 }));

  harness.document.hidden = true;
  await harness.getDocumentListener('visibilitychange')();

  assert.equal(harness.intervals.size, 0);
  assert.equal(harness.elements.requestStatus.textContent, '已暂停');

  harness.document.hidden = false;
  await harness.getDocumentListener('visibilitychange')();
  await harness.flushAsyncWork();

  assert.equal(callCount, 2);
  assert.equal(harness.intervals.size, 1);
});

test('response details format JSON on click without rounding numbers or changing plain text', async () => {
  const entry = { id: 42, cached: false, cache_key: '1.0000_2.0000' };
  let raw = '{"upstream_id":12345678901234567890,"value":"<example>","extra":[]}';
  const output = '{"address":{"neighbourhood":"后海"},"lat":"22.5109709","place_name":"后海"}\n';
  const calls = [];
  const harness = createHarness(async () => ({ data: [entry] }), async id => {
    calls.push(id);
    return { upstream_source: 'photon.komoot.io', upstream_body: raw, response_status: 200, response_body: output };
  });

  await harness.dispatchDocumentEvent('DOMContentLoaded');
  await harness.flushAsyncWork();
  assert.deepEqual(calls, []);
  assert.match(harness.elements.requestTableBody.innerHTML, /data-log-id="42"/);
  await harness.elements.requestTableBody.trigger('click', {
    target: { closest: () => ({ dataset: { logId: '42', cached: '0' } }) }
  });
  assert.deepEqual(calls, ['42']);
  assert.equal(harness.elements.requestDetail.open, true);
  assert.equal(harness.elements.requestDetailStatus.textContent, '请求 #42 · photon.komoot.io · HTTP 200');
  assert.equal(harness.elements.requestUpstream.textContent, [
    '{', '  "upstream_id": 12345678901234567890,', '  "value": "<example>",', '  "extra": []', '}'
  ].join('\n'));
  assert.equal(harness.elements.requestOutput.textContent, [
    '{', '  "address": {', '    "neighbourhood": "后海"', '  },', '  "lat": "22.5109709",', '  "place_name": "后海"', '}'
  ].join('\n'));

  raw = '  <html>上游错误</html>\n';
  harness.elements.requestDetail.open = false;
  await harness.elements.requestTableBody.trigger('click', {
    target: { closest: () => ({ dataset: { logId: '42', cached: '0' } }) }
  });
  assert.equal(harness.elements.requestUpstream.textContent, raw);
});

test('request details show cache provenance or a missing-source note alongside the actual output', async () => {
  const replies = {
    42: {
      cache_original_source: 'photon.komoot.io',
      cache_original_kind: 'upstream',
      cache_original_body: '{"type":"FeatureCollection","features":[]}',
      response_status: 200,
      response_body: '{"place_name":"后海","cached":true}'
    },
    43: { response_status: 500, response_body: '{"error":"Cache unavailable"}' }
  };
  const harness = createHarness(async () => ({ data: [] }), async id => replies[id]);
  await harness.dispatchDocumentEvent('DOMContentLoaded');
  for (const [id, cached] of [['42', '1'], ['43', '0']]) {
    harness.elements.requestDetail.open = false;
    await harness.elements.requestTableBody.trigger('click', {
      target: { closest: () => ({ dataset: { logId: id, cached } }) }
    });
    if (cached === '1') {
      assert.equal(harness.elements.requestUpstreamTitle.textContent, '缓存最初的上游原文');
      assert.match(harness.elements.requestUpstream.textContent, /"type": "FeatureCollection"/);
      assert.match(harness.elements.requestOutput.textContent, /"place_name": "后海"/);
    } else {
      assert.match(harness.elements.requestUpstream.textContent, /没有可展示的上游原文/);
      assert.match(harness.elements.requestOutput.textContent, /"error": "Cache unavailable"/);
    }
  }
});

test('recent requests show recorded sources without loading request details', async () => {
  const harness = createHarness(async () => ({
    data: [
      { id: 1, cached: 0, source: 'photon.komoot.io' },
      { id: 2, cached: 1, source: 'manual' },
      { id: 3, cached: 1, source: null }
    ]
  }), () => assert.fail('summary rendering should not load detail'));
  await harness.dispatchDocumentEvent('DOMContentLoaded');
  await harness.flushAsyncWork();
  const rows = harness.elements.requestTableBody.innerHTML;
  assert.match(rows, /class="text-nowrap">photon\.komoot\.io</);
  assert.match(rows, /class="text-nowrap">人工录入</);
  assert.match(rows, /class="text-secondary">未记录</);
});
