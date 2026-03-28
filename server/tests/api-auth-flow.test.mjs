import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const scriptPath = new URL('../../frontend/js/api.js', import.meta.url);
const scriptSource = await readFile(scriptPath, 'utf8');

function createResponse(status, body) {
  return {
    status,
    async json() {
      return body;
    }
  };
}

function createApiHarness({ promptValues = [], fetchImpl, storedAuth = null }) {
  const storage = new Map();
  if (storedAuth !== null) {
    storage.set('nominatim_auth', storedAuth);
  }

  let promptIndex = 0;
  const prompts = [];
  const fetchCalls = [];

  const localStorage = {
    getItem(key) {
      return storage.has(key) ? storage.get(key) : null;
    },
    setItem(key, value) {
      storage.set(key, value);
    },
    removeItem(key) {
      storage.delete(key);
    }
  };

  const fetch = async (url, options = {}) => {
    fetchCalls.push({ url, options });
    return await fetchImpl(url, options);
  };

  const context = vm.createContext({
    fetch,
    localStorage,
    URL,
    URLSearchParams,
    window: {
      prompt(message) {
        prompts.push(message);
        const value = promptValues[promptIndex];
        promptIndex += 1;
        return value ?? null;
      }
    },
    btoa(value) {
      return Buffer.from(value, 'utf8').toString('base64');
    }
  });

  vm.runInContext(scriptSource, context, { filename: 'api.js' });
  vm.runInContext('globalThis.__api = api;', context);

  return {
    api: context.__api,
    prompts,
    fetchCalls,
    storage
  };
}

test('cancelled auth prompt does not trigger a second prompt after 401', async () => {
  const harness = createApiHarness({
    promptValues: [null],
    fetchImpl: async () => createResponse(401, {
      success: false,
      error: 'Authentication required'
    })
  });

  await assert.rejects(() => harness.api.getStats(), /Authentication required/);

  assert.equal(harness.prompts.length, 1);
  assert.equal(harness.fetchCalls.length, 1);
});

test('invalid prompted credentials do not trigger an immediate reprompt loop', async () => {
  const harness = createApiHarness({
    promptValues: ['tester', 'wrong-pass'],
    fetchImpl: async () => createResponse(401, {
      success: false,
      error: 'Invalid credentials'
    })
  });

  await assert.rejects(() => harness.api.getStats(), /Invalid credentials/);

  assert.equal(harness.prompts.length, 2);
  assert.equal(harness.fetchCalls.length, 1);
});

test('stored invalid credentials retry once with fresh prompt', async () => {
  let callCount = 0;
  const harness = createApiHarness({
    storedAuth: 'b2xkOnBhc3M=',
    promptValues: ['tester', 'secret-pass'],
    fetchImpl: async (url, options) => {
      callCount += 1;
      if (callCount === 1) {
        assert.equal(options.headers.Authorization, 'Basic b2xkOnBhc3M=');
        return createResponse(401, {
          success: false,
          error: 'Invalid credentials'
        });
      }

      assert.equal(options.headers.Authorization, 'Basic dGVzdGVyOnNlY3JldC1wYXNz');
      return createResponse(200, {
        success: true,
        data: { cache_count: 1 }
      });
    }
  });

  const result = await harness.api.getStats();

  assert.deepEqual(result, { cache_count: 1 });
  assert.equal(harness.prompts.length, 2);
  assert.equal(harness.fetchCalls.length, 2);
});
