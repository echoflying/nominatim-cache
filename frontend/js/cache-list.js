/**
 * 缓存列表页面逻辑
 */

document.addEventListener('DOMContentLoaded', () => {
  const tableBody = document.getElementById('cacheTableBody');
  const searchInput = document.getElementById('searchInput');
  const searchBtn = document.getElementById('searchBtn');
  const refreshBtn = document.getElementById('refreshBtn');
  const prevBtn = document.getElementById('prevBtn');
  const nextBtn = document.getElementById('nextBtn');
  const pageInfo = document.getElementById('pageInfo');

  let currentPage = 1;
  const pageSize = 20;
  let currentSearch = '';

  const modal = document.getElementById('queryModal');
  const closeModalBtn = document.getElementById('closeQueryModal');
  const cancelBtn = document.getElementById('cancelQueryBtn');
  const addToCacheBtn = document.getElementById('addToCacheBtn');
  const queryCoordsLabel = document.getElementById('queryCoords');
  const queryResultsDiv = document.getElementById('queryResults');
  const selectedDisplayNameInput = document.getElementById('selectedDisplayName');

  let currentQueryData = null;
  let selectedResultIndex = 0;

  // 加载缓存列表
  async function loadCacheList() {
    try {
      tableBody.innerHTML = '<tr><td colspan="6" class="loading">加载中...</td></tr>';

      const result = await api.getCacheList({
        page: currentPage,
        limit: pageSize,
        search: currentSearch
      });

      renderTable(result.data);
      updatePagination(result.pagination);

    } catch (error) {
      console.error('加载列表失败:', error);
      tableBody.innerHTML = `<tr><td colspan="6" class="loading">加载失败: ${error.message}</td></tr>`;
    }
  }

  // 渲染表格
  function renderTable(data) {
    if (!data || data.length === 0) {
      tableBody.innerHTML = '<tr><td colspan="7" class="loading">暂无缓存数据</td></tr>';
      return;
    }

    tableBody.innerHTML = data.map(entry => `
      <tr>
        <td>${entry.lat.toFixed(4)}, ${entry.lon.toFixed(4)}</td>
        <td>${truncateText(entry.display_name, 30)}</td>
        <td><span class="type-badge">${getTypeLabel(entry.place_type)}</span></td>
        <td><span class="source-badge ${entry.source || 'unknown'}">${getSourceLabel(entry.source)}</span></td>
        <td>${formatDate(entry.first_cached_at)}</td>
        <td>${entry.access_count}</td>
        <td>
          <div class="action-btns">
            <button class="action-btn delete" data-key="${entry.cache_key}">删除</button>
            <button class="action-btn query" data-lat="${entry.lat}" data-lon="${entry.lon}">查询上游</button>
          </div>
        </td>
      </tr>
    `).join('');

    // 绑定事件
    document.querySelectorAll('.action-btn.delete').forEach(btn => {
      btn.addEventListener('click', () => handleDelete(btn.dataset.key, btn));
    });
    document.querySelectorAll('.action-btn.query').forEach(btn => {
      btn.addEventListener('click', () => handleQueryUpstream(parseFloat(btn.dataset.lat), parseFloat(btn.dataset.lon)));
    });
  }

  // 更新分页
  function updatePagination(pagination) {
    pageInfo.textContent = `第 ${pagination.page} 页 / 共 ${pagination.total_pages} 页`;
    prevBtn.disabled = pagination.page <= 1;
    nextBtn.disabled = !pagination.has_more;
  }

  // 搜索
  function handleSearch() {
    currentSearch = searchInput.value.trim();
    currentPage = 1;
    loadCacheList();
  }

  // 删除缓存
  async function handleDelete(cacheKey, btn) {
    if (!confirm('确定要删除这条缓存吗？')) {
      return;
    }

    const originalText = btn.textContent;
    btn.textContent = '已删除';
    btn.disabled = true;
    btn.classList.add('deleted');

    try {
      await api.deleteCache(cacheKey);
      showToast('已删除');
      loadCacheList();
    } catch (error) {
      console.error('删除失败:', error);
      btn.textContent = originalText;
      btn.disabled = false;
      showToast('删除失败: ' + error.message, 'error');
    }
  }

  function showToast(message, type = 'success') {
    const existing = document.querySelector('.toast');
    if (existing) existing.remove();

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    document.body.appendChild(toast);

    setTimeout(() => {
      toast.classList.add('fade-out');
      setTimeout(() => toast.remove(), 300);
    }, 2000);
  }

  // 查询上游
  async function handleQueryUpstream(lat, lon) {
    try {
      queryCoordsLabel.textContent = `${lat.toFixed(4)}, ${lon.toFixed(4)}`;
      queryResultsDiv.innerHTML = '<p class="loading">查询所有上游源中...</p>';
      selectedDisplayNameInput.value = '';
      modal.classList.add('show');

      const result = await api.queryUpstream(lat, lon);

      currentQueryData = {
        lat: result.coordinates.lat,
        lon: result.coordinates.lon
      };

      // 显示摘要
      const summary = result.summary;
      let summaryHtml = `<div class="query-summary">`;
      if (summary.failed > 0) {
        summaryHtml += `<span class="summary-badge fail">${summary.failed}个失败</span>`;
      }
      summaryHtml += `<span class="summary-badge success">${summary.success}个成功</span>`;
      summaryHtml += `</div>`;

      renderQueryResults(result.results, 0, summaryHtml);
    } catch (error) {
      console.error('查询上游失败:', error);
      queryResultsDiv.innerHTML = `<p class="loading">查询失败: ${error.message}</p>`;
    }
  }

  // 渲染查询结果
  function renderQueryResults(results, selectedIndex, summaryHtml = '') {
    selectedResultIndex = selectedIndex;

    const html = summaryHtml + results.map((result, index) => {
      const isSelected = index === selectedIndex;
      const isDisabled = !result.success;
      const sourceClass = result.success ? 'source-tag' : 'source-tag disabled';
      const itemClass = `result-item ${isSelected ? 'selected' : ''} ${isDisabled ? 'disabled' : ''}`;
      
      return `
        <div class="${itemClass}" data-index="${index}" ${isDisabled ? '' : `data-source="${result.source}"`}>
          <div class="result-header">
            <span class="${sourceClass}">${result.source}</span>
            ${!result.success ? '<span class="fail-badge">失败</span>' : '<span class="success-badge">可用</span>'}
          </div>
          <div class="result-display-name">${escapeHtml(result.display_name)}</div>
        </div>
      `;
    }).join('');

    queryResultsDiv.innerHTML = html;

    if (results[selectedIndex]) {
      selectedDisplayNameInput.value = results[selectedIndex].display_name;
    }

    // 绑定点击事件
    document.querySelectorAll('.result-item:not(.disabled)').forEach(item => {
      item.addEventListener('click', () => {
        const index = parseInt(item.dataset.index);
        renderQueryResults(results, index, summaryHtml);
      });
    });
  }

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  // 添加到缓存
  async function handleAddToCache() {
    const displayName = selectedDisplayNameInput.value.trim();
    if (!displayName) {
      alert('请输入地名');
      return;
    }

    try {
      const result = await api.addManualCache({
        lat: currentQueryData.lat,
        lon: currentQueryData.lon,
        display_name: displayName,
        address: {},
        source: 'manual'
      });

      alert(result.message);
      closeModal();
      loadCacheList();
    } catch (error) {
      console.error('添加到缓存失败:', error);
      alert('添加到缓存失败: ' + error.message);
    }
  }

  // 关闭模态框
  function closeModal() {
    modal.classList.remove('show');
    currentQueryData = null;
    selectedResultIndex = 0;
  }

  // 上一页
  prevBtn.addEventListener('click', () => {
    if (currentPage > 1) {
      currentPage--;
      loadCacheList();
    }
  });

  // 下一页
  nextBtn.addEventListener('click', () => {
    currentPage++;
    loadCacheList();
  });

  // 搜索按钮
  searchBtn.addEventListener('click', handleSearch);

  // 回车搜索
  searchInput.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
      handleSearch();
    }
  });

  // 刷新
  refreshBtn.addEventListener('click', () => {
    loadCacheList();
  });

  // 模态框关闭按钮
  closeModalBtn.addEventListener('click', closeModal);
  cancelBtn.addEventListener('click', closeModal);

  // 添加到缓存按钮
  addToCacheBtn.addEventListener('click', handleAddToCache);

  // 点击模态框外部关闭
  modal.addEventListener('click', (e) => {
    if (e.target === modal) {
      closeModal();
    }
  });

  // 格式化日期
  function formatDate(timestamp) {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
  }

  // 截断文本
  function truncateText(text, maxLength) {
    if (!text) return '-';
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
  }

  // 获取类型标签
  function getTypeLabel(type) {
    const labels = {
      temple: '寺庙',
      peak: '山峰',
      city: '城市',
      street: '街道',
      other: '其他'
    };
    return labels[type] || type || '其他';
  }

  // 获取来源标签
  function getSourceLabel(source) {
    const labels = {
      'mirror-earth.com': 'Earth',
      'photon.komoot.io': 'Photon',
      'openstreetmap.org': 'OSM',
      'manual': '手动'
    };
    return labels[source] || source || '-';
  }

  // 初始化
  loadCacheList();
});
