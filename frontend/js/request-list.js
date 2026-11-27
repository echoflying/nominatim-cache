/**
 * 最近请求页面逻辑
 */

document.addEventListener('DOMContentLoaded', () => {
  const tableBody = document.getElementById('requestTableBody');
  const refreshBtn = document.getElementById('refreshBtn');
  const requestStatus = document.getElementById('requestStatus');
  const detail = document.getElementById('requestDetail');
  const detailStatus = document.getElementById('requestDetailStatus');
  const upstreamTitle = document.getElementById('requestUpstreamTitle');
  const upstream = document.getElementById('requestUpstream');
  const output = document.getElementById('requestOutput');

  let timer = null;
  let detailLoad = 0;

  async function loadRecentRequests() {
    try {
      const result = await api.getRecentLogs({ page: 1, limit: 100 });
      renderTable(result.data || []);
      requestStatus.textContent = `自动刷新中 · ${formatTime(new Date())}`;
      return true;
    } catch (error) {
      console.error('加载最近请求失败:', error);
      tableBody.innerHTML = `<tr><td colspan="7" class="table-loading">加载失败: ${escapeHtml(error.message)}</td></tr>`;
      if (isAuthError(error)) {
        stopPolling();
        requestStatus.textContent = '认证失败，请点击立即刷新';
        return false;
      }

      requestStatus.textContent = '刷新失败';
      return false;
    }
  }

  function renderTable(data) {
    if (!data || data.length === 0) {
      tableBody.innerHTML = '<tr><td colspan="6" class="table-loading">暂无请求记录</td></tr>';
      return;
    }

    tableBody.innerHTML = data.map(entry => {
      const id = String(entry.id);
      const detailButton = /^[1-9]\d*$/.test(id)
        ? `<button type="button" class="btn btn-sm btn-outline-primary" data-log-id="${id}" data-cached="${entry.cached ? 1 : 0}">查看详情</button>`
        : '—';
      return `
      <tr>
        <td>${formatDate(entry.accessed_at)}</td>
        <td>${formatCoords(entry.lat, entry.lon)}</td>
        <td>${entry.cached ? '<span class="success-badge">命中</span>' : '<span class="fail-badge">未命中</span>'}</td>
        <td>${entry.place_name ? escapeHtml(entry.place_name) : '<span class="text-secondary">—</span>'}</td>
        <td class="text-nowrap">${entry.source ? escapeHtml(formatSource(entry.source)) : '<span class="text-secondary">未记录</span>'}</td>
        <td title="${escapeHtml(entry.cache_key || '')}">${escapeHtml(truncateText(entry.cache_key || '-', 36))}</td>
        <td class="text-end">${detailButton}</td>
      </tr>
    `;
    }).join('');
  }

  async function showDetail(button) {
    const id = button.dataset.logId;
    const cached = button.dataset.cached === '1';
    const load = ++detailLoad;
    detailStatus.textContent = `请求 #${id} · 正在读取`;
    upstreamTitle.textContent = cached ? '缓存来源' : '本次上游原始响应';
    upstream.textContent = '';
    output.textContent = '';
    detail.showModal();
    try {
      const data = await api.getLogDetail(id);
      if (load !== detailLoad || !detail.open) return;
      const labels = [`请求 #${id}`];
      const source = cached ? data.cache_original_source : data.upstream_source;
      if (source) labels.push(formatSource(source));
      if (data.response_status != null) labels.push(`HTTP ${data.response_status}`);
      detailStatus.textContent = labels.join(' · ');
      if (cached && data.cache_original_body != null) {
        upstreamTitle.textContent = data.cache_original_kind === 'upstream' ? '缓存最初的上游原文' : '缓存入库快照';
      }
      const input = cached ? data.cache_original_body : data.upstream_body;
      const missingInput = cached ? '这条记录未保存缓存来源。' : '这条记录没有可展示的上游原文。';
      upstream.textContent = input == null ? missingInput : formatJSON(input);
      output.textContent = data.response_body == null ? '这条历史记录未保存下发正文。' : formatJSON(data.response_body);
    } catch (error) {
      if (load !== detailLoad || !detail.open) return;
      detailStatus.textContent = `请求 #${id} · ${error.message}`;
    }
  }

  function formatSource(source) {
    return source === 'manual' ? '人工录入' : source;
  }

  function formatJSON(text) {
    try {
      JSON.parse(text);
    } catch {
      return text;
    }

    // Tokenize the original text so large numbers and string escapes survive formatting.
    const tokens = text.match(/"(?:\\[\s\S]|[^"\\])*"|[{}\[\],:]|[^\s{}\[\],:"]+/g);
    const parts = [];
    let depth = 0;
    const lineBreak = () => parts.push('\n', '  '.repeat(depth));
    for (let i = 0; i < tokens.length; i++) {
      const token = tokens[i];
      switch (token) {
        case '{':
        case '[':
          parts.push(token);
          if (++depth > 32) return text;
          if (tokens[i + 1] !== (token === '{' ? '}' : ']')) lineBreak();
          break;
        case '}':
        case ']':
          depth--;
          if (tokens[i - 1] !== (token === '}' ? '{' : '[')) lineBreak();
          parts.push(token);
          break;
        case ',':
          parts.push(',');
          lineBreak();
          break;
        case ':':
          parts.push(': ');
          break;
        default:
          parts.push(token);
      }
    }
    return parts.join('');
  }

  async function startPolling() {
    stopPolling();
    const success = await loadRecentRequests();
    if (success && !document.hidden) {
      timer = window.setInterval(loadRecentRequests, 1000);
    }
  }

  async function handleManualRefresh() {
    const success = await loadRecentRequests();
    if (success && !document.hidden && !timer) {
      timer = window.setInterval(loadRecentRequests, 1000);
    }
  }

  function stopPolling() {
    if (timer) {
      window.clearInterval(timer);
      timer = null;
    }
  }

  function handleVisibilityChange() {
    if (document.hidden) {
      stopPolling();
      requestStatus.textContent = '已暂停';
      return;
    }

    requestStatus.textContent = '自动刷新中';
    startPolling();
  }

  function formatDate(timestamp) {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
  }

  function formatCoords(lat, lon) {
    if (typeof lat !== 'number' || typeof lon !== 'number') return '-';
    return `${lat.toFixed(4)}, ${lon.toFixed(4)}`;
  }

  function formatTime(date) {
    return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}`;
  }

  function isAuthError(error) {
    const message = error?.message || '';
    return message.includes('Authentication required') || message.includes('Invalid credentials') || message.includes('OPS 会话已过期');
  }

  function truncateText(text, maxLength) {
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
  }

  function escapeHtml(text) {
    return String(text).replace(/[&<>"']/g, ch => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[ch]);
  }

  tableBody.addEventListener('click', event => {
    const button = event.target.closest('button[data-log-id]');
    if (button) return showDetail(button);
  });

  refreshBtn.addEventListener('click', handleManualRefresh);
  document.addEventListener('visibilitychange', handleVisibilityChange);

  startPolling();
});
