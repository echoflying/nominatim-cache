/**
 * SQLite 数据库服务
 */

import Database from 'better-sqlite3';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

let db: Database.Database | null = null;

export interface CacheEntry {
  id: number;
  cache_key: string;
  lat: number;
  lon: number;
  display_name: string;
  place_type: string;
  nominatim_response: string;
  first_cached_at: number;
  last_accessed_at: number;
  access_count: number;
}

export interface Stats {
  id: number;
  total_requests: number;
  cache_hits: number;
  cache_misses: number;
  last_updated_at: number;
}

export interface AccessLog {
  id: number;
  cache_key: string | null;
  lat: number | null;
  lon: number | null;
  cached: number;
  accessed_at: number;
}

/**
 * 初始化数据库
 */
export function initDatabase(dbPath: string): Database.Database {
  // 确保目录存在
  const dir = path.dirname(dbPath);
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }

  db = new Database(dbPath);
  db.pragma('journal_mode = WAL'); // 使用 WAL 模式提升并发性能

  // 创建表
  createTables();

  // 初始化统计表
  initStats();

  return db;
}

/**
 * 创建数据库表
 */
function createTables(): void {
  if (!db) throw new Error('Database not initialized');

  // 缓存条目表
  db.exec(`
    CREATE TABLE IF NOT EXISTS cache_entries (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      cache_key TEXT UNIQUE NOT NULL,
      lat REAL NOT NULL,
      lon REAL NOT NULL,
      display_name TEXT,
      place_type TEXT,
      source TEXT,
      nominatim_response TEXT NOT NULL,
      first_cached_at INTEGER NOT NULL,
      last_accessed_at INTEGER NOT NULL,
      access_count INTEGER DEFAULT 1
    )
  `);

  // 统计表
  db.exec(`
    CREATE TABLE IF NOT EXISTS stats (
      id INTEGER PRIMARY KEY CHECK (id = 1),
      total_requests INTEGER DEFAULT 0,
      cache_hits INTEGER DEFAULT 0,
      cache_misses INTEGER DEFAULT 0,
      last_updated_at INTEGER NOT NULL
    )
  `);

  // 访问日志表
  db.exec(`
    CREATE TABLE IF NOT EXISTS access_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      cache_key TEXT,
      lat REAL,
      lon REAL,
      cached INTEGER DEFAULT 0,
      accessed_at INTEGER NOT NULL
    )
  `);

  // 创建索引
  db.exec(`CREATE INDEX IF NOT EXISTS idx_cache_key ON cache_entries(cache_key)`);
  db.exec(`CREATE INDEX IF NOT EXISTS idx_lat_lon ON cache_entries(lat, lon)`);
  db.exec(`CREATE INDEX IF NOT EXISTS idx_accessed_at ON access_logs(accessed_at DESC)`);
  db.exec(`CREATE INDEX IF NOT EXISTS idx_cache_key_logs ON access_logs(cache_key)`);
}

/**
 * 初始化统计表
 */
function initStats(): void {
  if (!db) throw new Error('Database not initialized');

  const existing = db.prepare('SELECT id FROM stats WHERE id = 1').get();
  if (!existing) {
    db.prepare(`
      INSERT INTO stats (id, total_requests, cache_hits, cache_misses, last_updated_at)
      VALUES (1, 0, 0, 0, ?)
    `).run(Date.now());
  }
}

/**
 * 获取数据库实例
 */
export function getDatabase(): Database.Database {
  if (!db) {
    throw new Error('Database not initialized. Call initDatabase() first.');
  }
  return db;
}

/**
 * 关闭数据库连接
 */
export function closeDatabase(): void {
  if (db) {
    db.close();
    db = null;
  }
}
