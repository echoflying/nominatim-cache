# Nominatim Cache Service

一个自托管的 Nominatim 逆地理编码缓存服务，提供透明缓存、访问统计和管理界面。

## 功能特性

- **透明缓存**: 接口兼容官方 Nominatim API，缓存永久有效
- **多上游源**: 支持多个 Nominatim 镜像源，自动故障切换
- **访问统计**: 实时统计请求数、命中率等指标
- **管理界面**: 简洁的 Dashboard 和缓存清单页面
- **Basic Auth**: 保护管理功能
- **访问日志**: 保留 30 天访问记录
- **请求日志页面**: 查看实时请求历史
- **SQLite 存储**: 轻量级数据库，易于备份迁移

## 快速开始

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

## 使用方法

### 逆地理编码调用

```bash
curl "http://localhost:3000/api/reverse?lat=30.28746&lon=120.16145"
```

### 管理功能

```bash
# 查看统计 (admin/admin123)
curl -u admin:admin123 http://localhost:3000/admin/stats/daily

# 获取缓存列表
curl -u admin:admin123 "http://localhost:3000/admin/cache/list?page=1&limit=20"
```

## 配置说明

| 变量 | 默认值 | 说明 |
|------|--------|------|
| PORT | 3000 | 服务端口 |
| ADMIN_USERNAME | admin | 管理用户名 (生产环境必须修改) |
| ADMIN_PASSWORD | admin123 | 管理密码 (生产环境必须修改) |
| NOMINATIM_PRIMARY | https://api.mirror-earth.com/nominatim/reverse | 主上游 |
| NOMINATIM_BACKUP_1 | https://photon.komoot.io/reverse | 备选上游1 |
| NOMINATIM_BACKUP_2 | https://nominatim.openstreetmap.org/reverse | 备选上游2 |

## 上游 Nominatim 源

服务按以下优先级调用上游：

1. **镜像地球代理** (https://api.mirror-earth.com/nominatim/reverse) - 首选，国内访问稳定
2. **Photon** (https://photon.komoot.io/reverse) - 备选，支持 CORS
3. **OSM 官方** (https://nominatim.openstreetmap.org/reverse) - 最后兜底

## 文档索引

| 文档 | 用途 |
|------|------|
| [API 文档](docs/API.md) | 接口说明、参数、响应格式 |
| [设计文档](docs/DESIGN.md) | 需求、架构、数据库设计 |
| [部署文档](docs/DEPLOYMENT.md) | Docker/PM2 生产部署 |

## 备份与恢复

```bash
# 备份
cp server/data/cache.db backup/cache_$(date +%Y%m%d).db

# 恢复
cp backup/cache_20240115.db server/data/cache.db
```

## 许可证

MIT License
