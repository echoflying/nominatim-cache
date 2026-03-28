/**
 * 数据库迁移脚本
 */

import { initDatabase, closeDatabase } from '../services/database.js';
import { loadConfig } from '../utils/config.js';

async function main() {
  console.log('[Migrate] 开始执行数据库迁移...');

  const config = loadConfig();
  const dbPath = config.DATABASE_PATH;

  try {
    initDatabase(dbPath);
    console.log('[Migrate] 迁移完成:', dbPath);
    closeDatabase();
  } catch (error) {
    console.error('[Migrate] 迁移失败:', error);
    process.exit(1);
  }
}

main();
