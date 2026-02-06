/**
 * 日志中间件
 */

import { Request, Response, NextFunction } from 'express';
import { getDatabase } from '../services/database.js';

export function logger(req: Request, res: Response, next: NextFunction): void {
  const start = Date.now();

  res.on('finish', () => {
    const duration = Date.now() - start;

    // 只记录 API 请求
    if (req.path.startsWith('/api/') || req.path.startsWith('/admin/')) {
      console.log(`[${new Date().toISOString()}] ${req.method} ${req.path} ${res.statusCode} ${duration}ms`);
    }
  });

  next();
}

/**
 * 保存访问日志 (导出供路由使用)
 * @param req - Express 请求对象
 * @param isCached - 是否命中缓存 (true=命中, false=未命中)
 * @param success - API 调用是否成功
 */
export async function saveAccessLog(req: Request, isCached: boolean, success: boolean): Promise<void> {
  try {
    console.log(`[Logger] 保存日志: isCached=${isCached}, success=${success}`);
    const db = getDatabase();
    const lat = parseFloat(req.query.lat as string);
    const lon = parseFloat(req.query.lon as string);
    const cacheKey = formatCacheKey(lat, lon);

    // 插入日志
    const insertResult = db.prepare(`
      INSERT INTO access_logs (cache_key, lat, lon, cached, accessed_at)
      VALUES (?, ?, ?, ?, ?)
    `).run(cacheKey, lat, lon, isCached ? 1 : 0, Date.now());
    console.log(`[Logger] 日志插入: changes=${insertResult.changes}`);

    // 更新统计
    const updateResult = db.prepare(`
      UPDATE stats SET
        total_requests = total_requests + 1,
        cache_hits = cache_hits + ?,
        cache_misses = cache_misses + ?,
        last_updated_at = ?
      WHERE id = 1
    `).run(isCached ? 1 : 0, (!isCached && success) ? 1 : 0, Date.now());
    console.log(`[Logger] 统计更新: hits+${isCached ? 1 : 0}, misses+${(!isCached && success) ? 1 : 0}`);

    // 清理旧日志 (30天前)
    const cutoffTime = Date.now() - (30 * 24 * 60 * 60 * 1000);
    db.prepare(`DELETE FROM access_logs WHERE accessed_at < ?`).run(cutoffTime);

  } catch (err) {
    const error = err instanceof Error ? err : new Error(String(err));
    console.error('[Logger] 保存日志失败:', error.message);
  }
}

/**
 * 格式化缓存键
 */
function formatCacheKey(lat: number, lon: number): string {
  const latFixed = lat.toFixed(4);
  const lonFixed = lon.toFixed(4);
  return `${latFixed}_${lonFixed}`;
}
