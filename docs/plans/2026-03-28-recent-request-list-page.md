# Recent Request List Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 `nominatim-cache` 后台增加一个独立的“最近请求清单”页面，显示最近 100 条访问记录，并在页面打开时自动刷新。

**Architecture:** 复用现有 `/admin/logs` 接口，不新增后端协议；前端新增单独 HTML + JS 页面，通过每 1 秒拉取一次最新 100 条日志来更新表格。为了减少无意义请求，在页面不可见时暂停轮询，页面恢复可见时立即刷新。

**Tech Stack:** Express, SQLite, Vanilla JS, 静态前端页面

---

### Task 1: 确认日志接口和列表展示格式

**Files:**
- Modify: `frontend/index.html`
- Create: `frontend/requests.html`
- Create: `frontend/js/request-list.js`
- Modify: `frontend/js/api.js`

**Step 1: 写失败验证**

确认当前没有独立的最近请求页面，也没有对应 API 封装函数。

**Step 2: 写最小实现**

- 新增 `api.getRecentLogs({ limit: 100 })`
- 新增 `requests.html`
- 新增对应脚本渲染最近 100 条请求

**Step 3: 运行验证**

Run: 启动服务后访问 `/requests.html`
Expected: 页面能打开并显示请求列表。

### Task 2: 实现自动刷新与暂停机制

**Files:**
- Modify: `frontend/js/request-list.js`

**Step 1: 写失败验证**

确认当前页面不会自动刷新。

**Step 2: 写最小实现**

- 页面可见时每 1 秒刷新一次
- 页面隐藏时暂停刷新
- 页面恢复可见时立即刷新一次

**Step 3: 运行验证**

Run: 打开 `/requests.html`
Expected: 新请求会在 1 秒内出现在页面中。

### Task 3: 更新入口链接

**Files:**
- Modify: `frontend/index.html`

**Step 1: 写最小实现**

在 Dashboard 中新增“最近请求”入口按钮。

**Step 2: 运行验证**

Run: 打开首页 `/`
Expected: 可以点击跳转到 `/requests.html`。

### Task 4: 最终验证

**Files:**
- Verify only

**Step 1: 手动验证**

- 打开 `/requests.html`
- 触发几次 `/api/reverse`
- 确认最近请求列表自动更新

**Step 2: 构建/运行验证**

Run: 启动服务并访问页面
Expected: 页面可用，无控制台错误。
