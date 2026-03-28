import test from 'node:test';
import assert from 'node:assert/strict';
import { startServer } from './helpers/server.mjs';

test('health route reports service readiness without auth', async () => {
  const server = await startServer();

  try {
    const response = await fetch(`http://127.0.0.1:${server.port}/health`);
    const body = await response.json();

    assert.equal(response.status, 200);
    assert.equal(body.status, 'ok');
    assert.equal(body.service, 'nominatim-cache');
    assert.equal(typeof body.timestamp, 'number');
  } finally {
    await server.stop();
  }
});
