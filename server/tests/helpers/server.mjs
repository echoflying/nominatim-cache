import { randomInt } from 'node:crypto';
import { spawn } from 'node:child_process';

function createPort() {
  return randomInt(32000, 39000);
}

export async function startServer(env = {}) {
  const port = env.PORT || createPort();
  const proc = spawn('node', ['dist/index.js'], {
    cwd: new URL('../../', import.meta.url),
    env: {
      ...process.env,
      PORT: String(port),
      NODE_ENV: 'test',
      DATABASE_PATH: `./data/test-${port}.db`,
      ADMIN_USERNAME: 'tester',
      ADMIN_PASSWORD: 'secret-pass',
      ...env
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

  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`server start timeout\n${output}`));
    }, 10000);

    proc.once('exit', code => {
      clearTimeout(timeout);
      reject(new Error(`server exited before ready (${code})\n${output}`));
    });

    const onData = () => {
      if (output.includes('服务已启动')) {
        clearTimeout(timeout);
        proc.stdout.off('data', onData);
        proc.stderr.off('data', onData);
        resolve();
      }
    };

    proc.stdout.on('data', onData);
    proc.stderr.on('data', onData);
  });

  return {
    port,
    proc,
    output: () => output,
    async stop() {
      if (!proc.killed) {
        proc.kill('SIGTERM');
      }
      await new Promise(resolve => proc.once('exit', resolve));
    }
  };
}

export async function waitForExit(proc, timeoutMs = 5000) {
  return await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('process exit timeout')), timeoutMs);

    proc.once('exit', code => {
      clearTimeout(timeout);
      resolve(code);
    });
  });
}
