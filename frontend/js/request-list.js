/**
 * 最近请求页面逻辑
 */

document.addEventListener('DOMContentLoaded', () => {
  const tableBody = document.getElementById('requestTableBody');
  const refreshBtn = document.getElementById('refreshBtn');
  const requestStatus = document.getElementById('requestStatus');

  let timer = null;

  async function loadRecentRequests() {
    try {
      const result = await api.getRecentLogs({ page: 1, limit: 100 });
      renderTable(result.data || []);
      requestStatus.textContent = `自动刷新中 · ${formatTime(new Date())}`;
      return true;
    } catch (error) {
      console.error('加载最近请求失败:', error);
      tableBody.innerHTML = `<tr><td colspan="4" class="loading">加载失败: ${error.message}</td></tr>`;
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
      tableBody.innerHTML = '<tr><td colspan="4" class="loading">暂无请求记录</td></tr>';
      return;
    }

    tableBody.innerHTML = data.map(entry => `
      <tr>
        <td>${formatDate(entry.accessed_at)}</td>
        <td>${formatCoords(entry.lat, entry.lon)}</td>
        <td>${entry.cached ? '<span class="success-badge">命中</span>' : '<span class="fail-badge">未命中</span>'}</td>
        <td title="${escapeHtml(entry.cache_key || '')}">${truncateText(entry.cache_key || '-', 36)}</td>
      </tr>
    `).join('');
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
    return message.includes('Authentication required') || message.includes('Invalid credentials');
  }

  function truncateText(text, maxLength) {
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
  }

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  refreshBtn.addEventListener('click', handleManualRefresh);
  document.addEventListener('visibilitychange', handleVisibilityChange);

  startPolling();
});
