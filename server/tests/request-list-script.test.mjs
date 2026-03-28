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
    async trigger(type) {
      const handler = this.listeners.get(type);
      if (!handler) {
        return;
      }
      return await handler();
    }
  };
}

function createHarness(apiImpl) {
  const elements = {
    requestTableBody: createElement(),
    refreshBtn: createElement(),
    requestStatus: createElement()
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
    api: { getRecentLogs: apiImpl },
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
