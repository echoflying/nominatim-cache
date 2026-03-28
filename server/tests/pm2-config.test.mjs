import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);

test('pm2 ecosystem config can be loaded from CommonJS in ESM project', () => {
  const config = require('../ecosystem.config.cjs');

  assert.equal(config.apps[0].name, 'nominatim-cache');
  assert.equal(config.apps[0].env.NODE_ENV, 'production');
});

test('mac mini deploy script exists in repository', () => {
  const scriptPath = path.resolve(process.cwd(), '../scripts/deploy-macmini.sh');

  assert.equal(fs.existsSync(scriptPath), true);
});

test('launchd path sanitizer removes duplicates while preserving required bins', async () => {
  const mod = await import('../dist/utils/config.js');
  const sanitized = mod.sanitizePathEntries([
    '/opt/local/bin',
    '/Users/weifeng/.npm-global/bin',
    '/opt/local/bin',
    '/usr/local/bin',
    '/Users/weifeng/.npm-global/bin',
    '/usr/bin'
  ]);

  assert.deepEqual(sanitized, [
    '/opt/local/bin',
    '/Users/weifeng/.npm-global/bin',
    '/usr/local/bin',
    '/usr/bin'
  ]);
});
