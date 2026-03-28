import test from 'node:test';
import assert from 'node:assert/strict';

test('env path candidates prefer repo root .env before server .env', async () => {
  const mod = await import('../dist/utils/config.js');

  const candidates = mod.getEnvFileCandidates('/workspace/app/server/dist/utils');

  assert.deepEqual(candidates, [
    '/workspace/app/.env',
    '/workspace/app/server/.env'
  ]);
});
