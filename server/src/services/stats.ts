/**
 * 统计服务
 */

import { getDatabase } from './database.js';

interface StatsData {
  total_requests: number;
  cache_hits: number;
  cache_misses: number;
  hit_rate: number;
  cache_count: number;
  uptime: number;
  last_updated_at: number;
}

/**
 * 获取统计数据
 */
export async function getStats(): Promise<StatsData> {
  try {
    const db = getDatabase();

    // 获取统计表数据
    const statsRow = db.prepare(`
      SELECT * FROM stats WHERE id = 1
    `).get() as any;

    // 获取缓存数量
    const cacheCountRow = db.prepare(`
      SELECT COUNT(*) as count FROM cache_entries
    `).get() as any;

    const stats = statsRow || {
      total_requests: 0,
      cache_hits: 0,
      cache_misses: 0,
      last_updated_at: Date.now()
    };

    const hitRate = stats.total_requests > 0
      ? parseFloat(((stats.cache_hits / stats.total_requests) * 100).toFixed(1))
      : 0;

    return {
      total_requests: stats.total_requests,
      cache_hits: stats.cache_hits,
      cache_misses: stats.cache_misses,
      hit_rate: hitRate,
      cache_count: cacheCountRow.count,
      uptime: Math.floor((Date.now() - stats.last_updated_at) / 1000),
      last_updated_at: stats.last_updated_at
    };

  } catch (error) {
    console.error('[Stats] 获取统计失败:', error);
    return {
      total_requests: 0,
      cache_hits: 0,
      cache_misses: 0,
      hit_rate: 0,
      cache_count: 0,
      uptime: 0,
      last_updated_at: Date.now()
    };
  }
}

/**
 * 获取访问日志
 */
export async function getLogs(
  page: number = 1,
  limit: number = 100,
  cached?: number
): Promise<{ data: any[]; total: number }> {
  try {
    const db = getDatabase();
    const offset = (page - 1) * limit;

    let whereClause = '';
    let params: any[] = [];

    if (cached !== undefined) {
      whereClause = `WHERE cached = ?`;
      params.push(cached);
    }

    // 获取总数
    const totalRow = db.prepare(`
      SELECT COUNT(*) as count FROM access_logs ${whereClause}
    `).get(...params) as any;
    const total = totalRow.count;

    // 获取日志
    const rows = db.prepare(`
      SELECT id, cache_key, lat, lon, cached, accessed_at
      FROM access_logs
      ${whereClause}
      ORDER BY accessed_at DESC
      LIMIT ? OFFSET ?
    `).all(...params, limit, offset) as any[];

    return {
      data: rows.map(row => ({
        ...row,
        accessed_at_str: new Date(row.accessed_at).toLocaleString('zh-CN')
      })),
      total
    };

  } catch (error) {
    console.error('[Stats] 获取日志失败:', error);
    return { data: [], total: 0 };
  }
}

/**
 * 获取每日统计 (最近7天)
 */
export async function getDailyStats(): Promise<any[]> {
  try {
    const db = getDatabase();
    const sevenDaysAgo = Date.now() - (7 * 24 * 60 * 60 * 1000);

    const rows = db.prepare(`
      SELECT
        DATE(accessed_at / 1000, 'unixepoch', 'localtime') as date,
        SUM(cached) as hits,
        SUM(1 - cached) as misses,
        COUNT(*) as total
      FROM access_logs
      WHERE accessed_at > ?
      GROUP BY DATE(accessed_at / 1000, 'unixepoch', 'localtime')
      ORDER BY date ASC
    `).all(sevenDaysAgo) as any[];

    // 补全缺失的日期
    const result = [];
    for (let i = 6; i >= 0; i--) {
      const date = new Date();
      date.setDate(date.getDate() - i);
      const dateStr = date.toISOString().split('T')[0];

      const found = rows.find(r => r.date === dateStr);
      result.push({
        date: dateStr,
        label: `${date.getMonth() + 1}/${date.getDate()}`,
        hits: found ? found.hits : 0,
        misses: found ? found.misses : 0,
        total: found ? found.total : 0
      });
    }

    return result;

  } catch (error) {
    console.error('[Stats] 获取每日统计失败:', error);
    return [];
  }
}
