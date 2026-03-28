/**
 * 数据库初始化脚本
 */

import { initDatabase, closeDatabase } from '../services/database.js';
import { loadConfig } from '../utils/config.js';

async function main() {
  console.log('[Init] 开始初始化数据库...');
  const config = loadConfig();
  const dbPath = config.DATABASE_PATH;

  try {
    const db = initDatabase(dbPath);
    console.log('[Init] 数据库初始化完成:', dbPath);

    // 验证表
    const tables = db.prepare(`
      SELECT name FROM sqlite_master WHERE type='table'
    `).all();

    console.log('[Init] 已创建的表:', (tables as { name: string }[]).map(t => t.name).join(', '));

    closeDatabase();
    console.log('[Init] 完成!');

  } catch (error) {
    console.error('[Init] 初始化失败:', error);
    process.exit(1);
  }
}

main();
