import test from 'node:test';
import assert from 'node:assert/strict';

test('env path candidates include server and repo root .env files', async () => {
  const mod = await import('../dist/utils/config.js');

  const candidates = mod.getEnvFileCandidates('/workspace/app/server/dist/utils');

  assert.deepEqual(candidates, [
    '/workspace/app/server/.env',
    '/workspace/app/.env'
  ]);
});
