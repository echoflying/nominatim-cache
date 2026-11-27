(() => {
  const form = document.getElementById('credentialForm');
  const rows = document.getElementById('credentialRows');
  const message = document.getElementById('credentialMessage');
  const tokenPanel = document.getElementById('newCredential');
  const date = value => value ? new Date(Number(value)).toLocaleString('zh-CN') : '—';
  const cell = value => { const td = document.createElement('td'); td.textContent = value ?? ''; return td; };

  async function refresh() {
    rows.replaceChildren();
    const loading = cell('正在读取…'); loading.colSpan = 8; loading.className = 'table-loading';
    const loadingRow = document.createElement('tr'); loadingRow.append(loading); rows.append(loadingRow);
    try {
      const items = await api.listBatchCredentials();
      rows.replaceChildren();
      if (!items.length) {
        const empty = cell('还没有创建调用方凭据'); empty.colSpan = 8; empty.className = 'table-loading';
        const emptyRow = document.createElement('tr'); emptyRow.append(empty); rows.append(emptyRow); return;
      }
      items.forEach(item => {
        const tr = document.createElement('tr');
        tr.append(cell(item.caller_id), cell(item.label), cell(item.token_hint), cell(date(item.created_at)), cell(date(item.last_used_at)));
        const revealedAt = cell(date(item.last_revealed_at)); tr.append(revealedAt);
        const badge = document.createElement('td');
        badge.innerHTML = item.revoked_at ? '<span class="badge bg-secondary-lt">已撤销</span>' : '<span class="badge bg-green-lt">有效</span>';
        tr.append(badge);

        const actions = document.createElement('td'); actions.className = 'text-end';
        const revealButton = document.createElement('button'); revealButton.type = 'button'; revealButton.className = 'btn btn-sm btn-outline-secondary'; revealButton.textContent = '查看密钥';
        revealButton.addEventListener('click', async () => {
          const existing = actions.querySelector('.credential-secret');
          if (existing) { existing.remove(); revealButton.textContent = '查看密钥'; return; }
          revealButton.disabled = true;
          try {
            const token = await api.revealBatchCredential(item.id);
            const group = document.createElement('div'); group.className = 'input-group input-group-sm credential-secret mt-2';
            const input = document.createElement('input'); input.className = 'form-control font-monospace'; input.readOnly = true; input.value = token; input.setAttribute('aria-label', `${item.caller_id} 密钥`);
            const copy = document.createElement('button'); copy.type = 'button'; copy.className = 'btn btn-outline-secondary'; copy.textContent = '复制';
            copy.addEventListener('click', async () => {
              try { await navigator.clipboard.writeText(input.value); message.className = 'small mt-2 text-success'; message.textContent = `已复制 ${item.caller_id} 的密钥。`; }
              catch { input.select(); document.execCommand('copy'); message.textContent = '已复制密钥。'; }
            });
            group.append(input, copy); actions.append(group); revealButton.textContent = '收起密钥'; revealedAt.textContent = date(Date.now());
          } catch (error) { alert(error.message); }
          finally { revealButton.disabled = false; }
        });
        actions.append(revealButton);
        if (!item.revoked_at) {
          const revoke = document.createElement('button'); revoke.type = 'button'; revoke.className = 'btn btn-sm btn-outline-danger ms-1'; revoke.textContent = '撤销';
          revoke.addEventListener('click', async () => {
            if (!confirm(`撤销 ${item.caller_id} 的这把密钥？使用它的客户端将立即无法调用批量 API。`)) return;
            try { await api.revokeBatchCredential(item.id); await refresh(); } catch (error) { alert(error.message); }
          });
          actions.append(revoke);
        }
        tr.append(actions); rows.append(tr);
      });
    } catch (error) {
      rows.replaceChildren(); const failed = cell(error.message); failed.colSpan = 8; failed.className = 'table-loading text-danger';
      const failedRow = document.createElement('tr'); failedRow.append(failed); rows.append(failedRow);
    }
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
    try { await navigator.clipboard.writeText(input.value); message.textContent = '密钥已复制。'; }
    catch { document.execCommand('copy'); message.textContent = '密钥已复制。'; }
  });
  document.getElementById('refreshCredentials').addEventListener('click', refresh);
  refresh();
})();
