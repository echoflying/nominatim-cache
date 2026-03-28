import test from 'node:test';
import assert from 'node:assert/strict';
import { startServer } from './helpers/server.mjs';

test('dashboard links to recent requests page and page is reachable', async () => {
  const server = await startServer();

  try {
    const dashboardResponse = await fetch(`http://127.0.0.1:${server.port}/`);
    const dashboardHtml = await dashboardResponse.text();

    assert.equal(dashboardResponse.status, 200);
    assert.match(dashboardHtml, /href="\/requests\.html"/);

    const requestsResponse = await fetch(`http://127.0.0.1:${server.port}/requests.html`);
    const requestsHtml = await requestsResponse.text();

    assert.equal(requestsResponse.status, 200);
    assert.match(requestsHtml, /最近请求/);
    assert.match(requestsHtml, /request-list\.js/);
  } finally {
    await server.stop();
  }
});
