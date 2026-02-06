# Nominatim Cache Service

一个自托管的 Nominatim 逆地理编码缓存服务，提供透明缓存、访问统计和管理界面。


## 参考文档
- 基本需求和设计：DESIGN.md
- 外部接口和管理接口：API.md
- 外部如何调用API的说明：API_nominatim-cache.md


## 功能特性

- **透明缓存**: 接口兼容官方 Nominatim API，缓存永久有效
- **多上游源**: 支持多个 Nominatim 镜像源，自动故障切换
- **访问统计**: 实时统计请求数、命中率等指标
- **管理界面**: 简洁的 Dashboard 和缓存清单页面
- **Basic Auth**: 保护管理功能
- **访问日志**: 保留 30 天访问记录
- **SQLite 存储**: 轻量级数据库，易于备份迁移

## 快速开始

### 环境要求

- Node.js 18+
- npm 或 yarn

### 安装运行

```bash
# 1. 进入 server 目录
cd server

# 2. 安装依赖
npm install

# 3. 初始化数据库
npm run db:init

# 4. 启动开发服务器
npm run dev

# 5. 访问页面
# Dashboard: http://localhost:3000
# 缓存列表: http://localhost:3000/list.html
```

### Docker 部署

```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

## 使用方法

### 逆地理编码调用

```bash
# 基本调用
curl "http://localhost:3000/api/reverse?lat=30.28746&lon=120.16145"

# 完整参数
curl "http://localhost:3000/api/reverse?lat=30.28746&lon=120.16145&format=jsonv2&addressdetails=1"
```

### 管理功能

```bash
# 查看统计 (admin/admin123)
curl -u admin:admin123 http://localhost:3000/admin/stats

# 获取缓存列表
curl -u admin:admin123 "http://localhost:3000/admin/cache/list?page=1&limit=20"

# 清空缓存
curl -u admin:admin123 -X POST http://localhost:3000/admin/cache/clear
```

## 配置说明

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| PORT | 3000 | 服务端口 |
| DATABASE_PATH | ./data/cache.db | 数据库路径 |
| ADMIN_USERNAME | admin | 管理用户名 |
| ADMIN_PASSWORD | admin123 | 管理密码 |
| NOMINATIM_PRIMARY | mirror-earth.com | 主上游地址 |
| NOMINATIM_BACKUP_1 | photon.komoot.io | 备选上游1 |
| NOMINATIM_BACKUP_2 | openstreetmap.org | 备选上游2 |
| LOG_RETENTION_DAYS | 30 | 日志保留天数 |

### 认证

默认账号: `admin` / `admin123`

建议在生产环境修改默认密码：

```bash
# 使用环境变量
export ADMIN_USERNAME=your_username
export ADMIN_PASSWORD=your_password
```

## 项目结构

```
nominatim-cache/
├── server/                 # Node.js 后端
│   ├── src/
│   │   ├── index.ts       # 主入口
│   │   ├── routes/        # API 路由
│   │   ├── services/      # 业务逻辑
│   │   ├── middleware/    # 中间件
│   │   └── utils/         # 工具函数
│   └── data/              # SQLite 数据库
├── frontend/              # 管理页面
│   ├── index.html         # Dashboard
│   ├── list.html          # 缓存清单
│   └── js/                # 前端脚本
├── docs/                  # 文档
│   ├── DESIGN.md          # 设计文档
│   └── API.md             # API 文档
├── docker-compose.yml     # Docker 配置
└── README.md              # 本文件
```

## API 文档

详见 [API 文档](docs/API.md)

## 设计文档

详见 [设计文档](docs/DESIGN.md)

## 上游 Nominatim 源

服务按以下优先级调用上游：

1. **镜像地球代理** (api.mirror-earth.com) - 首选，国内访问稳定
2. **Photon** (photon.komoot.io) - 备选，支持 CORS
3. **OSM 官方** (nominatim.openstreetmap.org) - 最后兜底

## 生产部署

### 1. 安装 Node.js

```bash
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt install nodejs
```

### 2. 使用 PM2 部署

```bash
# 安装 PM2
sudo npm install -g pm2

# 启动服务
cd server
npm install --production
npm run build
pm2 start ecosystem.config.js

# 配置开机自启
pm2 startup
pm2 save
```

### 3. Nginx 配置

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 4. 配置 HTTPS

使用 Let's Encrypt：

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## 备份与恢复

```bash
# 备份
cp server/data/cache.db backup/cache_$(date +%Y%m%d).db

# 恢复
cp backup/cache_20240115.db server/data/cache.db
```

## 许可证

MIT License
