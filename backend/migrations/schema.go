package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var files embed.FS

var migrationName = regexp.MustCompile(`^([0-9]{3})_(?:compat([0-9]{3})_)?[a-z][a-z0-9_]*\.sql$`)

type migration struct {
	name string
	body string
	sum  string
}

func localMigrations() ([]migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	local := make([]migration, 0, len(entries))
	for i, entry := range entries {
		name := entry.Name()
		version, _, err := migrationVersion(name)
		if err != nil || version != i+1 {
			return nil, fmt.Errorf("迁移文件命名或编号不连续: %s", name)
		}
		body, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		local = append(local, migration{name: name, body: string(body), sum: checksum(body)})
	}
	return local, nil
}

func migrationVersion(name string) (version, minimum int, err error) {
	match := migrationName.FindStringSubmatch(name)
	if match == nil {
		return 0, 0, fmt.Errorf("迁移文件名无效: %s", name)
	}
	version, _ = strconv.Atoi(match[1])
	if match[2] != "" {
		minimum, _ = strconv.Atoi(match[2])
		if minimum < 1 || minimum >= version {
			return 0, 0, fmt.Errorf("迁移兼容版本无效: %s", name)
		}
	}
	return version, minimum, nil
}

func (m migration) pending() string { return "!" + m.sum[1:] }

func Apply(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('nc-migrations:', MD5(DATABASE())), 10)").Scan(&acquired); err != nil {
		return err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return errors.New("无法取得数据库迁移锁")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, "SELECT RELEASE_LOCK(CONCAT('nc-migrations:', MD5(DATABASE())))")
	}()
	_, err = conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (name VARCHAR(128) PRIMARY KEY, checksum CHAR(64) NOT NULL) ENGINE=InnoDB")
	if err != nil {
		return err
	}
	local, err := localMigrations()
	if err != nil {
		return err
	}
	installed, err := readLedger(ctx, conn)
	if err != nil {
		return err
	}
	if err := checkLedger(local, installed, true); err != nil {
		return err
	}
	for _, m := range local {
		switch installed[m.name] {
		case m.sum:
			continue
		case "":
			// MariaDB DDL 会隐式提交，先登记执行中的状态，阻止旧程序启动。
			if _, err := conn.ExecContext(ctx, "INSERT INTO schema_migrations (name,checksum) VALUES (?,?)", m.name, m.pending()); err != nil {
				return err
			}
		}
		// 这些版本文件仅包含幂等 DDL 和初始数据语句。MariaDB DDL 会隐式提交。
		for statement := range strings.SplitSeq(m.body, ";") {
			if statement = strings.TrimSpace(statement); statement == "" {
				continue
			}
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("应用迁移 %s: %w", m.name, err)
			}
		}
		if _, err := conn.ExecContext(ctx, "UPDATE schema_migrations SET checksum=? WHERE name=? AND checksum=?", m.sum, m.name, m.pending()); err != nil {
			return err
		}
	}
	return Verify(ctx, conn)
}

type rowQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func Verify(ctx context.Context, db rowQuerier) error {
	local, err := localMigrations()
	if err != nil {
		return err
	}
	installed, err := readLedger(ctx, db)
	if err != nil {
		return err
	}
	return checkLedger(local, installed, false)
}

func readLedger(ctx context.Context, db rowQuerier) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name,checksum FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("数据库结构未就绪，请先执行 nc-migrate schema: %w", err)
	}
	defer rows.Close()
	installed := make(map[string]string)
	for rows.Next() {
		var name, sum string
		if err := rows.Scan(&name, &sum); err != nil {
			return nil, err
		}
		installed[name] = sum
	}
	return installed, rows.Err()
}

func checkLedger(local []migration, installed map[string]string, applying bool) error {
	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	slices.Sort(names)
	for i, name := range names {
		version, minimum, err := migrationVersion(name)
		if err != nil || version != i+1 {
			return fmt.Errorf("已应用迁移的编号不连续: %s", name)
		}
		sum := installed[name]
		if version <= len(local) {
			expected := local[version-1]
			if name == expected.name && (sum == expected.sum ||
				(applying && i == len(names)-1 && sum == expected.pending())) {
				continue
			}
			return fmt.Errorf("迁移未完成或与当前程序不一致: %s", name)
		}
		if strings.HasPrefix(sum, "!") || minimum == 0 || minimum > len(local) {
			return fmt.Errorf("数据库有未完成或不兼容的新迁移: %s", name)
		}
	}
	if !applying && len(names) < len(local) {
		return errors.New("数据库结构未就绪，请先执行 nc-migrate schema")
	}
	return nil
}

func checksum(body []byte) string {
	canonical := strings.ReplaceAll(string(body), "\r\n", "\n")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
}
