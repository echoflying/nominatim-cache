/**
 * Nominatim 上游调用服务
 * 基于 Sequential Round Robin + 断路器模式 + 指数退避封禁策略
 */

import axios from 'axios';
import { loadConfig } from '../utils/config.js';

const config = loadConfig();

export interface NominatimResponse {
  lat: string;
  lon: string;
  display_name: string;
  address: Record<string, any>;
  extratags?: Record<string, any>;
  type?: string;
  class?: string;
  error?: string;
  _source?: string;
}

interface UpstreamSource {
  url: string;
  name: string;
  basePriority: number;
}

interface SourceState {
  consecutiveFailures: number;
  banLevel: number;
  banExpiresAt: number;
}

const sources: UpstreamSource[] = [
  { url: config.NOMINATIM_PRIMARY, name: 'mirror-earth.com', basePriority: 1 },
  { url: config.NOMINATIM_BACKUP_1, name: 'photon.komoot.io', basePriority: 2 },
  { url: config.NOMINATIM_BACKUP_2, name: 'openstreetmap.org', basePriority: 3 }
];

const sourceStates: Map<string, SourceState> = new Map();
for (const s of sources) {
  sourceStates.set(s.name, {
    consecutiveFailures: 0,
    banLevel: 0,
    banExpiresAt: 0
  });
}

let pollingPointer = 0;

const UPSTREAM_INTERVAL = 1500;
const MAX_ATTEMPTS = 3;
const FAILURE_THRESHOLD = 3;
const BAN_DURATIONS = [0, 4, 8, 16, 32, 64, 128, 256];
const BLOCKED_TTL = 60 * 60 * 1000;

let lastUpstreamCall = 0;

/**
 * 等待上游间隔
 */
async function waitUpstreamInterval(): Promise<void> {
  const now = Date.now();
  const elapsed = now - lastUpstreamCall;
  if (elapsed < UPSTREAM_INTERVAL) {
    console.log(`[Nominatim] 上游限速，等待 ${UPSTREAM_INTERVAL - elapsed}ms...`);
    await new Promise(resolve => setTimeout(resolve, UPSTREAM_INTERVAL - elapsed));
  }
  lastUpstreamCall = Date.now();
}

/**
 * 获取当前时间戳
 */
function now(): number {
  return Date.now();
}

/**
 * 检查源是否可用
 */
function isSourceAvailable(source: UpstreamSource): boolean {
  const state = sourceStates.get(source.name)!;
  if (state.banExpiresAt > 0 && now() < state.banExpiresAt) {
    return false;
  }
  return true;
}

/**
 * 解除源封禁（到期自动恢复）
 */
function checkAndUnban(source: UpstreamSource): void {
  const state = sourceStates.get(source.name)!;
  if (state.banExpiresAt > 0 && now() >= state.banExpiresAt) {
    state.banLevel = 0;
    state.banExpiresAt = 0;
    state.consecutiveFailures = 0;
    console.log(`[Nominatim] ${source.name} 封禁已到期，恢复可用`);
  }
}

/**
 * 处理成功响应
 */
function handleSuccess(source: UpstreamSource): void {
  const state = sourceStates.get(source.name)!;
  state.consecutiveFailures = 0;
  state.banLevel = 0;
  state.banExpiresAt = 0;
}

/**
 * 处理失败响应
 */
function handleFailure(source: UpstreamSource): void {
  const state = sourceStates.get(source.name)!;
  state.consecutiveFailures++;

  if (state.consecutiveFailures >= FAILURE_THRESHOLD) {
    state.banLevel = Math.min(state.banLevel + 1, BAN_DURATIONS.length - 1);
    const duration = BAN_DURATIONS[state.banLevel] * 60 * 1000;
    state.banExpiresAt = now() + duration;
    console.log(`[Nominatim] ${source.name} 连续${FAILURE_THRESHOLD}次失败，触发封禁等级${state.banLevel}，时长${duration / 60000}分钟`);
  }
}

/**
 * 获取下一个可用源
 */
function getNextAvailableSource(): UpstreamSource | null {
  for (let i = 0; i < MAX_ATTEMPTS; i++) {
    const idx = pollingPointer % sources.length;
    pollingPointer++;
    const source = sources[idx];

    checkAndUnban(source);

    if (isSourceAvailable(source)) {
      return source;
    }
    console.log(`[Nominatim] ${source.name} 封禁中，跳过`);
  }

  return null;
}

/**
 * 根据上游类型构建请求参数
 */
function buildParams(lat: number, lon: number, sourceName: string): Record<string, any> {
  if (sourceName === 'photon.komoot.io') {
    return { lat, lon };
  }
  return {
    lat,
    lon,
    format: 'jsonv2',
    addressdetails: 1,
    extratags: 1,
    accept_language: 'zh-CN',
    zoom: 18
  };
}

/**
 * 检测是否为 Photon 响应格式
 */
function isPhotonResponse(data: any): boolean {
  return data && data.type === 'FeatureCollection' && Array.isArray(data.features);
}

/**
 * Photon 响应转换为 Nominatim 格式
 */
function convertPhotonToNominatim(data: any, lat: number, lon: number): NominatimResponse | null {
  if (!data || !data.features || !Array.isArray(data.features) || data.features.length === 0) {
    return null;
  }

  const props = data.features[0].properties || {};
  const addressFields = ['name', 'house_number', 'street', 'road', 'neighbourhood',
                        'suburb', 'city', 'town', 'village', 'county', 'state', 'country'];

  const displayNameParts: string[] = [];
  const address: Record<string, any> = {};

  addressFields.forEach(field => {
    if (props[field]) {
      address[field] = props[field];
      displayNameParts.push(props[field]);
    }
  });

  return {
    lat: lat.toString(),
    lon: lon.toString(),
    display_name: displayNameParts.length > 0 ? displayNameParts.reverse().join(', ') : props.name || '未知位置',
    address: address,
    type: props.type,
    extratags: props.extratags,
    error: undefined
  };
}

/**
 * 判断是否为有效 Nominatim 响应
 */
function isValidResponse(data: any): data is NominatimResponse {
  return data && !data.error && data.display_name;
}

/**
 * 调用 Nominatim API
 */
export async function fetchNominatim(lat: number, lon: number): Promise<NominatimResponse> {
  await waitUpstreamInterval();

  const errors: string[] = [];

  for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
    const source = getNextAvailableSource();

    if (!source) {
      console.log(`[Nominatim] 所有源均不可用`);
      break;
    }

    try {
      console.log(`[Nominatim] [${attempt + 1}/${MAX_ATTEMPTS}] 尝试: ${source.name}`);

      const params = buildParams(lat, lon, source.name);
      const response = await axios.get(source.url, {
        params,
        timeout: 10000,
        headers: {
          'User-Agent': 'PhotoOrdo-Local/1.0 (echoflying@gmail.com)',
          'Referer': 'http://localhost:3000/',
          'Accept-Language': 'zh-CN,zh'
        }
      });

      console.log(`[Nominatim] ${source.name} status=${response.status}`);

      const status = response.status;
      const data: any = response.data;

      if (status === 400 || status === 429 || status >= 500) {
        errors.push(`${source.name}: HTTP ${status}`);
        console.log(`[Nominatim] ${source.name} HTTP ${status}，标记失败`);
        handleFailure(source);
        continue;
      }

      if (typeof data === 'string') {
        if (data.startsWith('<!DOCTYPE') || data.startsWith('<html')) {
          errors.push(`${source.name}: 返回 HTML`);
          console.log(`[Nominatim] ${source.name} 返回 HTML，标记失败`);
          handleFailure(source);
          continue;
        }
        const result = {
          lat: lat.toString(),
          lon: lon.toString(),
          display_name: data.trim(),
          address: {},
          error: undefined,
          _source: source.name
        };
        handleSuccess(source);
        console.log(`[Nominatim] ${source.name} 成功 (字符串)`);
        return result;
      }

      if (isPhotonResponse(data)) {
        console.log(`[Nominatim] ${source.name} 检测到 Photon 格式，转换中...`);
        const converted = convertPhotonToNominatim(data, lat, lon);
        if (converted) {
          converted._source = source.name;
          handleSuccess(source);
          console.log(`[Nominatim] ${source.name} 成功 (Photon转换)`);
          return converted;
        } else {
          errors.push(`${source.name}: Photon 转换失败`);
          handleFailure(source);
          continue;
        }
      }

      if (isValidResponse(data)) {
        data._source = source.name;
        handleSuccess(source);
        console.log(`[Nominatim] ${source.name} 成功`);
        return data;
      }

      errors.push(`${source.name}: 无有效数据`);
      console.log(`[Nominatim] ${source.name} 无有效数据，标记失败`);
      handleFailure(source);

    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      errors.push(`${source.name}: ${msg}`);
      console.warn(`[Nominatim] ${source.name} 异常: ${msg}`);
      handleFailure(source);
    }
  }

  throw new Error(`所有上游 Nominatim 服务均不可用: ${errors.join('; ')}`);
}

/**
 * 查询所有上游源，返回所有结果（包括失败的）
 * 用于管理界面，让用户选择要缓存的结果
 */
export async function fetchAllUpstream(lat: number, lon: number): Promise<UpstreamResult[]> {
  await waitUpstreamInterval();

  const results: UpstreamResult[] = [];

  for (const source of sources) {
    checkAndUnban(source);

    const result: UpstreamResult = {
      source: source.name,
      success: false
    };

    try {
      const params = buildParams(lat, lon, source.name);
      const response = await axios.get(source.url, {
        params,
        timeout: 10000,
        headers: {
          'User-Agent': 'PhotoOrdo-Local/1.0 (echoflying@gmail.com)',
          'Referer': 'http://localhost:3000/',
          'Accept-Language': 'zh-CN,zh'
        }
      });

      const status = response.status;
      const data: any = response.data;

      if (status === 400 || status === 429 || status >= 500) {
        result.error = `HTTP ${status}`;
        handleFailure(source);
        results.push(result);
        continue;
      }

      if (typeof data === 'string') {
        if (data.startsWith('<!DOCTYPE') || data.startsWith('<html')) {
          result.error = '返回 HTML';
          handleFailure(source);
          results.push(result);
          continue;
        }
        result.success = true;
        result.display_name = data.trim();
        result.address = {};
        handleSuccess(source);
        results.push(result);
        continue;
      }

      if (isPhotonResponse(data)) {
        const converted = convertPhotonToNominatim(data, lat, lon);
        if (converted) {
          result.success = true;
          result.display_name = converted.display_name;
          result.address = converted.address;
          handleSuccess(source);
        } else {
          result.error = 'Photon 转换失败';
          handleFailure(source);
        }
        results.push(result);
        continue;
      }

      if (isValidResponse(data)) {
        result.success = true;
        result.display_name = data.display_name;
        result.address = data.address;
        handleSuccess(source);
      } else {
        result.error = '无有效数据';
        handleFailure(source);
      }
      results.push(result);

    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      result.error = msg;
      handleFailure(source);
      results.push(result);
    }

    // 每个源之间也加一点间隔
    await new Promise(resolve => setTimeout(resolve, 500));
  }

  return results;
}


/**
 * 上游查询结果类型
 */
export interface UpstreamResult {
  source: string;
  success: boolean;
  display_name?: string;
  address?: Record<string, any>;
  error?: string;
}

/**
 * 从响应中提取地点类型
 */
function extractPlaceType(address: Record<string, any>, type?: string): string {
  if (address.tourism === 'temple' || address.amenity === 'place_of_worship') return 'temple';
  if (address.natural === 'peak' || address.natural === 'volcano') return 'peak';
  if (address.road || address.pedestrian) return 'street';
  if (address.city || address.town || address.village) return 'city';
  return 'other';
}

/**
 * 解析 Nominatim 响应
 */
export function parseNominatimResponse(data: NominatimResponse): {
  display_name: string;
  place_type: string;
  full_response: string;
} {
  const place_type = extractPlaceType(data.address, data.type);

  return {
    display_name: data.display_name || '未知位置',
    place_type,
    full_response: JSON.stringify(data)
  };
}

/**
 * 获取各源状态（用于调试）
 */
export function getSourceStates(): Record<string, any> {
  const result: Record<string, any> = {};
  for (const s of sources) {
    const state = sourceStates.get(s.name)!;
    result[s.name] = {
      consecutiveFailures: state.consecutiveFailures,
      banLevel: state.banLevel,
      banExpiresAt: state.banExpiresAt,
      available: isSourceAvailable(s)
    };
  }
  return result;
}
