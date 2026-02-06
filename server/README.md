# Nominatim 缓存服务

## 快速开始

### 1. 安装依赖

```bash
cd server
npm install
```

### 2. 初始化数据库

```bash
npm run db:init
```

### 3. 启动开发服务器

```bash
npm run dev
```

服务将在 `http://localhost:3000` 启动。

## 目录结构

```
server/
├── src/
│   ├── index.ts            # 主入口
│   ├── routes/             # API 路由
│   ├── services/           # 业务逻辑
│   ├── middleware/         # 中间件
│   └── utils/              # 工具函数
├── data/                   # SQLite 数据库目录
├── package.json
└── tsconfig.json
```

## 可用命令

| 命令 | 说明 |
|------|------|
| `npm run dev` | 启动开发服务器 (热重载) |
| `npm run build` | 编译 TypeScript |
| `npm start` | 启动生产服务器 |
| `npm run db:init` | 初始化数据库 |

## 配置

通过环境变量或 `.env` 文件配置：

```env
PORT=3000
DATABASE_PATH=./data/cache.db
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123
```
