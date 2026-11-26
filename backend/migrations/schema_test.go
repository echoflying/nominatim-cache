package migrations

import (
	"maps"
	"strings"
	"testing"
)

func TestRollbackAllowsOnlyCompletedCompatibleMigrations(t *testing.T) {
	local, err := localMigrations()
	if err != nil {
		t.Fatal(err)
	}
	base := make(map[string]string)
	for _, m := range local {
		base[m.name] = m.sum
	}
	previous := local[:2]
	previousLedger := map[string]string{local[0].name: local[0].sum, local[1].name: local[1].sum}
	withResponses := maps.Clone(previousLedger)
	withResponses[local[2].name] = local[2].sum
	if err := checkLedger(previous, withResponses, false); err != nil {
		t.Fatalf("previous release cannot read the completed response migration: %v", err)
	}
	sum := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, checksum string
		allowed        bool
	}{
		{"003_compat002_add_index.sql", sum, true},
		{"003_replace_table.sql", sum, false},
		{"003_compat002_add_index.sql", "!" + sum[1:], false},
		{"004_compat003_add_index.sql", sum, false},
		{"004_compat002_add_index.sql", sum, false},
	} {
		t.Run(tc.name+"/"+tc.checksum[:1], func(t *testing.T) {
			installed := maps.Clone(previousLedger)
			installed[tc.name] = tc.checksum
			if err := checkLedger(previous, installed, false); (err == nil) != tc.allowed {
				t.Fatalf("checked %s, expected allowed=%t, got %v", tc.name, tc.allowed, err)
			}
		})
	}
	last := local[len(local)-1]
	base[last.name] = last.pending()
	if err := checkLedger(local, base, false); err == nil {
		t.Error("partially applied known migration was accepted on startup")
	}
	if err := checkLedger(local, base, true); err != nil {
		t.Fatalf("interrupted migration cannot be resumed: %v", err)
	}
}
