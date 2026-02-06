# Nominatim 缓存服务 - 设计文档

## 一、项目概述

### 1.1 项目背景

PhotoOrdo 是一个本地优先的照片整理工具，需要频繁调用 Nominatim API 进行逆地理编码（GPS 坐标转地名）。Nominatim 官方限制 1 次/秒的请求频率，且国内访问不稳定。为减少上游 API 调用次数、提升响应速度、规避访问限制，需要搭建一个本地缓存服务。

### 1.2 项目目标

- 缓存 Nominatim 逆地理编码结果，实现永久缓存
- 提供简洁的管理界面，查看缓存状态和统计信息
- 支持多上游 Nominatim 镜像源，提高可用性
- 支持 Basic Auth 认证，保护管理功能

### 1.3 项目位置

```
/mnt/d/home.it/nominatim-cache/  (独立项目)
```

---

## 二、需求说明

### 2.1 功能需求

| 编号 | 功能 | 优先级 | 说明 |
|------|------|--------|------|
| F01 | Nominatim 代理接口 | P0 | 接口兼容官方，透明缓存 |
| F02 | 多上游源调用 | P0 | 按优先级调用不同 Nominatim 镜像 |
| F03 | 缓存永久有效 | P0 | 缓存永不过期 |
| F04 | 访问统计 | P1 | 总请求数、命中数、命中率 |
| F05 | 缓存清单 | P1 | 显示所有缓存条目及元信息 |
| F06 | 管理认证 | P1 | Basic Auth (admin/admin123) |
| F07 | 访问日志 | P2 | 保留 30 天 |

### 2.2 非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| N01 | 性能 | 单次请求响应 < 200ms |
| N02 | 可靠性 | 上游故障时自动切换备选源 |
| N03 | 存储 | SQLite 文件存储，易于备份迁移 |
| N04 | 部署 | 支持 Docker 部署和本地运行 |

### 2.3 第二期规划

| 编号 | 功能 | 说明 |
|------|------|------|
| P01 | 地图显示 | 在地图上显示所有缓存点位置 |
| P02 | 高德/百度集成 | 使用国产地图 API 作为补充 |

---

## 三、架构设计

### 3.1 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      Nominatim Cache Service                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                    控制页面 (HTML/CSS/JS)               │   │
│   │   - Dashboard: 统计仪表盘                              │   │
│   │   - List: 缓存清单页面                                 │   │
│   └─────────────────────────────────────────────────────────┘   │
│                              ↓                                   │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  Node.js API Server                     │   │
│   │   - Express.js 框架                                    │   │
│   │   - Basic Auth 中间件                                  │   │
│   │   - SQLite 数据库访问层                                │   │
│   └─────────────────────────────────────────────────────────┘   │
│                              ↓                                   │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                      SQLite 数据库                       │   │
│   │   - cache_entries: 缓存条目表                           │   │
│   │   - stats: 统计表                                      │   │
│   │   - access_logs: 访问日志表 (30天滚动)                 │   │
│   └─────────────────────────────────────────────────────────┘   │
│                              ↓                                   │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  上游 Nominatim 服务                     │   │
│   │   1. 镜像地球代理 (mirror-earth.com)                   │   │
│   │   2. Photon (komoot.io)                                │   │
│   │   3. OSM 官方 (openstreetmap.org)                      │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 目录结构

```
nominatim-cache/
├── server/                          # 后端服务
│   ├── src/
│   │   ├── index.ts                 # 主入口
│   │   ├── routes/
│   │   │   ├── nominatim.ts        # Nominatim 代理接口
│   │   │   ├── admin.ts           # 管理接口
│   │   │   └── stats.ts           # 统计接口
│   │   ├── services/
│   │   │   ├── database.ts        # SQLite 数据库服务
│   │   │   ├── cache.ts           # 缓存业务逻辑
│   │   │   ├── nominatim.ts       # 上游 API 调用
│   │   │   └── stats.ts           # 统计服务
│   │   ├── middleware/
│   │   │   ├── auth.ts            # Basic Auth 中间件
│   │   │   └── logger.ts          # 访问日志中间件
│   │   └── utils/
│   │       └── geo.ts              # 坐标工具函数
│   ├── data/                       # SQLite 数据库目录
│   ├── package.json
│   └── tsconfig.json
│
├── frontend/                        # 前端页面
│   ├── index.html                  # Dashboard 页面
│   ├── list.html                   # 缓存清单页面
│   ├── css/
│   │   └── style.css              # 公共样式
│   └── js/
│       ├── api.js                  # API 客户端封装
│       ├── dashboard.js            # Dashboard 逻辑
│       └── cache-list.js           # 缓存列表逻辑
│
├── docs/                           # 文档
│   ├── DESIGN.md                   # 本设计文档
│   └── API.md                      # API 接口文档
│
├── docker-compose.yml              # Docker 编排配置
│
├── .env.example                    # 环境变量示例
│
└── README.md                       # 项目说明
```

---

## 四、数据库设计

### 4.1 数据库选择

使用 SQLite 作为数据存储，主要考虑：
- 无需独立数据库服务
- 文件级存储，易于备份和迁移
- 单文件设计，适合中小规模应用
- 未来可平滑迁移到 PostgreSQL

### 4.2 表结构

#### 4.2.1 cache_entries (缓存条目表)

```sql
CREATE TABLE cache_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cache_key TEXT UNIQUE NOT NULL,      -- 格式: "lat_lon" (保留4位小数)
    lat REAL NOT NULL,                    -- 纬度
    lon REAL NOT NULL,                   -- 经度
    display_name TEXT,                   -- 显示名称
    place_type TEXT,                     -- 地点类型: temple, peak, city, street, other
    nominatim_response TEXT NOT NULL,    -- 完整的 Nominatim JSON 响应
    first_cached_at INTEGER NOT NULL,    -- 首次缓存时间 (Unix timestamp)
    last_accessed_at INTEGER NOT NULL,   -- 最后访问时间 (Unix timestamp)
    access_count INTEGER DEFAULT 1      -- 访问次数
);

CREATE INDEX idx_cache_key ON cache_entries(cache_key);
CREATE INDEX idx_lat_lon ON cache_entries(lat, lon);
CREATE INDEX idx_first_cached ON cache_entries(first_cached_at DESC);
```

#### 4.2.2 stats (统计表)

```sql
CREATE TABLE stats (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    total_requests INTEGER DEFAULT 0,    -- 总请求数
    cache_hits INTEGER DEFAULT 0,       -- 缓存命中次数
    cache_misses INTEGER DEFAULT 0,     -- 缓存未命中次数
    last_updated_at INTEGER NOT NULL    -- 最后更新时间
);
```

#### 4.2.3 access_logs (访问日志表)

```sql
CREATE TABLE access_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cache_key TEXT,                     -- 关联的缓存键
    lat REAL,                          -- 请求纬度
    lon REAL,                          -- 请求经度
    cached INTEGER DEFAULT 0,           -- 是否命中缓存: 0=no, 1=yes
    accessed_at INTEGER NOT NULL        -- 访问时间
);

CREATE INDEX idx_accessed_at ON access_logs(accessed_at DESC);
CREATE INDEX idx_cache_key_logs ON access_logs(cache_key);
```

### 4.3 数据示例

**cache_entries:**

| cache_key | lat | lon | display_name | place_type | access_count |
|-----------|-----|-----|--------------|------------|--------------|
| 30.2875_120.1615 | 30.2875 | 120.1615 | 灵隐寺, 灵隐路... | temple | 156 |
| 31.2304_121.4737 | 31.2304 | 121.4737 | 外滩, 中山东路... | street | 89 |

---

## 五、API 接口设计

### 5.1 接口概览

| 接口 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/api/reverse` | GET | ❌ | Nominatim 逆地理编码代理 |
| `/admin/stats` | GET | ✅ | 获取统计信息 |
| `/admin/cache/list` | GET | ✅ | 获取缓存列表（分页） |
| `/admin/cache/:key` | GET | ✅ | 获取单个缓存详情 |
| `/admin/cache/:key` | DELETE | ✅ | 删除单个缓存 |
| `/admin/cache/clear` | POST | ✅ | 清空所有缓存 |
| `/admin/logs` | GET | ✅ | 获取访问日志 |

### 5.2 接口详情

#### 5.2.1 Nominatim 逆地理编码代理

**请求:**
```
GET /api/reverse?lat=30.28746&lon=120.16145&format=jsonv2&addressdetails=1
```

**参数:**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| lat | float | ✅ | 纬度 |
| lon | float | ✅ | 经度 |
| format | string | ❌ | 响应格式 (默认: jsonv2) |
| addressdetails | int | ❌ | 是否返回地址详情 (默认: 1) |
| extratags | int | ❌ | 是否返回扩展标签 (默认: 1) |
| accept-language | string | ❌ | 语言 (默认: zh-CN) |
| zoom | int | ❌ | 缩放级别 (默认: 18) |

**响应 (兼容 Nominatim 官方):**
```json
{
  "lat": "30.2874600",
  "lon": "120.1614500",
  "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
  "address": {
    "tourism": "temple",
    "road": "灵隐路",
    "city": "杭州市"
  }
}
```

> **说明:** 接口完全兼容 Nominatim 官方格式，缓存状态对调用方透明。

#### 5.2.2 获取统计信息

**请求:**
```
GET /admin/stats
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**响应:**
```json
{
  "total_requests": 12583,
  "cache_hits": 11920,
  "cache_misses": 663,
  "hit_rate": 94.7,
  "cache_count": 1247,
  "last_updated_at": 1704153600
}
```

#### 5.2.3 获取缓存列表

**请求:**
```
GET /admin/cache/list?page=1&limit=20&search=灵隐
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**参数:**

| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码 (默认: 1) |
| limit | int | 每页数量 (默认: 20, 最大: 100) |
| search | string | 搜索关键词 |

**响应:**
```json
{
  "data": [
    {
      "cache_key": "30.2875_120.1615",
      "lat": 30.2875,
      "lon": 120.1615,
      "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市...",
      "place_type": "temple",
      "first_cached_at": 1704067200,
      "last_accessed_at": 1704153600,
      "access_count": 156
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 1247,
    "total_pages": 63
  }
}
```

#### 5.2.4 清空缓存

**请求:**
```
POST /admin/cache/clear
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**响应:**
```json
{
  "success": true,
  "message": "所有缓存已清空",
  "deleted_count": 1247
}
```

---

## 六、上游 Nominatim 源

### 6.1 优先级列表

| 优先级 | URL | 说明 |
|--------|-----|------|
| 1 | https://api.mirror-earth.com/nominatim/reverse | 镜像地球代理（首选，国内稳定） |
| 2 | https://photon.komoot.io/reverse | Photon（备选，支持 CORS） |
| 3 | https://nominatim.openstreetmap.org/reverse | OSM 官方（最后兜底） |

### 6.2 故障切换策略

- 按优先级依次尝试调用
- 任意一个源返回有效响应即返回
- 所有源都失败时返回 500 错误
- 检测上游返回 HTML 页面时自动降级（详见 10.3 节）

---

## 七、部署指南

### 7.1 开发环境（Ubuntu 本地）

```bash
# 1. 进入项目目录
cd nominatim-cache

# 2. 安装依赖
cd server && npm install

# 3. 初始化数据库
npm run db:init

# 4. 启动开发服务器
npm run dev

# 5. 访问控制页面
# Dashboard: http://localhost:3000
# 缓存列表: http://localhost:3000/list.html
```

### 7.2 Docker 部署

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

### 7.3 生产环境（Linux 服务器）

```bash
# 1. 安装 Node.js (v18+)
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt install nodejs

# 2. 安装 PM2
sudo npm install -g pm2

# 3. 上传代码并安装依赖
cd server && npm install --production

# 4. 初始化数据库
npm run db:init

# 5. 启动服务
pm2 start ecosystem.config.js

# 6. 配置开机自启
pm2 startup
pm2 save

# 7. 配置 Nginx (参考 nginx.conf)
```

### 7.4 环境变量

```bash
# .env
PORT=3000
NODE_ENV=development

# 数据库
DATABASE_PATH=./data/cache.db

# Basic Auth
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123

# Nominatim 上游
NOMINATIM_PRIMARY=https://api.mirror-earth.com/nominatim/reverse
NOMINATIM_BACKUP_1=https://photon.komoot.io/reverse
NOMINATIM_BACKUP_2=https://nominatim.openstreetmap.org/reverse

# 日志保留天数
LOG_RETENTION_DAYS=30

# 请求间隔 (ms)
REQUEST_INTERVAL=1000
```

---

## 八、监控和维护

### 8.1 健康检查

```bash
# 检查服务状态
curl http://localhost:3000/admin/stats

# 检查数据库
sqlite3 data/cache.db "SELECT COUNT(*) FROM cache_entries;"
```

### 8.2 日志位置

| 环境 | 日志位置 |
|------|----------|
| 本地开发 | 控制台输出 |
| Docker | `docker-compose logs app` |
| PM2 | `pm2 logs` |

### 8.3 备份恢复

```bash
# 备份
cp data/cache.db backup/cache_$(date +%Y%m%d).db

# 恢复
cp backup/cache_20240115.db data/cache.db
```

---

## 十、OSM Nominatim 使用规范

### 10.1 API 使用规则

根据 [OSM Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/):

| 要求 | 说明 | 当前实现 |
|------|------|---------|
| User-Agent | 必须包含应用名称和联系方式 | `PhotoOrdo-Local/1.0 (echoflying@gmail.com)` |
| Referer | 必须设置来源 URL | `http://localhost:3000/` |
| 请求频率 | 不超过 1 次/秒 | 前端限制 1.5 秒间隔 |
| 批量请求 | 禁止大规模批量请求 | 缓存机制减少上游调用 |

### 10.2 请求头配置

```typescript
// Nominatim API 请求头
headers: {
  'User-Agent': 'PhotoOrdo-Local/1.0 (echoflying@gmail.com)',
  'Referer': 'http://localhost:3000/',
  'Accept-Language': 'zh-CN,zh'
}
```

### 10.3 上游源健康检查

#### 10.3.1 检测机制

- 检测上游返回 HTML 页面（返回内容以 `<!DOCTYPE` 或 `<html` 开头）
- 检测 HTTP 错误状态码（429, 403, 503 等）

#### 10.3.2 动态优先级

- 被 Block 的源自动降级到优先级列表末尾
- 每 60 分钟重试已降级的源
- 成功响应后恢复优先级

#### 10.3.3 降级示例

```
初始优先级:
1. mirror-earth.com (blocked) → 降级到最后
2. photon.komoot.io
3. openstreetmap.org

下次请求优先级:
1. photon.komoot.io
2. openstreetmap.org
3. mirror-earth.com (稍后重试)
```

### 10.4 上游 API 限速

OSM Nominatim 官方限制 1 次/秒，服务端实现 **1.5 秒间隔** 限制：

```typescript
// 服务端上游请求间隔 1.5 秒
const UPSTREAM_INTERVAL = 1500;
let lastUpstreamCall = 0;

async function waitUpstreamInterval() {
  const now = Date.now();
  const elapsed = now - lastUpstreamCall;
  if (elapsed < UPSTREAM_INTERVAL) {
    await sleep(UPSTREAM_INTERVAL - elapsed);
  }
  lastUpstreamCall = Date.now();
}
```

> **说明:** 
> - 外部调用 Nominatim Cache **不限速**
> - 只有向上游 Nominatim API 发起请求时才限速
> - 缓存命中时直接返回，不触发上游调用

### 10.5 合规性检查清单

- [x] 自定义 User-Agent 包含联系方式
- [x] 设置 Referer 请求头
- [x] 上游 API 调用间隔 ≥ 1.5 秒
- [x] 缓存减少上游调用次数
- [x] 上游源健康检测
- [x] 被 Block 时自动降级

---

## 十一、附录

### 11.1 相关链接

- [OSM Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)
- [Nominatim 官方文档](https://nominatim.org/release-docs/latest/)
- [OSM 服务条款](https://www.openstreetmap.org/copyright)

### 11.2 更新日志

| 日期 | 版本 | 说明 |
|------|------|------|
| 2024-02-06 | 1.0 | 初始版本，完成第一期设计 |
| 2026-02-06 | 1.1 | 添加 OSM 合规性、HTTP 请求头、动态上游优先级 |
