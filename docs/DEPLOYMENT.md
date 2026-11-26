# 部署说明

本文档描述 `nominatim-cache` 当前支持的两种部署方式：`Docker Compose` 和 `PM2 + Nginx`。

如果部署目标是一台通过 Tailscale 访问的 Mac mini，请直接参考 `docs/deploy/macmini-tailscale.md`。

Go + MariaDB 重写及向 Raven 迁移的后续方案见 [Nominatim Cache v2 重写与 Raven 迁移计划](deploy/nominatim-cache-v2-raven.md)。
Raven 首次部署所用的实际端口、数据库和切换步骤见 [Raven 部署执行单](deploy/raven-cutover.md)。

## Go v2 本地运行与发布

从 `backend/` 使用 Go 1.26 构建。使用专用 MariaDB 库，按 `deploy/raven/backend.env.example` 设置进程环境。运行前用具有建表权限的 `MIGRATE_DSN` 执行 `go run ./cmd/nc-migrate schema`。然后使用应用账号的 `MARIADB_DSN` 执行 `go run ./cmd/nominatim-cache`。导入使用 `go run ./cmd/nc-migrate sqlite -snapshot /path/to/cache.db`，可加 `-dry-run` 仅检查来源。导入目标必须为空，重复导入相同快照只返回原回执。运行与迁移的连接信息都从进程环境读取。

公开 API 默认监听 `127.0.0.1:3000`，路径前缀用 `BASE_PATH` 指定。管理页面和管理 API 默认监听 `127.0.0.1:3001`，用 `ADMIN_LISTEN_ADDR` 配置独立回环地址，用 `FRONTEND_DIR` 指定静态资源目录。生产用独立的私有 HTTPS 管理入口，并设置非默认管理账号与密码。管理凭据只保留在当前页面内存，旧存储键会清理，首次发布时轮换旧凭据。

Raven 使用 `BASE_PATH=/nominatim`。在 Nginx `http` 块引入 `deploy/raven/nominatim-rate.conf`，在 `x.zaitu.cn` 的 HTTPS `server` 块引入 `deploy/raven/nominatim-location.conf`。运行 `nginx -t` 再重载。公网接入 `/reverse`、`/health`、`/help` 及限流的 `/altview/place-batches` 路由，路径均带 `/nominatim` 前缀；通用批量任务路由仍不对公网开放。

Raven 的管理入口为 `https://x.zaitu.cn/ops/nominatim/`。OPS 注册系统 `nominatim` 与权限 `nominatim.access`，首次授权现有超级管理员。在同一 HTTPS `server` 引入 `deploy/raven/nominatim-ops-location.conf`。用 root 运行 `python3 deploy/raven/render-ops-upstream-auth.py` 生成仅 root 可读的 Nginx 认证片段。Nominatim 的管理账号或密码轮换后重新生成片段，再校验和重载 Nginx。验证登录前跳转、未授权账号 403、登录后统计及缓存列表，并检查跨站写请求被拒绝。

会话过期时，概览、缓存清单和最近请求页面都跳转到 `/ops/`。管理 API 保留 HTTP 401，并以 UTF-8 JSON 提示重新登录。

生产必须通过 `NOMINATIM_PRIMARY`、`NOMINATIM_BACKUP_1`、`NOMINATIM_BACKUP_2` 至少配置一个交互上游。`REVERSE_COLD_LIMIT` 默认允许 16 个未命中的同步请求同时进入查询流程。`LEGACY_HOST` 默认是 `nc.zaitu.cn`，`LEGACY_TIMEOUT_MS` 默认 6500，范围 1000 至 7000 毫秒。两项仅在旧域名转给 Go 时使用。发布验收检查接口指南、健康、冷查询、缓存命中、CORS 及公开入口对批量和管理路由的隔离。

用 `BATCH_TOKENS` 配置具有独立身份的试用调用方，格式为 `caller=token,other=token`。另用 `NOMINATIM_BATCH_URL` 配置获准批量使用的自建或签约源。缺省只允许批量命中缓存。公共上游的用量及政策见 [批量设计](deploy/nominatim-cache-v2-raven.md#3-批量接口与队列)。

新增 SQL 若已确认可与现有程序共用，文件名用 `003_compat002_description.sql` 表示最低兼容版本 002。没有 `compat` 标记的新增迁移要求对应新版程序。旧程序始终校验自己认识的迁移内容，并在新迁移执行期间停止启动。将兼容性声明纳入迁移演练，包含一次旧二进制的启动验证。

回退到 Node 时，先暂停接收写入并停止 Go 服务，再用 `nc-migrate export-sqlite -output /path/to/new-cache.db` 从 `MIGRATE_DSN` 指定的 MariaDB 导出新 SQLite。输出路径必须不存在。命令校验三表的内容摘要和 SQLite 完整性，再生成文件和回执。保留迁移前 SQLite 与 MariaDB 的备份，用导出的文件启动旧程序做验证。批量任务只保留在 MariaDB，切回 Node 前暂停批量提交。

在 Linux/WSL 的仓库根目录运行 `bash scripts/build-raven.sh v2.0.0-rc.1` 生成 Linux amd64 包及校验值。先按样例配置 systemd 和 Nginx，再切换流量。以下两种部署方式适用于 Node 版。

## 部署前要求

- Linux 服务器
- Node.js 20+（仅 PM2 方案需要）
- Docker / Docker Compose（仅 Docker 方案需要）
- 服务器可以访问上游 Nominatim 服务

## 环境变量

服务会按以下顺序查找 dotenv 文件：

1. 仓库根目录 `.env`
2. `server/.env`

推荐统一使用仓库根目录 `.env` 作为部署入口。系统环境变量优先级最高，会覆盖 dotenv 文件中的同名值。

生产环境必须显式设置以下变量，且不得使用默认值：

```env
NODE_ENV=production
ADMIN_USERNAME=your-admin
ADMIN_PASSWORD=strong-password
```

推荐完整配置：

```env
PORT=3000
NODE_ENV=production
DATABASE_PATH=./data/cache.db
ADMIN_USERNAME=your-admin
ADMIN_PASSWORD=strong-password
NOMINATIM_PRIMARY=https://api.mirror-earth.com/nominatim/reverse
NOMINATIM_BACKUP_1=https://photon.komoot.io/reverse
NOMINATIM_BACKUP_2=https://nominatim.openstreetmap.org/reverse
LOG_RETENTION_DAYS=30
REQUEST_INTERVAL=1500
```

## 方式一：Docker Compose

### 1. 准备配置

在仓库根目录创建 `.env`：

```bash
cp .env.example .env
```

然后修改其中的生产凭证。

### 2. 启动服务

```bash
docker compose up -d --build
```

### 3. 检查状态

```bash
docker compose ps
curl http://127.0.0.1:3000/health
docker compose logs -f app
```

### 4. 数据与静态文件

- 数据库目录挂载为 `./server/data`
- 前端静态文件打包进镜像，同时保留本地 `./frontend` 挂载，便于热替换页面

## 方式二：PM2 + Nginx

### 1. 安装依赖并构建

```bash
cd server
npm ci
npm run build
```

### 2. 初始化数据库

确保已经设置好环境变量后执行：

```bash
npm run db:init
```

### 3. 启动 PM2

```bash
cp .env /opt/nominatim-cache/.env
export NOMINATIM_CACHE_SERVER_CWD=/opt/nominatim-cache/server
pm2 start ecosystem.config.cjs
pm2 save
```

说明：

- `ecosystem.config.js` 只固定 `NODE_ENV=production`
- 端口、数据库路径、管理员账号密码等都建议放在仓库根目录 `.env`
- 这样 Docker、手工部署、PM2 三种方式共用一套配置入口

日志会写入 `server/logs/`。

### 4. Nginx 反代

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 健康检查

服务提供独立健康检查接口：

```bash
curl http://127.0.0.1:3000/health
```

返回示例：

```json
{
  "status": "ok",
  "service": "nominatim-cache",
  "environment": "production",
  "timestamp": 1711111111111,
  "uptime": 42
}
```

该接口不依赖外部上游服务，适合用于容器或负载均衡健康探针。

## 管理后台登录

- 浏览器首次访问管理接口时会提示输入账号密码
- 不再内置默认 `admin/admin123` 自动登录行为
- 若修改密码，清理浏览器 `localStorage` 中的 `nominatim_auth` 即可重新输入

## 备份与恢复

```bash
# 备份
cp server/data/cache.db backup/cache_$(date +%Y%m%d).db

# 恢复
cp backup/cache_20260328.db server/data/cache.db
```

如果数据库处于活跃写入状态，建议先停服务再做冷备份。

## 升级建议

1. 先备份数据库
2. 拉取新代码
3. 执行 `npm run build`
4. 如有结构变更，执行 `npm run db:migrate`
5. 重启 PM2 或重新部署 Compose
