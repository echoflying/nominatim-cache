# Mac mini + Tailscale 部署说明

本文档记录如何将 `nominatim-cache` 部署到一台加入 Tailscale 网络的 Mac mini，并通过 Tailscale 内网地址访问。

## 适用场景

- 家庭或办公室内网节点
- 不打算公网暴露服务
- 通过 Tailscale IP 或 MagicDNS 访问服务

## 前置条件

- Mac mini 已安装并登录 Tailscale
- 项目代码位于 `~/nominatim-cache`
- Node.js 与 npm 可用
- 已将数据库迁移到 `~/nominatim-cache/server/data/cache.db`

## 首次部署

### 1. 拉取代码并安装依赖

```bash
cd ~/nominatim-cache/server
npm install
```

### 2. 准备配置

在仓库根目录创建 `.env`：

```bash
cd ~/nominatim-cache
cp .env.example .env
```

建议至少设置：

```env
PORT=3000
NODE_ENV=development
DATABASE_PATH=./data/cache.db
ADMIN_USERNAME=your-admin
ADMIN_PASSWORD=strong-password
REQUEST_INTERVAL=1500
```

说明：

- 若仅通过 Tailscale 内网访问，当前保留 HTTP 即可
- 如切到公网或反代 HTTPS，请再评估更严格的安全头配置

### 3. 构建并初始化数据库

```bash
cd ~/nominatim-cache/server
npm run build
npm run db:init
```

### 4. 使用 PM2 托管

```bash
cd ~/nominatim-cache/server
~/.npm-global/bin/pm2 start dist/index.js --name nominatim-cache
~/.npm-global/bin/pm2 save
```

## 日常更新

```bash
cd ~/nominatim-cache
git pull
cd server
npm install
npm run build
~/.npm-global/bin/pm2 restart nominatim-cache
```

## 验证命令

### 本机验证

```bash
curl http://127.0.0.1:3000/health
```

### Tailscale 验证

```bash
curl http://100.93.208.107:3000/health
```

浏览器访问：

- `http://100.93.208.107:3000/`
- `http://100.93.208.107:3000/list.html`

## PM2 常用命令

```bash
~/.npm-global/bin/pm2 status
~/.npm-global/bin/pm2 logs nominatim-cache
~/.npm-global/bin/pm2 restart nominatim-cache
~/.npm-global/bin/pm2 stop nominatim-cache
~/.npm-global/bin/pm2 delete nominatim-cache
```

## 开机自启

建议在 Mac mini 上执行：

```bash
~/.npm-global/bin/pm2 startup
~/.npm-global/bin/pm2 save
```

`pm2 startup` 会输出一条需要 `sudo` 执行的命令，复制执行即可。

## 数据迁移

Ubuntu 旧库迁移到 Mac mini 时，至少复制：

- `server/data/cache.db`

如存在以下文件，也一起复制：

- `server/data/cache.db-wal`
- `server/data/cache.db-shm`

## 当前方案与其他部署方式的区别

- 本方案面向 Tailscale 私网访问，不依赖 Nginx 或 Cloudflare
- 与 `docs/DEPLOYMENT.md` 中的 Docker / PM2 + Nginx 方案并列
- 机器本地状态（PM2 当前进程、日志、用户路径）不纳入 git；但部署步骤、文档和配置文件应纳入 git
