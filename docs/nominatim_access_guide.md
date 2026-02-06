# Nominatim 上游源访问说明

本文档记录各 Nominatim 上游源的访问注意事项、参数格式差异和响应格式转换。

---

## 一、调度算法

本服务实现 **Sequential Round Robin + 断路器模式 + 指数退避封禁策略**。

### 1.1 轮询调度

- 数据源按固定序列 `A → B → C` 轮询
- 使用轮询指针 `pollingPointer` 计算候选源
- 单请求最多遍历 **3 次**，强制终止避免死循环

### 1.2 状态管理（单机本地存储）

为每个数据源维护以下状态：

| 状态 | 说明 |
|------|------|
| 连续失败计数 | 记录源连续调用失败的次数 |
| 封禁等级 | 数字 0-5，用于计算封禁时长 |
| 封禁截止时间戳 | 记录当前封禁周期的结束时间 |

---

## 二、上游源列表

| 优先级 | 名称 | 地址 | 说明 |
|--------|------|------|------|
| 1 | mirror-earth | `https://api.mirror-earth.com/nominatim/reverse` | 国内访问稳定 |
| 2 | photon | `https://photon.komoot.io/reverse` | 支持 CORS |
| 3 | osm | `https://nominatim.openstreetmap.org/reverse` | 官方服务 |

---

## 三、失败判定规则

满足以下任一条件，标记本次调用**失败**：

| 条件 | 说明 |
|------|------|
| 网络异常 | 请求超时、连接拒绝、域名解析失败 |
| HTTP 错误码 | 400、429、5xx 系列 |
| HTML 响应 | 返回内容以 `<!DOCTYPE` 或 `<html` 开头 |
| 无有效数据 | 响应成功但无地址结果 |

---

## 四、封禁策略

### 4.1 触发条件

- 连续失败次数达到 **3 次** 触发封禁

### 4.2 指数退避

封禁时长采用指数递增规则：

| 封禁等级 | 时长 |
|----------|------|
| 0 | 0 分钟（未封禁） |
| 1 | 1 分钟 |
| 2 | 2 分钟 |
| 3 | 4 分钟 |
| 4 | 8 分钟 |
| 5 | 16 分钟 |

### 4.3 成功重置

源调用成功后**立即重置**：
- 连续失败计数 = 0
- 封禁等级 = 0
- 封禁截止时间戳 = 0

---

## 五、HTTP 请求头要求

根据 OSM Nominatim Usage Policy，所有向上游的请求必须包含：

```http
User-Agent: PhotoOrdo-Local/1.0 (echoflying@gmail.com)
Referer: http://localhost:3000/
Accept-Language: zh-CN,zh
```

---

## 六、参数格式差异

| 参数 | Nominatim/mirror-earth | Photon |
|------|----------------------|--------|
| lat | ✅ | ✅ |
| lon | ✅ | ✅ |
| format | ✅ | ❌ |
| addressdetails | ✅ | ❌ |
| extratags | ✅ | ❌ |
| accept_language | ✅ | ❌ |
| zoom | ✅ | ❌ |

> **注意:** 向 Photon 传递不支持的参数会触发 400 Bad Request。

---

## 七、响应格式差异

### 7.1 Nominatim 格式

```json
{
  "lat": "30.2874600",
  "lon": "120.1614500",
  "display_name": "灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国",
  "address": { "city": "杭州市", "state": "浙江省" }
}
```

### 7.2 Photon 格式

```json
{
  "type": "FeatureCollection",
  "features": [{
    "properties": {
      "name": "灵隐寺",
      "city": "杭州市",
      "state": "浙江省"
    }
  }]
}
```

### 7.3 自动转换

代码自动将 Photon 响应转换为 Nominatim 格式。

---

## 八、频率限制

- **OSM 官方限制**: 1 次/秒
- **本服务实现**: 1.5 秒间隔（略高于官方要求）

> **说明:** 外部调用 Nominatim Cache 不限速，只有向上游发起请求时才触发限速。

---

## 九、执行流程

```
1. 检查上游间隔 (1.5秒)
2. 通过轮询指针计算候选源
3. 检查候选源是否可用（封禁/到期）
4. 执行调用
5. 成功 → 重置该源所有状态
6. 失败 → 连续失败+1，达到3次触发封禁
7. 遍历次数+1，达到3次终止
8. 遍历用尽 → 返回错误
```

---

## 十、故障处理

### 10.1 被 Block 检测

检测到以下情况时标记失败：

- HTTP 400/429/5xx
- 返回 HTML 页面
- 无有效地址数据

### 10.2 降级示例

```
初始状态:
1. mirror-earth.com (正常)
2. photon.komoot.io (正常)
3. openstreetmap.org (正常)

mirror-earth 连续失败3次后:
1. photon.komoot.io
2. openstreetmap.org
3. mirror-earth.com (封禁等级1，1分钟后恢复)
```

---

## 十一、合规性检查清单

- [x] User-Agent 包含应用名称和联系方式
- [x] 设置 Referer 请求头
- [x] 上游 API 调用间隔 ≥ 1.5 秒
- [x] 各上游使用兼容的参数格式
- [x] Photon 响应自动转换为 Nominatim 格式
- [x] 连续失败 3 次触发封禁
- [x] 指数退避 (1→2→4→8 分钟)
- [x] 单请求最多遍历 3 次

---

## 十二、相关链接

- [OSM Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)
- [Nominatim 官方文档](https://nominatim.org/release-docs/latest/)
- [Photon 文档](https://photon.komoot.io/)
