import test from 'node:test';
import assert from 'node:assert/strict';
import { startServer } from './helpers/server.mjs';

test('http responses do not force upgrade-insecure-requests', async () => {
  const server = await startServer();

  try {
    const response = await fetch(`http://127.0.0.1:${server.port}/`);
    const csp = response.headers.get('content-security-policy') || '';

    assert.equal(response.status, 200);
    assert.doesNotMatch(csp, /upgrade-insecure-requests/i);
  } finally {
    await server.stop();
  }
});
