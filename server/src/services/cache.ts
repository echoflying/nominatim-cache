/**
 * 缓存服务
 */

import { getDatabase } from './database.js';
import { parseNominatimResponse, NominatimResponse, selectDisplayPlace } from './nominatim.js';

interface CacheData {
  lat: string;
  lon: string;
  display_name: string;
  address: Record<string, any>;
  place_name: string;
  place_level: string;
  place_quality: 'precise' | 'regional' | 'coarse' | 'none';
  place_source_field: string;
  cached: boolean;
  cache_stats?: {
    first_cached_at: number;
    last_accessed_at: number;
    access_count: number;
  };
}

/**
 * 获取缓存
 */
export async function getCache(cacheKey: string): Promise<CacheData | null> {
  try {
    const db = getDatabase();
    const row = db.prepare(`
      SELECT * FROM cache_entries WHERE cache_key = ?
    `).get(cacheKey) as any;

    if (!row) {
      return null;
    }

    // 更新访问时间和次数
    db.prepare(`
      UPDATE cache_entries
      SET last_accessed_at = ?, access_count = access_count + 1
      WHERE cache_key = ?
    `).run(Date.now(), cacheKey);

    // 解析响应
    const response = JSON.parse(row.nominatim_response);

    return {
      lat: response.lat,
      lon: response.lon,
      display_name: row.display_name,
      address: response.address,
      ...selectDisplayPlace(response),
      cached: true as const,
      cache_stats: {
        first_cached_at: row.first_cached_at,
        last_accessed_at: row.last_accessed_at,
        access_count: row.access_count
      }
    };

  } catch (error) {
    console.error('[Cache] 获取缓存失败:', error);
    return null;
  }
}

/**
 * 设置缓存
 */
export async function setCache(
  cacheKey: string,
  lat: number,
  lon: number,
  parsed: { display_name: string; place_type: string; full_response: string },
  nominatimData: NominatimResponse,
  source?: string
): Promise<void> {
  try {
    const db = getDatabase();
    const now = Date.now();

    const existing = db.prepare(`
      SELECT id FROM cache_entries WHERE cache_key = ?
    `).get(cacheKey);

    if (existing) {
      const addressJson = nominatimData.address ? JSON.stringify(nominatimData.address) : null;
      db.prepare(`
        UPDATE cache_entries
        SET last_accessed_at = ?, access_count = access_count + 1, address = ?
        WHERE cache_key = ?
      `).run(now, addressJson, cacheKey);
    } else {
      const addressJson = nominatimData.address ? JSON.stringify(nominatimData.address) : null;
      db.prepare(`
        INSERT INTO cache_entries (cache_key, lat, lon, display_name, address, place_type, source, nominatim_response, first_cached_at, last_accessed_at, access_count)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
      `).run(
        cacheKey,
        lat,
        lon,
        parsed.display_name,
        addressJson,
        parsed.place_type,
        source || null,
        parsed.full_response,
        now,
        now
      );
    }

    console.log(`[Cache] 已缓存: ${cacheKey} (${source || 'unknown'}) - ${parsed.display_name.slice(0, 20)}...`);

  } catch (error) {
    console.error('[Cache] 设置缓存失败:', error);
  }
}

/**
 * 管理员更新缓存（不增加访问次数）
 */
export async function adminUpdateCache(
  cacheKey: string,
  lat: number,
  lon: number,
  displayName: string,
  placeType: string,
  fullResponse: string,
  source?: string
): Promise<void> {
  try {
    const db = getDatabase();
    const now = Date.now();

    db.prepare(`
      UPDATE cache_entries
      SET lat = ?, lon = ?, display_name = ?, place_type = ?, source = ?, nominatim_response = ?, first_cached_at = ?
      WHERE cache_key = ?
    `).run(lat, lon, displayName, placeType, source || null, fullResponse, now, cacheKey);

    console.log(`[Cache] 管理员更新: ${cacheKey} (${source || 'manual'}) - ${displayName.slice(0, 20)}...`);

  } catch (error) {
    console.error('[Cache] 管理员更新缓存失败:', error);
  }
}

/**
 * 获取缓存条目详情
 */
export async function getCacheEntry(cacheKey: string): Promise<any | null> {
  try {
    const db = getDatabase();
    const row = db.prepare(`
      SELECT * FROM cache_entries WHERE cache_key = ?
    `).get(cacheKey) as any;

    if (!row) {
      return null;
    }

    return {
      cache_key: row.cache_key,
      lat: row.lat,
      lon: row.lon,
      display_name: row.display_name,
      place_type: row.place_type,
      source: row.source,
      nominatim_response: JSON.parse(row.nominatim_response),
      ...selectDisplayPlace(JSON.parse(row.nominatim_response)),
      first_cached_at: row.first_cached_at,
      last_accessed_at: row.last_accessed_at,
      access_count: row.access_count
    };

  } catch (error) {
    console.error('[Cache] 获取条目失败:', error);
    return null;
  }
}

/**
 * 获取所有缓存 (分页)
 */
export async function getAllCaches(
   page: number = 1,
   limit: number = 20,
   search?: string,
   sort?: string,
   order?: string
): Promise<{ data: any[]; total: number; totalPages: number }> {
  try {
    const db = getDatabase();
    const offset = (page - 1) * limit;

    let whereClause = '';
    let params: any[] = [];

    if (search) {
      whereClause = `WHERE display_name LIKE ?`;
      params.push(`%${search}%`);
    }

    // 获取总数
    const totalRow = db.prepare(`
      SELECT COUNT(*) as count FROM cache_entries ${whereClause}
    `).get(...params) as any;
    const total = totalRow.count;

     // Determine sort column and order
     const sortColumn = sort === 'cached' ? 'first_cached_at' : 'last_accessed_at';
     const sortDirection = (order && order.toUpperCase() === 'ASC') ? 'ASC' : 'DESC';
     
     // 获取列表
     const rows = db.prepare(`
       SELECT cache_key, lat, lon, display_name, address, nominatim_response, place_type, source, first_cached_at, last_accessed_at, access_count
       FROM cache_entries
       ${whereClause}
       ORDER BY ${sortColumn} ${sortDirection}
       LIMIT ? OFFSET ?
     `).all(...params, limit, offset) as any[];

     // 如果 address 为空，尝试从 nominatim_response 中提取
     for (const row of rows) {
       try {
         const response = JSON.parse(row.nominatim_response);
         Object.assign(row, selectDisplayPlace(response));
         if (!row.address && response.address) row.address = JSON.stringify(response.address);
       } catch (_) {
         Object.assign(row, { place_name: '', place_level: 'unknown', place_quality: 'none', place_source_field: 'none' });
       }
     }

    return {
      data: rows,
      total,
      totalPages: Math.ceil(total / limit)
    };

  } catch (error) {
    console.error('[Cache] 获取列表失败:', error);
    return { data: [], total: 0, totalPages: 0 };
  }
}

/**
 * 删除单个缓存
 */
export async function deleteCache(cacheKey: string): Promise<boolean> {
  try {
    const db = getDatabase();
    const result = db.prepare(`
      DELETE FROM cache_entries WHERE cache_key = ?
    `).run(cacheKey);

    return (result.changes ?? 0) > 0;
  } catch (error) {
    console.error('[Cache] 删除失败:', error);
    return false;
  }
}

/**
 * 清空所有缓存
 */
export async function clearAllCache(): Promise<number> {
  try {
    const db = getDatabase();
    const result = db.prepare(`DELETE FROM cache_entries`).run();
    console.log(`[Cache] 已清空 ${result.changes ?? 0} 条缓存`);
    return result.changes ?? 0;
  } catch (error) {
    console.error('[Cache] 清空失败:', error);
    return 0;
  }
}
