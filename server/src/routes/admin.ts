/**
 * 管理路由
 */

import { Router, Request, Response } from 'express';
import { getAllCaches, getCacheEntry, setCache, adminUpdateCache, deleteCache, clearAllCache } from '../services/cache.js';
import { fetchAllUpstream } from '../services/nominatim.js';
import { getLogs, getDailyStats } from '../services/stats.js';

const router = Router();

/**
 * 获取缓存列表
 * GET /admin/cache/list?page=1&limit=20&search=关键词
 */
router.get('/cache/list', async (req: Request, res: Response): Promise<void> => {
  try {
    const page = parseInt(req.query.page as string) || 1;
    const limit = Math.min(parseInt(req.query.limit as string) || 20, 100);
    const search = req.query.search as string | undefined;

    const result = await getAllCaches(page, limit, search);

    res.json({
      success: true,
      data: result.data,
      pagination: {
        page,
        limit,
        total: result.total,
        total_pages: result.totalPages,
        has_more: page < result.totalPages
      }
    });

  } catch (error) {
    console.error('[Admin] 获取缓存列表失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to get cache list'
    });
  }
});

/**
 * 获取单个缓存详情
 * GET /admin/cache/:key
 */
router.get('/cache/:key', async (req: Request, res: Response): Promise<void> => {
  try {
    const cacheKey = req.params.key;
    const entry = await getCacheEntry(cacheKey);

    if (!entry) {
      res.status(404).json({
        success: false,
        error: 'Cache entry not found'
      });
      return;
    }

    res.json({
      success: true,
      data: entry
    });

  } catch (error) {
    console.error('[Admin] 获取缓存详情失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to get cache entry'
    });
  }
});

/**
 * 删除单个缓存
 * DELETE /admin/cache/:key
 */
router.delete('/cache/:key', async (req: Request, res: Response): Promise<void> => {
  try {
    const cacheKey = req.params.key;
    const deleted = await deleteCache(cacheKey);

    if (!deleted) {
      res.status(404).json({
        success: false,
        error: 'Cache entry not found'
      });
      return;
    }

    res.json({
      success: true,
      message: 'Cache deleted',
      deleted_key: cacheKey
    });

  } catch (error) {
    console.error('[Admin] 删除缓存失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to delete cache'
    });
  }
});

/**
 * 清空所有缓存
 * POST /admin/cache/clear
 */
router.post('/cache/clear', async (req: Request, res: Response): Promise<void> => {
  try {
    const count = await clearAllCache();

    res.json({
      success: true,
      message: 'All caches cleared',
      deleted_count: count
    });

  } catch (error) {
    console.error('[Admin] 清空缓存失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to clear cache'
    });
  }
});

/**
 * 获取访问日志
 * GET /admin/logs?page=1&limit=100&cached=1
 */
router.get('/logs', async (req: Request, res: Response): Promise<void> => {
  try {
    const page = parseInt(req.query.page as string) || 1;
    const limit = Math.min(parseInt(req.query.limit as string) || 100, 1000);
    const cached = req.query.cached !== undefined
      ? parseInt(req.query.cached as string)
      : undefined;

    const result = await getLogs(page, limit, cached);

    res.json({
      success: true,
      data: result.data,
      pagination: {
        page,
        limit,
        total: result.total,
        has_more: page * limit < result.total
      }
    });

  } catch (error) {
    console.error('[Admin] 获取日志失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to get logs'
    });
  }
});

/**
 * 获取每日统计
 * GET /admin/stats/daily
 */
router.get('/stats/daily', async (req: Request, res: Response): Promise<void> => {
  try {
    const dailyStats = await getDailyStats();
    res.json({
      success: true,
      data: dailyStats
    });
  } catch (error) {
    console.error('[Admin] 获取每日统计失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to get daily stats'
    });
  }
});

/**
 * 查询所有上游源
 * POST /admin/query
 */
router.post('/query', async (req: Request, res: Response): Promise<void> => {
  try {
    const lat = parseFloat(req.body.lat);
    const lon = parseFloat(req.body.lon);

    if (isNaN(lat) || isNaN(lon)) {
      res.status(400).json({
        success: false,
        error: 'Invalid coordinates'
      });
      return;
    }

    console.log(`[Admin] 查询上游: lat=${lat}, lon=${lon}`);

    const allResults = await fetchAllUpstream(lat, lon);

    const results = allResults.map(r => ({
      source: r.source,
      success: r.success,
      display_name: r.success ? r.display_name : `失败: ${r.error}`,
      address: r.address || {}
    }));

    const successCount = allResults.filter(r => r.success).length;

    res.json({
      success: true,
      coordinates: {
        lat: parseFloat(lat.toFixed(4)),
        lon: parseFloat(lon.toFixed(4))
      },
      results,
      summary: {
        total: allResults.length,
        success: successCount,
        failed: allResults.length - successCount
      }
    });

  } catch (error) {
    console.error('[Admin] 查询上游失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to query upstream'
    });
  }
});

/**
 * 手动添加/更新缓存
 * POST /admin/cache/add
 */
router.post('/cache/add', async (req: Request, res: Response): Promise<void> => {
  try {
    const lat = parseFloat(req.body.lat);
    const lon = parseFloat(req.body.lon);
    const displayName = req.body.display_name;
    const address = req.body.address || {};

    if (isNaN(lat) || isNaN(lon) || !displayName) {
      res.status(400).json({
        success: false,
        error: 'Missing required fields'
      });
      return;
    }

    const cacheKey = `${lat.toFixed(4)}_${lon.toFixed(4)}`;

    const existing = await getCacheEntry(cacheKey);
    const wasOverwritten = !!existing;

    const fullResponse = JSON.stringify({ lat, lon, display_name: displayName, address });

    if (existing) {
      await adminUpdateCache(cacheKey, lat, lon, displayName, existing.place_type || 'other', fullResponse, 'manual');
    } else {
      await setCache(
        cacheKey,
        lat,
        lon,
        { display_name: displayName, place_type: 'other', full_response: fullResponse },
        { lat: lat.toString(), lon: lon.toString(), display_name: displayName, address } as any,
        'manual'
      );
    }

    res.json({
      success: true,
      message: wasOverwritten ? '已更新缓存' : '已添加到缓存',
      cache_key: cacheKey,
      overwritten: wasOverwritten
    });

  } catch (error) {
    console.error('[Admin] 添加缓存失败:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to add cache'
    });
  }
});

export default router;
