(() => {
  // Tabler Icons are MIT licensed: https://github.com/tabler/tabler-icons
  const icons = {
    'layout-dashboard': '<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>',
    database: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14a9 3 0 0 0 18 0V5"/><path d="M3 12a9 3 0 0 0 18 0"/>',
    'list-details': '<path d="M13 5h8M13 9h5M13 15h8M13 19h5"/><path d="M3 4m0 1a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1zM3 14m0 1a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/>',
    activity: '<path d="M3 12h4l3-9 4 18 3-9h4"/>',
    'circle-check': '<path d="M11 3.055A9 9 0 1 0 20.945 13"/><path d="m9 11 3 3L22 4"/>',
    'chart-bar': '<path d="M3 3v18h18"/><path d="M18 17V9M13 17V5M8 17v-3"/>',
    'arrow-right': '<path d="M5 12h14M13 6l6 6-6 6"/>',
    refresh: '<path d="M20 11a8.1 8.1 0 0 0-15.5-2M4 4v5h5M4 13a8.1 8.1 0 0 0 15.5 2M20 20v-5h-5"/>',
    trash: '<path d="M4 7h16M10 11v6M14 11v6M5 7l1 13h12l1-13M9 7V4h6v3"/>',
    search: '<circle cx="10" cy="10" r="7"/><path d="m21 21-6-6"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 11h18"/>',
    'chevron-left': '<path d="m15 6-6 6 6 6"/>',
    'chevron-right': '<path d="m9 6 6 6-6 6"/>',
    x: '<path d="m18 6-12 12M6 6l12 12"/>',
    'database-plus': '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3M4 11v6c0 1.7 3.6 3 8 3M20 11v2M17 5v5"/><path d="M16 17h6M19 14v6"/>',
    key: '<circle cx="8" cy="15" r="5"/><path d="m21 2-9.6 9.6M15.5 7.5l3 3L22 7l-3-3"/>',
    'menu-2': '<path d="M4 6h16M4 12h16M4 18h16"/>'
  };
  const icon = name => `<svg class="icon" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icons[name] || ''}</svg>`;
  window.ncIcon = icon;
  const renderIcons = root => {
    if (root.matches?.('[data-icon]')) root.innerHTML = icon(root.dataset.icon);
    root.querySelectorAll?.('[data-icon]').forEach(node => { node.innerHTML = icon(node.dataset.icon); });
  };
  renderIcons(document);

  const content = document.querySelector('.admin-content');
  if (!content) return;
  const current = location.pathname.endsWith('/list.html') ? 'cache' : location.pathname.endsWith('/requests.html') ? 'requests' : location.pathname.endsWith('/credentials.html') ? 'credentials' : 'overview';
  const link = (key, href, label, iconName) => `<a class="nav-link${current === key ? ' active' : ''}" href="${href}"${current === key ? ' aria-current="page"' : ''}><span class="nav-link-icon">${icon(iconName)}</span><span>${label}</span></a>`;
  const shell = document.createElement('div');
  shell.className = 'admin-shell';
  shell.innerHTML = `<aside class="admin-sidebar" id="adminSidebar"><a class="brand-link" href="./"><span class="brand-mark">NC</span><span><strong>Nominatim Cache</strong><small>地点服务管理</small></span></a><div class="nav-section">服务管理</div><nav class="admin-nav" aria-label="管理导航">${link('overview','./','服务概览','layout-dashboard')}${link('cache','list.html','缓存清单','database')}${link('requests','requests.html','最近请求','list-details')}${link('credentials','credentials.html','调用方凭据','key')}</nav><div class="sidebar-footer"><span class="sidebar-status"></span>地点缓存服务</div></aside><button class="sidebar-scrim" type="button" aria-label="关闭菜单"></button><div class="admin-main"><header class="admin-topbar"><button class="btn btn-icon btn-ghost-secondary mobile-menu" type="button" aria-label="打开导航">${icon('menu-2')}</button><div class="topbar-path"><span>运维中心</span><span class="path-divider">/</span><strong>地点服务</strong></div><span class="topbar-badge">NOMINATIM CACHE</span></header><div class="admin-page-body"></div></div>`;
  shell.querySelector('.admin-page-body').append(content);
  document.body.prepend(shell);
  new MutationObserver(records => records.forEach(record => record.addedNodes.forEach(node => {
    if (node.nodeType === Node.ELEMENT_NODE) renderIcons(node);
  }))).observe(content, { childList: true, subtree: true });
  const toggle = open => shell.classList.toggle('menu-open', open);
  shell.querySelector('.mobile-menu').addEventListener('click', () => toggle(!shell.classList.contains('menu-open')));
  shell.querySelector('.sidebar-scrim').addEventListener('click', () => toggle(false));
  shell.querySelectorAll('.admin-nav .nav-link').forEach(node => node.addEventListener('click', () => toggle(false)));
})();
