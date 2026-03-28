import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { waitForExit } from './helpers/server.mjs';

test('production startup rejects default admin credentials', async () => {
  const proc = spawn('node', ['dist/index.js'], {
    cwd: new URL('../', import.meta.url),
    env: {
      ...process.env,
      NODE_ENV: 'production',
      PORT: '33100',
      DATABASE_PATH: './data/prod-test.db'
    },
    stdio: ['ignore', 'pipe', 'pipe']
  });

  let output = '';
  proc.stdout.on('data', chunk => {
    output += chunk.toString();
  });
  proc.stderr.on('data', chunk => {
    output += chunk.toString();
  });

  const code = await waitForExit(proc, 10000);

  assert.notEqual(code, 0);
  assert.match(output, /ADMIN_USERNAME|ADMIN_PASSWORD|默认密码|default credentials/i);
});
