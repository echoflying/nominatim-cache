# Raven 首次部署执行单

2026-11-25 只读盘点确认 `x.zaitu.cn` 指向 Raven，旧 `nc.zaitu.cn` 指向 Zion。上线采用两个入口共用一套数据，先做离线演练，再在切换窗口导入最终快照。

| 项目 | 部署值 |
| --- | --- |
| 主机 | SSH 别名 `raven`，Rocky Linux x86_64 |
| 公开 API | `127.0.0.1:18197` → `https://x.zaitu.cn/nominatim/` |
| 管理入口 | `127.0.0.1:18198`，通过 SSH 转发访问 |
| 现有端口 | `8097` 是 Ops Gateway，`8098` 是 TeamVoice |
| MariaDB | 项目独立进程，只监听 Unix socket `/run/nominatim-cache-db/mariadb.sock` |
| 存储 | `/mnt/datadisk0/nominatim-cache/`，分开存软件、数据库和备份 |
| Nginx | 在现有 `/etc/nginx/conf.d/x-zaitu.conf` 的 HTTPS server 内引入路由片段 |

部署前复查端口与目录。管理页面使用本机 SSH 隧道。独立 MariaDB 使用 Raven 现有 **11.4.13 软件目录的副本**，由单独的 Unix 账号和数据库配置管理。正式库名为 `nominatim_cache`，演练库使用单独的名称，分开创建账号。

## 1. 旧站备份与演练

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
5. 在演练库运行 `nc-migrate schema` 和 `nc-migrate sqlite -snapshot ...`，保存回执。管理员用 `sudo env MIGRATE_DSN='root@unix(/run/nominatim-cache-db/mariadb.sock)/nominatim_cache_rehearsal' /path/to/nc-migrate schema` 连接独立实例，导入使用同一 DSN。比较 SQLite 归档、导入回执及 MariaDB 三表行数和内容摘要。再用 `export-sqlite` 导出一个新文件，让旧 Node 验证能读取结果。

## 3. 新服务预演与正式切换

1. 上传 RC 归档与外部 SHA-256 文件，依次运行 `sha256sum -c <归档>.sha256`，解包，再在解包目录运行 `sha256sum -c SHA256SUMS`。给每次构建保留独立 `releases/<version>`，配置和数据不随解包覆盖。
2. 演练时将服务配置指向演练库，先通过回环访问。生产填写独立管理员密码、MariaDB 应用凭据和已确认可用的交互上游。批量鉴权和冷批量上游在约定的试用阶段再启用。检查 `/health` 的版本、完整 commit 和数据库就绪状态。用导入坐标验证缓存命中，再用一个允许查询的未缓存坐标验证回源。
3. 管理页通过 `ssh -N -L 18198:127.0.0.1:18198 raven`，访问本机 `http://127.0.0.1:18198/`。公开入口的 `/nominatim/admin/stats` 应返回 404。
4. 正式切换时先确认旧域名 TLS 和代理方案。暂停旧入口新请求，等在途处理完成后停旧 Node，再打最终 SQLite 包。将最终包导入全新正式库并验收。切换 `current`，以正式配置启动服务，再加 Nginx include。先跑 `nginx -t`，通过后重载。
5. 让旧域名直接转发到新版，保留原请求路径和查询参数。若 DNS 迁到 Raven，先准备旧域名证书和独立虚拟主机。DNS 传播期间，Zion 只转发到新服务，避免旧缓存库重新接收写入。用真实 Altview 测一次旧域名，确认状态码、短地名和 8 秒读取窗口。验收通过后再退役旧 Node。

## 4. 回退

首次切换前若导入或验收失败，旧入口和旧库继续作为生产基线。首次切换后需要回退时，先停止新版写入和批次提交，单独备份 MariaDB，再用 `nc-migrate export-sqlite -output <新文件>` 生成旧 Node 可读的快照。用旧程序抽验回切后再转流量。保留旧原库和 MariaDB 备份，批次状态仍由 MariaDB 保存。
