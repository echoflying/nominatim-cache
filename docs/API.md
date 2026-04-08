# Nominatim Cache API 接口文档

## 一、接口概览

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/health` | GET | 否 | 健康检查 |
| `/api/reverse` | GET | 否 | 逆地理编码 |
| `/admin/cache/list` | GET | Basic Auth | 缓存列表 |
| `/admin/cache/:key` | GET | Basic Auth | 缓存详情 |
| `/admin/cache/:key` | DELETE | Basic Auth | 删除缓存 |
| `/admin/cache/clear` | POST | Basic Auth | 清空缓存 |
| `/admin/cache/add` | POST | Basic Auth | 手动添加缓存 |
| `/admin/logs` | GET | Basic Auth | 访问日志 |
| `/admin/stats` | GET | Basic Auth | 统计概览 |
| `/admin/stats/daily` | GET | Basic Auth | 每日统计 |
| `/admin/query` | POST | Basic Auth | 查询所有上游 |

## 二、认证方式

所有管理接口使用 HTTP Basic Auth：

```
Authorization: Basic <base64(username:password)>
```

**默认账号：** `admin` / `admin123`

**生成方式：**
```bash
# 方式1: 使用 htpasswd
htpasswd -nb admin admin123

# 方式2: 使用 curl 测试
curl -u admin:admin123 http://localhost:3000/admin/stats
```

---

## 三、接口详情

### 3.1 Nominatim 逆地理编码代理

获取指定坐标的地理信息。

**请求:**
```
GET /api/reverse?lat=30.28746&lon=120.16145&format=jsonv2
```

**参数:**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| lat | float | ✅ | - | 纬度 (-90 到 90) |
| lon | float | ✅ | - | 经度 (-180 到 180) |
| format | string | ❌ | jsonv2 | 响应格式: json, jsonv2 |
| addressdetails | int | ❌ | 1 | 是否返回地址详情 (0 或 1) |
| extratags | int | ❌ | 1 | 是否返回扩展标签 (0 或 1) |
| accept-language | string | ❌ | zh-CN | 响应语言 |
| zoom | int | ❌ | 18 | 缩放级别 (1-18) |

**请求示例:**
```bash
curl "http://localhost:3000/api/reverse?lat=30.28746&lon=120.16145&format=jsonv2&addressdetails=1"
```

**响应示例 (HTTP 200):**
```json
{
  "lat": "30.2874600",
  "lon": "120.1614500",
  "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
  "address": {
    "tourism": "temple",
    "road": "灵隐路",
    "city_district": "西湖区",
    "city": "杭州市",
    "state": "浙江省",
    "country": "中国",
    "country_code": "cn"
  },
  "type": "tourism",
  "class": "amenity"
}
```

> **说明:** 接口完全兼容 Nominatim 官方 API 格式，缓存状态对调用方透明。
```json
{
  "success": false,
  "error": "所有上游 Nominatim 服务均不可用",
  "details": [
    "Error: connect ETIMEDOUT",
    "Error: Service unavailable"
  ]
}
```

---

### 3.2 获取统计信息

获取缓存服务的整体统计信息。

**请求:**
```
GET /admin/stats
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**响应示例:**
```json
{
  "success": true,
  "data": {
    "total_requests": 12583,
    "cache_hits": 11920,
    "cache_misses": 663,
    "hit_rate": 94.7,
    "cache_count": 1247,
    "uptime": 86400,
    "last_updated_at": 1704153600
  }
}
```

**字段说明:**

| 字段 | 类型 | 说明 |
|------|------|------|
| total_requests | int | 总请求次数 |
| cache_hits | int | 缓存命中次数 |
| cache_misses | int | 缓存未命中次数 |
| hit_rate | float | 命中率 (百分比) |
| cache_count | int | 当前缓存条目数 |
| uptime | int | 服务运行时间 (秒) |
| last_updated_at | int | 最后更新时间 (Unix timestamp) |

---

### 3.3 获取缓存列表

分页获取所有缓存条目。

**请求:**
```
GET /admin/cache/list?page=1&limit=20&search=灵隐&type=temple
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**参数:**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| page | int | ❌ | 1 | 页码 |
| limit | int | ❌ | 20 | 每页数量 (最大 100) |
| search | string | ❌ | - | 搜索关键词 (匹配 display_name) |
| type | string | ❌ | - | 过滤类型: temple, peak, city, street, other |
| sort | string | ❌ | accessed | 排序字段: accessed, cached, name |
| order | string | ❌ | desc | 排序方向: asc, desc |

**响应示例:**
```json
{
  "success": true,
  "data": [
    {
      "cache_key": "30.2875_120.1615",
      "lat": 30.2875,
      "lon": 120.1615,
      "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
      "place_type": "temple",
      "first_cached_at": 1704067200,
      "last_accessed_at": 1704153600,
      "access_count": 156
    },
    {
      "cache_key": "31.2304_121.4737",
      "lat": 31.2304,
      "lon": 121.4737,
      "display_name": "外滩, 中山东一路, 黄浦区, 上海市, 上海市, 中国",
      "place_type": "street",
      "first_cached_at": 1703980800,
      "last_accessed_at": 1704120000,
      "access_count": 89
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 1247,
    "total_pages": 63,
    "has_more": true
  }
}
```

---

### 3.4 获取单个缓存详情

获取指定缓存条目的详细信息。

**请求:**
```
GET /admin/cache/30.2875_120.1615
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**响应示例:**
```json
{
  "success": true,
  "data": {
    "cache_key": "30.2875_120.1615",
    "lat": 30.2875,
    "lon": 120.1615,
    "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
    "place_type": "temple",
    "nominatim_response": {
      "lat": "30.2874600",
      "lon": "120.1614500",
      "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
      "address": { ... },
      "extratags": { ... }
    },
    "first_cached_at": 1704067200,
    "last_accessed_at": 1704153600,
    "access_count": 156
  }
}
```

---

### 3.5 删除单个缓存

删除指定的缓存条目。

**请求:**
```
DELETE /admin/cache/30.2875_120.1615
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**响应示例:**
```json
{
  "success": true,
  "message": "缓存已删除",
  "deleted_key": "30.2875_120.1615"
}
```

---

### 3.6 清空所有缓存

清空所有缓存条目（不可恢复）。

**请求:**
```
POST /admin/cache/clear
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**请求体 (可选):**
```json
{
  "confirm": "DELETE ALL"
}
```

**响应示例:**
```json
{
  "success": true,
  "message": "所有缓存已清空",
  "deleted_count": 1247
}
```

---

### 3.7 获取访问日志

获取最近的访问日志。

**请求:**
```
GET /admin/logs?page=1&limit=100&cached=1
Authorization: Basic YWRtaW46YWRtaW4xMjM=
```

**参数:**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| page | int | ❌ | 1 | 页码 |
| limit | int | ❌ | 100 | 每页数量 (最大 1000) |
| cached | int | ❌ | - | 过滤: 0=未命中, 1=命中 |
| from | int | ❌ | - | 开始时间 (Unix timestamp) |
| to | int | ❌ | - | 结束时间 (Unix timestamp) |

**响应示例:**
```json
{
  "success": true,
  "data": [
    {
      "id": 1000001,
      "cache_key": "30.2875_120.1615",
      "lat": 30.2875,
      "lon": 120.1615,
      "cached": 1,
      "accessed_at": 1704153600,
      "accessed_at_str": "2024-01-02 10:00:00"
    },
    {
      "id": 1000002,
      "cache_key": "31.2304_121.4737",
      "lat": 31.2304,
      "lon": 121.4737,
      "cached": 0,
      "accessed_at": 1704153598,
      "accessed_at_str": "2024-01-02 09:59:58"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 100,
    "total": 50000,
    "has_more": true
  }
}
```

---

## 四、错误码说明

| 错误码 | 说明 |
|--------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 401 | 未认证或认证失败 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

**错误响应格式:**
```json
{
  "success": false,
  "error": "错误描述",
  "code": 400
}
```

---

## 五、使用示例

### 5.1 前端调用 (JavaScript)

```javascript
// API 客户端封装 (兼容 Nominatim 官方格式)
class NominatimClient {
  constructor(baseUrl = '') {
    this.baseUrl = baseUrl;
  }

  async reverse(lat, lon, params = {}) {
    const url = new URL(`${this.baseUrl}/api/reverse`);
    url.searchParams.set('lat', lat.toString());
    url.searchParams.set('lon', lon.toString());
    Object.entries(params).forEach(([key, value]) => {
      url.searchParams.set(key, value.toString());
    });

    const response = await fetch(url.toString());
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Request failed');
    }
    return response.json();
  }
}

// 使用示例
const client = new NominatimClient('http://localhost:3000');
const data = await client.reverse(30.28746, 120.16145);
console.log(data.display_name);
console.log(data.address.city);
```

### 5.2 curl 命令测试

```bash
# 逆地理编码
curl "http://localhost:3000/api/reverse?lat=30.28746&lon=120.16145"

# 获取统计 (带认证)
curl -u admin:admin123 "http://localhost:3000/admin/stats"

# 获取缓存列表 (带认证)
curl -u admin:admin123 "http://localhost:3000/admin/cache/list?page=1&limit=10"
```

---

## 六、注意事项

1. **频率限制**: 虽然缓存可以减少上游调用，但仍建议客户端控制请求频率
2. **认证安全**: 生产环境请修改默认账号密码
3. **HTTPS**: 生产环境请配置 HTTPS
4. **日志清理**: 日志表会自动清理 30 天前的数据
