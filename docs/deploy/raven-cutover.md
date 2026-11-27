# Raven 首次部署执行单

2026-11-25 只读盘点确认 `x.zaitu.cn` 指向 Raven，旧 `nc.zaitu.cn` 指向 Zion。Raven 新实例先从空库启动，随后导入用户提供的旧站 SQLite 快照。公开入口、客户端和升级门禁按 [Nominatim Cache 地点反查迁移说明](map-gateway-place-migration.md) 切换。

| 项目 | 部署值 |
| --- | --- |
| 主机 | SSH 别名 `raven`，Rocky Linux x86_64 |
| 地点反查 | `https://x.zaitu.cn/nominatim/reverse` → `127.0.0.1:18197/nominatim/reverse` |
| 健康与指南 | `/nominatim/health`、`/nominatim/help` → `127.0.0.1:18197` 同路径 |
| 管理入口 | `https://x.zaitu.cn/ops/nominatim/`，通过 OPS 授权后转发至 `127.0.0.1:18198` |
| 现有端口 | `8097` 是 Ops Gateway，`8098` 是 TeamVoice |
| MariaDB | 项目独立进程，只监听 Unix socket `/run/nominatim-cache-db/mariadb.sock` |
| 存储 | `/mnt/datadisk0/nominatim-cache/`，分开存软件、数据库和备份 |
| Nginx | 在 `http` 加 `nominatim-rate.conf`，并在 `x.zaitu.cn` HTTPS 虚拟主机加 `nominatim-location.conf` |

部署前复查端口与目录。管理页面使用本机 SSH 隧道。独立 MariaDB 使用 Raven 现有 **11.4.13 软件目录的副本**，由单独的 Unix 账号和数据库配置管理。正式库名为 `nominatim_cache`，演练库使用单独的名称，分开创建账号。

## 1. 从空库预部署

校验 Linux amd64 候选归档和 SHA-256 后，使用 `scripts/install-raven-fresh.sh` 在 Raven 初始化本项目的独立 MariaDB、建空库并启动回环 API 和管理服务。安装命令默认只执行检查，实际初始化需带 `--apply`。归档、版本和摘要取自同一次构建。

```bash
sudo bash install-raven-fresh.sh \
  --archive /private/path/nominatim-cache-v2.0.0-rc.1-linux-amd64.tar.gz \
  --sha256 ad325e65d66c8fb07ee394f8065df1423657f4e2f20e3526204970cd8caddabc \
  --upstream https://api.mirror-earth.com/nominatim/reverse \
  --apply
```

脚本生成独立应用和管理凭据，检查缓存、访问日志与请求计数均为空，只启动回环服务。公共 Nginx 路由、DNS 和开机启动由切换阶段处理。

2026-11-25 21:04（北京时间）已在 Raven 完成这一阶段。`v2.0.0-rc.1`（`22bd565`）与隔离的 MariaDB 11.4.13 均在运行，两个 unit 暂为 disabled。`/nominatim/health` 返回 `database_ready: true`，缓存、访问日志与累计请求均为零。两个监听均为回环地址。管理 API 在公开监听的检查结果为 404，在管理监听的未鉴权检查结果为 401。

2026-11-25 21:21（北京时间）将 `tmp/data.zip` 内的 SQLite 主库和 WAL 合成独立快照，快照完整性检查通过。使用 `nc-migrate` 导入尚为空的 Raven 项目库，源和目标内容摘要一致。MariaDB 实查为缓存 **2,122** 条、访问日志 **9,548** 条、累计请求 **27,819** 次。导入后新版重启并通过回环健康检查。回执存放于 Raven 的 `/mnt/datadisk0/nominatim-cache/backups/legacy-import-31921ff4.json`，公共入口尚未切换。

2026-11-25 23:06（北京时间）发布 `v2.0.0-rc.2`（`128078b`）。Nginx 直接开放 `/nominatim/reverse`、`/nominatim/health` 和 `/nominatim/help`。验证了 HTTPS、首次回源及再次命中、CORS 预检、无效坐标 400、限流 429 和 `Retry-After: 2`、后台及批量路径 404，以及现有 MapGateway 的 `/help`。两个 Nominatim unit 均已启用开机启动。发布前的 Nginx 虚拟主机备份为 `/etc/nginx/snippets/x-zaitu-before-nominatim.PJlhNEe3`。

2026-11-26（北京时间）上线 Nominatim `v2.0.0-rc.3`（`4807423`）和 OPS `0.3.24.945aa94`（Raven 仓库 `codex/nominatim-ops`）。`/ops/nominatim/` 接入 OPS 会话，`nominatim.access` 初始授予现有超级管理员。实测登录后显示项目入口，缓存列表及统计返回 200，无权限返回 403，跨站写入返回 403。临时验证账号和会话已撤销。公网业务前缀 `/nominatim/admin/` 实测为 404。此次发布的 Nginx 虚拟主机备份为 `/etc/nginx/snippets/x-zaitu-before-nominatim-ops.z324pmF5`，OPS 用户配置备份为 `/etc/ops/users-before-nominatim.huw5t31B`，均限 root 读取。

2026-11-26 20:42（北京时间）发布 `v2.0.0-rc.6`（`936b1a8`），先应用兼容迁移 `003_compat002_access_log_responses.sql`，再切换服务。验证了 HTTPS 健康、后台资源、匿名请求详情返回 401，并确认缓存命中的详情与客户端收到的 HTTP 200 正文一致。回退版本 `v2.0.0-rc.5` 保留。

2026-11-26 21:21（北京时间）发布 `v2.0.0-rc.7`（`17fde7f`），应用兼容迁移 `004_compat003_upstream_archive.sql`。上线时没有仍存的 RC.6 上游原文需要补存。验证了 HTTPS 健康、后台静态资源及匿名详情返回 401；缓存命中只保存摘要，14 天统计可读。一条公网缓存未命中请求返回 200，原始上游正文写入独立归档。回退版本 `v2.0.0-rc.6` 保留。

2026-11-27（北京时间）发布 `v2.0.0-rc.9`（`fee0d90cdff5912ad0bdd494ae259fd7c6c17751`），运行 `006_batch_scheduling_and_result_cursor.sql`。迁移前备份为 `/mnt/datadisk0/nominatim-cache/backups/nominatim-cache-before-rc9-20261127-02.sql.gz`，SHA-256 为 `3b846805331f13588ef425624f8b7709dcfed0efa7ee6a8c9ac0b5df1905f0e6`。上线检查发现当前 Nginx 片段尚未包含 Altview 批量路径，因此同步发布 RC.9 包内的 Nginx 路由与限流区；原配置分别保存在 `/etc/nginx/snippets/nominatim.conf.before-rc9-20261127` 和 `/etc/nginx/conf.d/00-nominatim-rate.conf.before-rc9-20261127`。`nginx -t` 通过后重载。公网健康与帮助、单点回源和缓存、Altview 批量提交及 cursor 结果读取均已验证；临时批次已清理。服务版本为 RC.9，数据库就绪，RC.8 保留在上一版本目录。由于迁移 006 未声明与 RC.8 兼容，直接切回 RC.8 会被数据库版本检查拒绝；如需回退，须先停止服务并恢复上述迁移前数据库备份，再切换版本。恢复备份会丢弃备份之后的新写入。

2026-11-27（北京时间）随后发布 `v2.0.0-rc.10`（`1349e2971bf98b52a0e6c209c8e51ea9eec55a4f`）和 `v2.0.0-rc.11`（`4a6ec56c02cb691736dfc59523d0f54bf518d85d`），将 Altview 专用路由替换为所有调用方共用的 `/nominatim/api/reverse/batches`，使用 Bearer 凭据、同一持久化队列和按 IP 限流。Nginx 旧片段备份为 `/etc/nginx/snippets/nominatim.conf.before-rc10-20261127` 与 `/etc/nginx/conf.d/00-nominatim-rate.conf.before-rc10-20261127`，配置校验通过后重载；此次未修改数据库结构。RC.11 健康检查正常，帮助接口列出通用 API 并返回 `batch.enabled=false`。生产 `BATCH_TOKENS` 为空，因此通用批量 API 当前关闭并返回 404；未生成或下发调用方凭据。Altview 专用公网路径已关闭。

2026-11-27（北京时间）发布 `v2.0.0-rc.12`，增加后台调用方凭据创建、列表和撤销，密钥仅创建时返回并只保存 SHA-256 摘要；应用兼容迁移 `007_compat006_batch_credentials.sql`。迁移前备份为 `/mnt/datadisk0/nominatim-cache/backups/nominatim-cache-before-rc12-20261127.sql.gz`。RC.12 随后由 `v2.0.0-rc.13` 替代，补齐概览、缓存和最近请求页的静态资源版本号，使新凭据导航在旧页面缓存下也能更新。RC.14 修正 ASCII 字段中的凭据提示格式。RC.15 增加独立密钥 AES-GCM 加密存储和运维后台按需查看，应用 `008_compat007_recoverable_batch_credentials.sql`。未创建调用方密钥；批量 API 继续关闭。

### 旧站备份方法

以下方法用于从仍在运行的旧版本制作独立快照。若已经拿到含 WAL 的归档，先用临时工作目录合成独立快照，再导入。

先核对 Zion 的反代目标。若 Node 运行在另一台机器，在实际应用主机定位进程、工作目录、容器挂载或 `DATABASE_PATH`。确认正在写入的 SQLite 文件后，用同一个旧站运行账号执行在线备份：

```bash
umask 077
python3 backup-legacy-sqlite.py \
  --source /actual/path/cache.db \
  --output /private/path/nc-rehearsal.tar.gz
```

归档只包含独立 SQLite 文件、SHA-256 和三表行数清单。通过 SSH 取回 Windows 项目下的 `data/migration/<快照编号>/`，核对归档两端的 SHA-256（Linux 用 `sha256sum`，macOS 用 `shasum -a 256`）。解包进新建目录，校验归档内的 `SHA256SUMS`，再用 Linux 发布包内的 `nc-migrate sqlite -snapshot /path/to/cache.db -dry-run` 核对三表内容摘要。Zion 的 SSH 入口及旧 SQLite 实际路径需单独确认。

## 2. 安装独立 MariaDB

以下操作在 Raven 的部署窗口执行。先确认目标账号、目标目录和 unit 名均为本项目。Raven 现有 MariaDB 服务及其目录由当前应用继续使用。

1. 创建无登录权限的系统账号 `nominatim-cache-db` 与 `nominatim-cache`。创建 `/opt/nominatim-cache/releases`、`/etc/nominatim-cache` 及数据盘项目目录。数据库目录归 `nominatim-cache-db` 所有，权限 0750。
2. 把已核查的完整 MariaDB 软件安装目录复制到 `/mnt/datadisk0/nominatim-cache/vendor/mariadb-11.4.13`。以 root 持有软件副本。新进程的 `basedir`、`datadir` 和 socket 均以包内 `mariadb.cnf` 为准。
3. 仅对全新的数据目录执行 MariaDB 初始化：

   ```bash
   sudo /mnt/datadisk0/nominatim-cache/vendor/mariadb-11.4.13/scripts/mariadb-install-db \
     --defaults-file=/etc/nominatim-cache/mariadb.cnf \
     --user=nominatim-cache-db --auth-root-authentication-method=socket \
     --auth-root-socket-user=root --skip-test-db
   ```

4. 安装并启动 `nominatim-cache-db.service`，用该安装自带的 `bin/mariadb` 通过专用 socket 执行 `SELECT VERSION(), @@datadir, @@socket, @@skip_networking`，核对全部值后创建项目库。使用预发账号做 CRUD 验证。生产应用账号只授予项目库的 `SELECT, INSERT, UPDATE, DELETE`，由数据库管理员执行建表和导入。正式环境配置文件归 root 所有，权限 0600。
5. 用 `nc-migrate schema` 初始化正式空库。管理员可使用 `sudo env MIGRATE_DSN='root@unix(/run/nominatim-cache-db/mariadb.sock)/nominatim_cache' /path/to/nc-migrate schema` 连接独立实例。检查缓存、访问日志和请求计数为零。

## 3. 新服务预演与正式切换

1. 上传 RC 归档与外部 SHA-256 文件，依次运行 `sha256sum -c <归档>.sha256`，解包，再在解包目录运行 `sha256sum -c SHA256SUMS`。给每次构建保留独立 `releases/<version>`，配置和数据不随解包覆盖。
2. 先通过回环访问，检查 `/health` 的版本、完整 commit 和数据库就绪状态。发布验收时用一个允许查询的坐标验证首次回源和第二次缓存命中。批量鉴权和冷批量上游在约定的试用阶段再启用。
3. 管理页从 `/ops/nominatim/` 登录 OPS。SSH 备用路径可执行 `ssh -N -L 18198:127.0.0.1:18198 raven`，再打开本机 `http://127.0.0.1:18198/`。公开入口的 `/nominatim/admin/stats` 应返回 404。
4. 发布含 `/reverse` 和 `/help` 的 Go 版本，验收回环探针后加 Nginx 规则。以 `nginx -t` 通过为重载条件。验证三个公网接口、CORS、后台／批量隔离和现有 MapGateway 路由。把已验收入口交给各客户端联调。启用 NC 服务和项目数据库的开机启动，核对两个回环监听。
5. 用户自行更新并确认测试客户端可用后，定位实际运行旧 Node 的主机和旧域名转发配置，停用旧 Node 与其自启动，并撤下旧站转发规则。旧库、归档和程序保留到切换验收结束。若旧站有必须保留的新增数据，停写后先取得最终快照，再决定是否补迁。

## 4. 回退

首次切换前若验收失败，旧客户端可继续使用旧入口。若切换后出现问题，优先把升级门禁切回 `off`，再回退 Nginx 规则或 NC 的兼容版本。若必须临时恢复旧 Node 并保留 Go 期间新增缓存，先停止新版写入和批次提交并单独备份 MariaDB，再用 `nc-migrate export-sqlite -output <新文件>` 生成旧 Node 可读的快照，抽验通过后恢复旧出口及相应客户端。
