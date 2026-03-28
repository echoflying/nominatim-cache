import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

test('pm2 ecosystem config can be loaded from CommonJS in ESM project', () => {
  const config = require('../ecosystem.config.cjs');

  assert.equal(config.apps[0].name, 'nominatim-cache');
  assert.equal(config.apps[0].env.NODE_ENV, 'production');
});
