document.addEventListener('DOMContentLoaded', () => {
  const number = value => new Intl.NumberFormat('zh-CN').format(value || 0);
  const labels = {
    single_reverse: '单点反查', batch_submit: '批量提交', batch_status: '批量状态查询', batch_results: '批量结果读取',
    anonymous: '匿名调用', unauthenticated: '未认证批量请求', invalid_credential: '无效凭据', unknown: '未提供', unspecified: '未提供'
  };
  const setRows = (id, items, columns, emptyColumns) => {
    const body = document.getElementById(id);
    if (!items?.length) {
      body.innerHTML = `<tr><td colspan="${emptyColumns}" class="text-secondary text-center py-4">暂无统计数据</td></tr>`;
      return;
    }
    body.innerHTML = items.map(item => `<tr>${columns.map(column => `<td${column.align ? ' class="text-end"' : ''}>${column.render(item)}</td>`).join('')}</tr>`).join('');
  };
  const name = item => labels[item.name] || item.name || '—';
  const errorCount = item => (item.client_errors || 0) + (item.server_errors || 0);
  const load = async () => {
    const status = document.getElementById('usageStatus');
    status.textContent = '正在读取…';
    try {
      const report = await api.getUsageStats(Number(document.getElementById('rangeSelect').value));
      document.getElementById('requests').textContent = number(report.summary.requests);
      document.getElementById('points').textContent = number(report.summary.points);
      document.getElementById('cacheHits').textContent = number(report.summary.cache_hits);
      document.getElementById('upstreamPoints').textContent = number(report.summary.upstream_points);
      const cols = [
        { render: name }, { align: true, render: item => number(item.requests) },
        { align: true, render: item => number(item.points) },
        { align: true, render: item => number(item.cache_hits) }
      ];
      setRows('callerRows', report.by_caller, cols, 4);
      setRows('operationRows', report.by_operation, [
        { render: name }, { align: true, render: item => number(item.requests) },
        { align: true, render: item => number(item.points) }, { align: true, render: item => number(errorCount(item)) }
      ], 4);
      setRows('platformRows', report.by_platform, [
        { render: name }, { align: true, render: item => number(item.requests) },
        { align: true, render: item => number(item.points) }, { align: true, render: item => `${number(item.duration_avg_ms)} ms` }
      ], 4);
      setRows('versionRows', report.by_version, [
        { render: name }, { align: true, render: item => number(item.requests) },
        { align: true, render: item => number(item.points) }, { align: true, render: item => number(errorCount(item)) }
      ], 4);
      setRows('dailyRows', report.daily, [
        { render: item => item.name }, { align: true, render: item => number(item.requests) },
        { align: true, render: item => number(item.points) }, { align: true, render: item => number(item.cache_hits) },
        { align: true, render: item => number(item.upstream_points) }, { align: true, render: item => number(item.rate_limited) },
        { align: true, render: item => number(errorCount(item)) }, { align: true, render: item => `${number(item.duration_avg_ms)} ms` }
      ], 8);
      status.textContent = `${report.from} 至 ${report.to} · 明细保存 90 天后汇总`;
    } catch (error) {
      status.textContent = error.message || '读取统计失败';
    }
  };
  document.getElementById('refreshBtn').addEventListener('click', load);
  document.getElementById('rangeSelect').addEventListener('change', load);
  load();
});
