/**
 * Dashboard 页面逻辑
 */

document.addEventListener('DOMContentLoaded', async () => {
  const cacheCountEl = document.getElementById('cacheCount');
  const totalRequestsEl = document.getElementById('totalRequests');
  const cacheHitsEl = document.getElementById('cacheHits');
  const cacheMissesEl = document.getElementById('cacheMisses');
  const hitRateEl = document.getElementById('hitRate');
  const clearBtn = document.getElementById('clearCacheBtn');
  const refreshBtn = document.getElementById('refreshBtn');

  // 加载统计数据
  async function loadStats() {
    try {
      const stats = await api.getStats();

      cacheCountEl.textContent = formatNumber(stats.cache_count);
      totalRequestsEl.textContent = formatNumber(stats.total_requests);
      cacheHitsEl.textContent = formatNumber(stats.cache_hits);
      cacheMissesEl.textContent = formatNumber(stats.cache_misses);
      hitRateEl.textContent = `${stats.hit_rate}%`;

    } catch (error) {
      console.error('加载统计失败:', error);
      alert('加载统计失败: ' + error.message);
    }
  }

  // 加载趋势图
  async function loadTrendChart() {
    try {
      const canvas = document.getElementById('trendChart');
      const ctx = canvas.getContext('2d');

      // 获取真实每日数据
      const dailyStats = await api.getDailyStats();

      const days = dailyStats.map(d => d.label);
      const hits = dailyStats.map(d => d.hits);
      const misses = dailyStats.map(d => d.misses);

      // 绘制简易柱状图
      canvas.width = canvas.parentElement.clientWidth;
      canvas.height = 350;  // 固定画布高度

      const barWidth = (canvas.width - 100) / 7 / 2 - 5;
      const maxHits = Math.max(...hits, 1);
      const maxMisses = Math.max(...misses, 1);
      const maxValue = Math.max(maxHits, maxMisses) * 1.3;

      // 绘制命中柱
      ctx.fillStyle = '#2fb344';
      days.forEach((_, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + 5;
        const height = (hits[i] / maxValue) * (canvas.height - 60);
        ctx.fillRect(x, canvas.height - 40 - height, barWidth, height);
        if (hits[i] > 0) {
          ctx.fillStyle = '#2fb344';
          ctx.font = '10px sans-serif';
          ctx.textAlign = 'center';
          ctx.fillText(hits[i].toString(), x + barWidth / 2, canvas.height - 40 - height - 5);
        }
      });

      // 绘制未命中柱
      ctx.fillStyle = '#d63939';
      days.forEach((_, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + barWidth + 10;
        const height = (misses[i] / maxValue) * (canvas.height - 60);
        ctx.fillRect(x, canvas.height - 40 - height, barWidth, height);
        if (misses[i] > 0) {
          ctx.fillStyle = '#d63939';
          ctx.font = '10px sans-serif';
          ctx.textAlign = 'center';
          ctx.fillText(misses[i].toString(), x + barWidth / 2, canvas.height - 40 - height - 5);
        }
      });

      // X 轴标签
      ctx.fillStyle = '#6c7a91';
      ctx.font = '12px sans-serif';
      days.forEach((day, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + 20;
        ctx.fillText(day, x, canvas.height - 20);
      });

      // 图例 - 用HTML实现
      const oldLegend = canvas.parentElement.querySelector('#chartLegend');
      if (oldLegend) oldLegend.remove();

      const legendDiv = document.createElement('div');
      legendDiv.id = 'chartLegend';
      legendDiv.style.cssText = 'position:absolute; top:5px; right:5px; font-size:12px; display:flex; flex-direction:column; gap:2px;';
      legendDiv.innerHTML = `
        <div style="display:flex; align-items:center; gap:4px;">
          <div style="width:10px; height:10px; background:#2fb344;"></div>
          <span>命中</span>
        </div>
        <div style="display:flex; align-items:center; gap:4px;">
          <div style="width:10px; height:10px; background:#d63939;"></div>
          <span>未命中</span>
        </div>
      `;
      canvas.parentElement.style.position = 'relative';
      canvas.parentElement.appendChild(legendDiv);

    } catch (error) {
      console.error('加载趋势图失败:', error);
    }
  }

  // 清空缓存
  clearBtn.addEventListener('click', async () => {
    if (!confirm('确定要清空所有缓存吗？此操作不可恢复。')) {
      return;
    }

    try {
      const result = await api.clearAllCache();
      alert(`已清空 ${result.deleted_count} 条缓存`);
      loadStats();
      loadTrendChart();
    } catch (error) {
      alert('清空失败: ' + error.message);
    }
  });

  // 刷新
  refreshBtn.addEventListener('click', () => {
    loadStats();
    loadTrendChart();
  });

  // 格式化数字
  function formatNumber(num) {
    return num.toLocaleString('zh-CN');
  }

  // 初始化
  loadStats();
  loadTrendChart();
});
