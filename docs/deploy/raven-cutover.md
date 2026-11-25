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
