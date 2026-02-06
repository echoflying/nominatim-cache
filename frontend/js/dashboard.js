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

      cacheCountEl.textContent = `缓存: ${formatNumber(stats.cache_count)}`;
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
      canvas.height = canvas.parentElement.clientHeight;

      const barWidth = (canvas.width - 100) / 7 / 2 - 5;
      const maxValue = Math.max(...hits, ...misses, 1) * 1.2;

      // 绘制命中柱
      ctx.fillStyle = '#28a745';
      days.forEach((_, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + 5;
        const height = (hits[i] / maxValue) * (canvas.height - 60);
        ctx.fillRect(x, canvas.height - 40 - height, barWidth, height);
      });

      // 绘制未命中柱
      ctx.fillStyle = '#cb2431';
      days.forEach((_, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + barWidth + 10;
        const height = (misses[i] / maxValue) * (canvas.height - 60);
        ctx.fillRect(x, canvas.height - 40 - height, barWidth, height);
      });

      // X 轴标签
      ctx.fillStyle = '#586069';
      ctx.font = '12px sans-serif';
      days.forEach((day, i) => {
        const x = 50 + i * ((canvas.width - 100) / 7) + 20;
        ctx.fillText(day, x, canvas.height - 20);
      });

      // 图例
      ctx.fillStyle = '#28a745';
      ctx.fillRect(canvas.width - 120, 10, 12, 12);
      ctx.fillStyle = '#333';
      ctx.fillText('命中', canvas.width - 100, 20);

      ctx.fillStyle = '#cb2431';
      ctx.fillRect(canvas.width - 60, 10, 12, 12);
      ctx.fillStyle = '#333';
      ctx.fillText('未命中', canvas.width - 40, 20);

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
