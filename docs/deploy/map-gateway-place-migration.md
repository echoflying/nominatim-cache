# Nominatim Cache 地点反查迁移说明

2026-11-25。此说明取代此前经 MapGateway 中转的方案。各项目在自己的仓库实现和验收，客户端由用户自行安装或更新。Raven 已将 2,122 条旧站缓存导入 Nominatim Cache 的 MariaDB。Nginx 直接向独立服务分流，MapGateway 继续提供天地图和瓦片。[Raven 部署执行单](raven-cutover.md)

## 客户端可依赖的接口

| 用途 | 公网接口 | Raven 本机后端 |
| --- | --- | --- |
| GPS 坐标反查地名 | `GET https://x.zaitu.cn/nominatim/reverse?lat=30.28746&lon=120.16145` | `GET http://127.0.0.1:18197/nominatim/reverse?lat=…&lon=…` |
| 地点服务就绪 | `GET https://x.zaitu.cn/nominatim/health` | `GET http://127.0.0.1:18197/nominatim/health` |
| 接口指南 | `GET https://x.zaitu.cn/nominatim/help` | `GET http://127.0.0.1:18197/nominatim/help` |
| 完整行政区划 | `GET https://x.zaitu.cn/map-gateway/geocode/reverse?lat=…&lon=…` | 现有天地图路径，供 Trip Craft 等业务使用 |

地点反查只使用十进制 `lat`、`lon`，范围分别为 `[-90,90]`、`[-180,180]`。三条 Nominatim 路径均为免登录只读，支持浏览器跨域和预检。Nginx 对地点查询单独按 IP 限流。批量和管理接口通过受限入口访问。

| 结果 | 客户端处理 |
| --- | --- |
| `200`，`place_quality` 为 `precise` 或 `regional` 且 `place_name` 非空 | 用 `place_name` 做照片分组和短地名展示。需要完整地址的独立详情可展示 `display_name`。保留 `lat`、`lon`、`address`、`place_level` 等现有字段。 |
| `200`，`place_quality` 为 `coarse` 或 `none`，或 `place_name` 为空 | 作为无合适短名处理。地点留空或使用项目现有的无地点状态，禁止用长地址或 Android `Geocoder` 回填短名。不要把瞬时错误与这种正常结果混在一起。 |
| `400`，JSON 包含 `error` | 请求参数有误，修正后再发。 |
| `429`（Nginx 限流）、`503`（NC 冷查询忙或不可用） | 尊重 `Retry-After`，冷查询忙为 2 秒。有需要才做有界重试，前台取消后停止。 |
| `500`（NC 查询失败）、`502`/`504`（Nginx 转发失败/超时） | 按暂时失败处理，先看 HTTP 状态码。Go 接口错误体为 `{"error": "…"}`。保留已有可用缓存。 |

就绪接口使用 JSON。HTTP 200 时要求 `status: "ok"`、`database_ready: true`，同时包含 `version` 和 `commit`。数据库不可用返回 HTTP 503，`status: "unavailable"`、`database_ready: false`。诊断请求使用 `Cache-Control: no-store`，应用正常反查无需先发健康请求。

Nginx 反查读取时限为 12 秒，客户端总请求时限建议 15 秒。就绪探测分别为 4 秒和 5 秒。上游限速由 NC 调度器执行，客户端合并重复查询、保留短地名缓存，前台预取初期单并发，并按繁忙响应退避。

## 各项目实施单

| 项目 | 本次交付 |
| --- | --- |
| **Nominatim Cache / Raven** | 在独立 Go 服务提供 `/reverse`、`/health` 和 `/help`。Nginx 公网只转发这三条接口，处理入口限流。验收已有 MapGateway 地图服务及 Nominatim 的批量／后台隔离。 |
| **Altview / 别览** | 修改 `android/app/src/main/kotlin/cn/zaitu/altview/ElsewherePlaceLookup.kt` 及调用链，替换旧 URL，总请求预算改为 15 秒，沿用请求取消与坐标去重。落实短名/长名区别，清理仍参与地点分组的 Android `Geocoder` 回退。按下节添加更新门禁，继续使用原有应用标识、签名和递增 `versionCode`。 |
| **Elsewhere / 路过** | 修改 `mobile/lib/share_place_resolver.dart`；检查照片按日/地点分组和分享的调用、缓存与空名分支。复用已有 `app-releases/channels/{channel}/manifest.json` 和发行后台承载升级策略，按通道验证开关。 |
| **TravelOdy** | 修改 `src/services/nominatim.ts`、`VITE_NOMINATIM_BASE_URL`、`AppShell.vue` 及配置说明。反查和诊断探针使用两个完整 URL，或用 `reverse`、`health` 相对 `/nominatim/` 解析，避免路径前导 `/` 丢掉前缀。同步检查设置项读取和历史配置迁移、照片刷新，以及实际交付的 Web/Android 版本门禁。 |
| **PhotoOrdo** | 当前可选缓存指向 Mac 上的服务，纳入统一入口一并迁移。修改 `VITE_NOMINATIM_LOCAL_URL`、`src/utils/nominatimService.ts` 和配置样例。新通道先读 `place_name` 与质量。验证过粗和失败不会通过其他回退写入错误短名，并为实际交付的形态接入升级门禁。 |
| **Trip Craft** | `web/src/api.ts` 继续使用现有 `/map-gateway/geocode/reverse`，验收 `administrative` 字段和原有地图功能。这次只实施 Web 更新门禁和做地图回归。 |

RaceHub 和 xPoint 不接这次的地点接口切换；它们后续发版同样采用以下升级约定。

## 所有 App 的升级约定

每个 App 复用自己的升级清单或配置服务。由维护者按 App、平台、发布通道设置远程策略，默认 `off`。没有现成清单时提供轻量 HTTPS 策略入口，升级配置和下载地址在登录前可用。策略独立于地图服务，Web 应可取到最新策略，静态壳和 Service Worker 也要测试更新路径。

```json
{
  "mode": "suggest",
  "min_supported_build": 15,
  "recommended_build": 16,
  "update_url": "https://downloads.example.invalid/app/android/beta",
  "message": "请升级以继续使用地点分组。"
}
```

| `mode` | App 行为 |
| --- | --- |
| `off` | 允许正常进入。已显示的必升拦截在读取新策略后解除。 |
| `suggest` | 当前构建号低于 `recommended_build` 时提示，有「更新」和「稍后」。同一推荐版本稍后后避免每次启动反复提示。 |
| `required` | 当前构建号低于 `min_supported_build` 时拦截业务界面和业务调用，包括深链接进入；仍能进入升级页、打开下载链接和重新检查。已达最低版本的客户端可继续使用。 |

版本比较用整数构建号，Android 使用 `versionCode`，Web 或桌面同一通道使用单调递增的发布构建号。验证 `0 <= min_supported_build <= recommended_build`，校验当前平台和通道的 HTTPS 下载地址。启动、恢复前台时限时刷新，配置取不到或无效时沿用最近有效策略。已收到的 `required` 离线也继续拦截，直到安装达标构建或取得新的有效策略。首次离线且无已知策略时按默认 `off`，恢复连接后立即检查。需要同时停用历史版本的在线业务时，由各自后端对受影响的接口执行同一门槛，可返回 `426`、`code: "upgrade_required"`。已经安装且不认识升级策略的历史构建不会弹窗，用户这次先手动覆盖安装。Android 构建为 `arm64-v8a`，沿用原包名与签名。

## 联调与切换顺序

1. Nominatim Cache 在项目内验证路由、CORS、状态码及接口指南。Raven 发布后测试就绪、短名、同点查询、限流和路径隔离，并验证 MapGateway 的既有路由。将公网联调结果交给各客户端。
2. 各客户端用固定样本和目标入口验证地点显示、长短名、失败保留缓存。分别用低于门槛和达到门槛的版本检查 `off`、`suggest`、`required`、断网恢复，以及回切 `off`。各项目提交候选版本、构建号、下载入口和验收结果，安装由用户自行完成。
3. 用户确认已升级并实际跑过地点功能后，停用旧 Node 的公开入口和自启动。测试切换采用一次性升级，可直接退役旧地址，现有 SQLite 快照留作回退。此阶段以 [Raven 部署执行单](raven-cutover.md) 为准。
4. 每次涉及兼容性变更的发版，先提醒用户本次最低构建号、可用下载地址和计划启用 `suggest`/`required` 的时点。下载链接和覆盖安装通过后才提高门槛。需要回退时先把策略切为 `off`，再回退应用或 Gateway 发布。
