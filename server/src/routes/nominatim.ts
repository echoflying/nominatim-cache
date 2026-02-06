/**
 * Nominatim 代理路由
 */

import { Router, Request, Response } from 'express';
import { getCache, setCache } from '../services/cache.js';
import { fetchNominatim, parseNominatimResponse } from '../services/nominatim.js';
import { saveAccessLog } from '../middleware/logger.js';

const router = Router();

/**
 * 逆地理编码接口
 * GET /api/reverse?lat=30.28746&lon=120.16145
 */
router.get('/', async (req: Request, res: Response): Promise<void> => {
  try {
    const lat = parseFloat(req.query.lat as string);
    const lon = parseFloat(req.query.lon as string);

    // 参数验证
    if (isNaN(lat) || isNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180) {
      res.status(400).json({
        error: 'Invalid coordinates'
      });
      return;
    }

    const cacheKey = formatCacheKey(lat, lon);

    // 1. 尝试从缓存获取
    const cached = await getCache(cacheKey);
    if (cached) {
      const displayName = cached.display_name?.slice(0, 30) || '未知';
      console.log(`[Nominatim] ${new Date().toISOString()} 缓存命中: ${cacheKey} - ${displayName}...`);
      res.json(cached);
      await saveAccessLog(req, true, true);
      return;
    }

    // 2. 缓存未命中，调用上游
    console.log(`[Nominatim] ${new Date().toISOString()} 缓存未命中: ${cacheKey}`);
    console.log(`[Nominatim] ${new Date().toISOString()} 调用上游: lat=${lat}, lon=${lon}`);

    const nominatimData = await fetchNominatim(lat, lon);
    const displayName = nominatimData.display_name?.slice(0, 30) || '未知';

    console.log(`[Nominatim] ${new Date().toISOString()} 上游返回: ${displayName}...`);

    const parsed = parseNominatimResponse(nominatimData);

    // 3. 存入缓存
    await setCache(cacheKey, lat, lon, parsed, nominatimData, nominatimData._source);
    console.log(`[Nominatim] ${new Date().toISOString()} 已缓存: ${cacheKey} - ${displayName}...`);

    // 4. 返回 Nominatim 官方格式
    res.json(nominatimData);
    // 未命中缓存，标记 cached=0
    await saveAccessLog(req, false, true);

  } catch (error) {
    const errorMsg = error instanceof Error ? error.message : 'Unknown error';
    console.error(`[Nominatim] ${new Date().toISOString()} 失败: ${errorMsg}`);
    res.status(500).json({
      error: errorMsg
    });
    await saveAccessLog(req, false, false);
  }
});

/**
 * 格式化缓存键 (保留4位小数)
 */
function formatCacheKey(lat: number, lon: number): string {
  return `${lat.toFixed(4)}_${lon.toFixed(4)}`;
}

export default router;
