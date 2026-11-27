(() => {
  const form = document.getElementById('credentialForm');
  const rows = document.getElementById('credentialRows');
  const message = document.getElementById('credentialMessage');
  const tokenPanel = document.getElementById('newCredential');
  const date = value => value ? new Date(Number(value)).toLocaleString('zh-CN') : '—';
  const cell = value => { const td = document.createElement('td'); td.textContent = value ?? ''; return td; };
  const status = item => item.revoked_at ? '已撤销' : '有效';
  async function refresh() {
    rows.replaceChildren(Object.assign(document.createElement('tr'), { innerHTML: '<td colspan="7" class="table-loading">正在读取…</td>' }));
    try {
      const items = await api.listBatchCredentials();
      rows.replaceChildren();
      if (!items.length) { rows.innerHTML = '<tr><td colspan="7" class="table-loading">还没有创建调用方凭据</td></tr>'; return; }
      items.forEach(item => {
        const tr = document.createElement('tr');
        tr.append(cell(item.caller_id), cell(item.label), cell(item.token_hint), cell(date(item.created_at)), cell(date(item.last_used_at)));
        const badge = document.createElement('td'); badge.innerHTML = item.revoked_at ? '<span class="badge bg-secondary-lt">已撤销</span>' : '<span class="badge bg-green-lt">有效</span>'; tr.append(badge);
        const actions = document.createElement('td'); actions.className = 'text-end';
        if (!item.revoked_at) { const button = document.createElement('button'); button.type = 'button'; button.className = 'btn btn-sm btn-outline-danger'; button.textContent = '撤销'; button.addEventListener('click', async () => {
          if (!confirm(`撤销 ${item.caller_id} 的这把密钥？使用它的客户端将立即无法调用批量 API。`)) return;
          try { await api.revokeBatchCredential(item.id); await refresh(); } catch (error) { alert(error.message); }
        }); actions.append(button); }
        tr.append(actions); rows.append(tr);
      });
    } catch (error) { rows.replaceChildren(); const tr = document.createElement('tr'); const td = cell(error.message); td.colSpan = 7; td.className = 'table-loading text-danger'; tr.append(td); rows.append(tr); }
  }
  form.addEventListener('submit', async event => {
    event.preventDefault(); message.textContent = ''; tokenPanel.classList.add('d-none');
    try {
      const created = await api.createBatchCredential({ caller_id: document.getElementById('callerId').value.trim(), label: document.getElementById('callerLabel').value.trim() });
      document.getElementById('newToken').value = created.token;
      tokenPanel.classList.remove('d-none'); message.textContent = `已为 ${created.credential.caller_id} 创建凭据。`;
      form.reset(); await refresh();
    } catch (error) { message.className = 'small mt-2 text-danger'; message.textContent = error.message; }
  });
  document.getElementById('copyToken').addEventListener('click', async () => {
    const input = document.getElementById('newToken'); input.select();
    try { await navigator.clipboard.writeText(input.value); message.textContent = '密钥已复制。'; } catch { document.execCommand('copy'); message.textContent = '密钥已复制。'; }
  });
  document.getElementById('refreshCredentials').addEventListener('click', refresh);
  refresh();
})();
