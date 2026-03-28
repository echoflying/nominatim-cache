# 部署说明

本文档描述 `nominatim-cache` 当前支持的两种部署方式：`Docker Compose` 和 `PM2 + Nginx`。

如果部署目标是一台通过 Tailscale 访问的 Mac mini，请直接参考 `docs/deploy/macmini-tailscale.md`。

## 部署前要求

- Linux 服务器
- Node.js 20+（仅 PM2 方案需要）
- Docker / Docker Compose（仅 Docker 方案需要）
- 服务器可以访问上游 Nominatim 服务

## 环境变量

服务会按以下顺序查找 dotenv 文件：

1. `server/.env`
2. 仓库根目录 `.env`

系统环境变量优先级最高，会覆盖 dotenv 文件中的同名值。

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
pm2 start ecosystem.config.js
pm2 save
```

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
