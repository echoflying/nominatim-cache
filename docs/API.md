# Nominatim Cache API 接口文档

## 一、接口概览

| 端点 | 方法 | 认证 | 说明 |
|------|------|------|------|
| `/health` | GET | 否 | 健康检查 |
| `/help` | GET | 否 | Go v2 接口指南 |
| `/reverse` | GET | 否 | Go v2 单点反查，沿用 `/api/reverse` 的响应 |
| `/api/reverse` | GET | 否 | 逆地理编码 |
| `/api/reverse/batches` | POST | Bearer | 提交批量反查（Go v2） |
| `/api/reverse/batches/{job_id}` | GET | Bearer | 查看批次状态（Go v2） |
| `/api/reverse/batches/{job_id}/results` | GET | Bearer | 分页读取批次结果（Go v2） |
| `/altview/place-batches` | POST | Altview 公共入口 | 提交 Altview 地点预热批次 |
| `/altview/place-batches/{job_id}` | GET | 随机任务 ID | 查看 Altview 批次状态 |
| `/altview/place-batches/{job_id}/results` | GET | 随机任务 ID | 分页读取 Altview 批次结果 |
| `/admin/cache/list` | GET | Basic Auth | 缓存列表 |
| `/admin/cache/:key` | GET | Basic Auth | 缓存详情 |
| `/admin/cache/:key` | DELETE | Basic Auth | 删除缓存 |
| `/admin/cache/clear` | POST | Basic Auth | 清空缓存 |
| `/admin/cache/add` | POST | Basic Auth | 手动添加缓存 |
| `/admin/logs` | GET | Basic Auth | 访问日志 |
| `/admin/logs/{id}` | GET | 管理认证 | 单条反查请求的上游与最终响应（Go v2） |
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

### Raven 公网入口（Go v2）

| 路径 | 用途 |
| --- | --- |
| `GET /nominatim/reverse?lat=30.28746&lon=120.16145` | 单点反查，返回 `place_name`、`place_quality`、`display_name` 和 `address` |
| `GET /nominatim/health` | 就绪检查。JSON 含 `status`、`version`、`commit`、`database_ready`、`timestamp` 和 `uptime`。数据库不可用返回 503 |
| `GET /nominatim/help` | JSON 接口指南，包含调用示例、字段说明、状态码与重试约定 |

这些路径通过 `https://x.zaitu.cn` 上的 Nginx 访问 Nominatim Cache。单点接口支持 CORS 和 `OPTIONS` 预检，并按 IP 限流，429 附 `Retry-After`。通用批量 API 仅在授权环境访问；Altview 专用批量入口单独限流，使用共享的 `altview-mobile` 队列调用方和不可预测的任务 ID，不接受调用方传入坐标以外的上游参数。Raven 的管理页面和同源管理接口通过 `/ops/nominatim/` 提供，由 OPS 校验会话及 `nominatim.access`。独立管理监听仍可通过 SSH 隧道使用 Basic Auth。

### 批量逆地理编码（Go v2）

使用 `Authorization: Bearer <token>`。提交请求需要 `Idempotency-Key`（1 至 128 字节）。同一个调用方、同一个键且坐标列表相同时返回已有任务。键相同但请求内容不同时返回 409。

调度按请求规模划分优先级：单点反查优先级最高；去重后少于 10 个坐标的批次为小批量优先级；其他批次按普通队列处理。所有类别仍共用每个上游的并发闸门和请求间隔。等待较久的批量任务会逐步提升优先级，避免持续的小请求使大任务一直等待。优先级只影响等待顺序，不增加上游速率。

提交示例为 `POST /api/reverse/batches`，请求体为 `{"points":[{"lat":30.28746,"lon":120.16145}]}`，默认接受 1 至 500 个点。返回 202 和 `Location` 头，正文包括 `job_id`、`status`、`total`、`pending`、`completed`、`failed`、`status_url` 和 `results_url`。状态为 `pending`、`running`、`completed` 或 `completed_with_errors`。待处理额度不足返回 429，并提供 `Retry-After`。

用状态接口轮询，建议两次查询间隔至少 2 秒。兼容分页方式接受 `page`、`limit`（默认 100，最大 500），按提交顺序返回 `items`，每项含从 0 开始的 `index`、`lat`、`lon` 及 `status`。已完成项携带 `result`。暂时失败项可能携带 `error`、`retry_at`，错误结束项携带 `error`。结果状态为 `cached`、`resolved`、`pending`、`running`、`no_result` 或 `failed`。

增量结果方式使用 `cursor`，可选 `limit` 为 1 至 50，默认 50，可选 `wait` 为 0 至 10 秒。服务最多等待 `wait` 秒，并返回这段时间内已完成的 `items`、`next_cursor` 和 `has_more`。首次传 `cursor=0`，随后原样传回响应里的 `next_cursor`；游标是服务端的递增位置标记，不是已收结果条数。每条已完成结果只需读取一次。增量结果按完成顺序返回，`index` 保留原提交位置。完成任务保留 7 天。过期或属于其他调用方的任务返回 404。

批量查询优先复用缓存。未配置获准批量查询的上游时，未命中的点标记 `batch_source_unavailable`，其他点仍返回已有结果。批量接口的启用与上游配置见 [部署说明](DEPLOYMENT.md)。

### Altview 地点批量预热

`POST /nominatim/altview/place-batches` 接受与通用批量接口相同的 `{"points":[{"lat":30.28746,"lon":120.16145}]}` JSON，但要求 `Idempotency-Key`，无需把服务端 Bearer 凭据放进 APK。每批最多 `BATCH_LIMIT` 个点，使用共享 Altview 调用方的 `BATCH_PENDING_LIMIT` 待处理上限；任务 ID 为随机 128 位值。状态和结果路径分别为 `/nominatim/altview/place-batches/{job_id}` 与 `/nominatim/altview/place-batches/{job_id}/results`，返回字段和状态语义与通用批量接口一致。少于 10 个去重坐标的小批量会优先于普通批量任务。单点反查始终拥有更高的上游调度优先级。Raven 对提交入口单独按 IP 限速。

Altview 结果接口同时支持 `page` 分页和 `cursor` 增量读取。新客户端建议每次请求 `limit=50`、`wait=8`，处理响应后保存 `next_cursor` 并继续请求，直到 `has_more=false` 且状态任务已完成。增量模式只返回新完成的条目，不重复发送先前已读结果。

**游标读取示例**
```http
GET /nominatim/altview/place-batches/0123456789abcdef0123456789abcdef/results?cursor=0&limit=50&wait=8
```
```json
{
  "job_id": "0123456789abcdef0123456789abcdef",
  "status": "running",
  "total": 120,
  "pending": 70,
  "completed": 50,
  "failed": 0,
  "cursor": 0,
  "next_cursor": 50,
  "has_more": false,
  "items": [
    {"index": 7, "lat": 30.28746, "lon": 120.16145, "status": "resolved", "result": {"place_name": "灵隐寺", "place_quality": "precise"}}
  ]
}
```
`items` 按完成顺序返回，`index` 对应原提交列表中的位置。示例只展示一项，实际一包最多 50 项。没有新完成项时，服务最多等待 `wait` 秒后返回空 `items`，客户端应沿用原 `cursor` 重试。

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
  "place_name": "灵隐寺",
  "place_level": "poi",
  "place_quality": "precise",
  "place_source_field": "name",
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

> **说明:** 接口接受 `lat` 和 `lon`，返回 Nominatim 风格的数据。其他请求参数被忽略，上游格式与语言由服务设置。缓存命中附带 `cached: true` 和 `cache_stats`。
> 服务额外返回 `place_name`、`place_level`、`place_quality` 与 `place_source_field`。调用方展示地点时优先使用 `place_name`。只有省级地址或由多层行政区划拼成的宽泛长地址会标为 `coarse`，`place_name` 为空；服务日志记录不带坐标和地址内容的质量告警，原始 `display_name` 和完整响应继续留在已有管理缓存中。质量等级为 `precise`、`regional`、`coarse` 或 `none`。

管理端 `/admin/cache/list` 同时返回相同的地点名称、精度等级和来源字段，缓存列表显示“地址过粗”或“无有效名称”标记，便于发现历史及新入库的低质量结果。上游全失败时返回 HTTP 500，响应只包含 `error`。未命中查询达到并发上限时立即返回 HTTP 503，带 `Retry-After: 2`，缓存命中照常返回。

如果旧域名 `nc.zaitu.cn` 配置为直接反代 Go 同步接口，保留 `Host` 后默认使用 6.5 秒的查询预算。新客户端使用 Raven 公网入口，批量查询通过任务接口处理。
```json
{
  "error": "所有上游 Nominatim 服务均不可用: <原因>"
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
| sort | string | ❌ | accessed | 排序字段: accessed (最近访问), cached (首次缓存) |
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
      "address": {
        "road": "灵隐路",
        "city_district": "西湖区",
        "city": "杭州市",
        "state": "浙江省",
        "country": "中国"
      },
      "place_type": "temple",
      "source": "mirror-earth.com",
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
缓存清单中的“详情”按钮展示该条目的地点名称判断字段、缓存元信息和保存的原始响应；其中 `place_name` 与 `place_source_field` 用于说明短地点名及其选取来源。

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

Go v2 的每条摘要包含 `source`。回源时是本次采用的上游名称，缓存命中时是请求当时记录的缓存来源，人工录入为 `manual`。无可用来源时返回 `null`。

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
      "source": "openstreetmap.org",
      "accessed_at": 1704153600,
      "accessed_at_str": "2024-01-02 10:00:00"
    },
    {
      "id": 1000002,
      "cache_key": "31.2304_121.4737",
      "lat": 31.2304,
      "lon": 121.4737,
      "cached": 0,
      "source": "photon.komoot.io",
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

### 3.8 获取单条反查请求详情（Go v2）

`GET /admin/logs/{id}` 需要管理权限。在 Raven 通过 `/ops/nominatim/admin/logs/{id}` 访问，请求 ID 来自日志列表。列表仍只返回摘要。

```json
{
  "success": true,
  "data": {
    "upstream_source": "openstreetmap.org",
    "upstream_body": "{\"display_name\":\"浙江省杭州市西湖区\",\"address\":{\"city\":\"杭州市\"}}",
    "cache_original_source": null,
    "cache_original_body": null,
    "cache_original_kind": null,
    "response_status": 200,
    "response_body": "{\"display_name\":\"浙江省杭州市西湖区\",\"address\":{\"city\":\"杭州市\"},\"place_name\":\"杭州市\"}\n"
  }
}
```

正文均以字符串传递，保留原文格式及数字精度。`upstream_body` 是本次查询最终采用的上游正文。缓存命中时它为 `null`，通过 `cache_original_source` 和 `cache_original_body` 查看本次命中的缓存来源。`cache_original_kind` 为 `upstream` 时正文来自填充该缓存版本的上游归档；为 `cache_snapshot` 时是当时的入库数据，包括旧缓存已转换的内容和人工录入。缓存后续修改或删除不会改变历史请求的详情。

新写入日志的 `response_body` 是接口本次实际发送的正文，包括缓存命中和没有采用上游结果的请求。历史记录中未保存的字段返回 `null`。详情适用于已写入访问日志的同步反查，入口拒绝、非法坐标和并发繁忙的提前返回不在此日志内。不存在或已过期的日志返回 404，非法 ID 返回 400。

Go v2 的 `GET /admin/stats/daily` 可传 `days`（1–366，默认 7）。每日统计由仍保留的访问日志与已清理日志的汇总合并，使用服务配置的统计时区。

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
