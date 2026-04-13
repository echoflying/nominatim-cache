/**
 * Nominatim 缓存服务 - 主入口
 */

import express from 'express';
import cors from 'cors';
import helmet from 'helmet';
import compression from 'compression';
import path from 'path';
import { fileURLToPath } from 'url';

import { loadConfig } from './utils/config.js';
import { initDatabase } from './services/database.js';
import { logger } from './middleware/logger.js';
import { authMiddleware } from './middleware/auth.js';

// 路由
import nominatimRouter from './routes/nominatim.js';
import adminRouter from './routes/admin.js';
import statsRouter from './routes/stats.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

async function main() {
  // 1. 加载配置
  const config = loadConfig();
  console.log(`[Server] 启动中... (${config.NODE_ENV})`);

  // 2. 初始化数据库
  await initDatabase(config.DATABASE_PATH);
  console.log('[Server] 数据库初始化完成');

  // 3. 创建 Express 应用
  const app = express();

  // 4. 中间件
  app.use(helmet({
    contentSecurityPolicy: {
      directives: {
        'upgrade-insecure-requests': null
      }
    },
    crossOriginOpenerPolicy: false,
    originAgentCluster: false
  }));
  app.use(cors());
  app.use(compression());
  app.use(express.json());
  app.use(logger);

  // 5. 静态文件服务 (前端页面)
  const frontendPath = path.resolve(__dirname, '../../frontend');
  app.use(express.static(frontendPath));

  app.get('/health', (req, res) => {
    res.json({
      status: 'ok',
      service: 'nominatim-cache',
      environment: config.NODE_ENV,
      timestamp: Date.now(),
      uptime: Math.round(process.uptime())
    });
  });

  // 6. API 路由 (无需认证)
  app.use('/api/reverse', nominatimRouter);

  // 7. 管理路由 (需要 Basic Auth)
  app.use('/admin', authMiddleware, adminRouter);
  app.use('/admin/stats', authMiddleware, statsRouter);

  // 8. 页面路由
  app.get('/', (req, res) => {
    res.sendFile(path.join(frontendPath, 'index.html'));
  });

  app.get('/list.html', (req, res) => {
    res.sendFile(path.join(frontendPath, 'list.html'));
  });

  // 9. 错误处理
  app.use((err: Error, req: express.Request, res: express.Response, next: express.NextFunction) => {
    console.error('[Error]', err.stack);
    res.status(500).json({
      success: false,
      error: 'Internal Server Error'
    });
  });

  // 10. 启动服务
  const port = config.PORT;
  app.listen(port, () => {
    console.log(`[Server] 服务已启动: http://localhost:${port}`);
    console.log(`[Server] Dashboard: http://localhost:${port}/`);
    console.log(`[Server] 缓存列表: http://localhost:${port}/list.html`);
  });
}

main().catch((error) => {
  console.error('[Server] 启动失败:', error);
  process.exit(1);
});
