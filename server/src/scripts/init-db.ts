/**
 * 数据库初始化脚本
 */

import path from 'path';
import { fileURLToPath } from 'url';
import { initDatabase, closeDatabase } from '../services/database.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

async function main() {
  console.log('[Init] 开始初始化数据库...');

  const dbPath = path.resolve(__dirname, '../../data/cache.db');

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
