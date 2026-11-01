package store_test

import (
	"testing"

	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func exec(t *testing.T, db *store.DB, q string, args ...any) error {
	t.Helper()
	_, err := db.SQL.Exec(db.Dialect.Rebind(q), args...)
	return err
}

func mustExec(t *testing.T, db *store.DB, q string, args ...any) {
	t.Helper()
	if err := exec(t, db, q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func addTenant(t *testing.T, db *store.DB, id string) {
	t.Helper()
	mustExec(t, db, "INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)", id, id, 1)
}

const insertFeedback = "INSERT INTO feedback (id, tenant_id, channel_id, message, idempotency_key, received_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)"

func TestIdempotencyKeyUnique(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		addTenant(t, db, "tn_a")
		addTenant(t, db, "tn_b")
		mustExec(t, db, insertFeedback, "fb_1", "tn_a", "ch_x", "hi", "key1", 1, 1)
		if err := exec(t, db, insertFeedback, "fb_2", "tn_a", "ch_x", "hi", "key1", 1, 1); err == nil {
			t.Fatal("duplicate (tenant, key) accepted")
		}
		res, err := db.SQL.Exec(db.Dialect.Rebind(insertFeedback+" ON CONFLICT (tenant_id, idempotency_key) DO NOTHING"),
			"fb_3", "tn_a", "ch_x", "hi", "key1", 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Errorf("rows affected = %d, want 0", n)
		}
		mustExec(t, db, insertFeedback, "fb_4", "tn_b", "ch_x", "hi", "key1", 1, 1)
	})
}

func TestTagAndChannelNamesUniquePerTenant(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		addTenant(t, db, "tn_a")
		addTenant(t, db, "tn_b")
		for _, tbl := range []string{"tags", "channels"} {
			q := "INSERT INTO " + tbl + " (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)"
			mustExec(t, db, q, tbl+"1", "tn_a", "bug", 1)
			if err := exec(t, db, q, tbl+"2", "tn_a", "bug", 1); err == nil {
				t.Errorf("%s: duplicate name in one tenant accepted", tbl)
			}
			mustExec(t, db, q, tbl+"3", "tn_b", "bug", 1)
		}
	})
}

func TestTenantDeleteCascades(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		addTenant(t, db, "tn_a")
		mustExec(t, db, "INSERT INTO channels (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)", "ch_1", "tn_a", "Default", 1)
		mustExec(t, db, "INSERT INTO users (id, tenant_id, email, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"us_1", "tn_a", "a@example.com", "h", "admin", 1, 1)
		mustExec(t, db, insertFeedback, "fb_1", "tn_a", "ch_1", "hi", "k", 1, 1)
		mustExec(t, db, "DELETE FROM tenants WHERE id = ?", "tn_a")
		for _, tbl := range []string{"channels", "users", "feedback"} {
			if n := count(t, db, tbl); n != 0 {
				t.Errorf("%s has %d rows after tenant delete", tbl, n)
			}
		}
	})
}

func TestFeedbackChannelHasNoForeignKey(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		addTenant(t, db, "tn_a")
		mustExec(t, db, insertFeedback, "fb_1", "tn_a", "ch_does_not_exist", "hi", "k", 1, 1)
	})
}

func TestTimestampsAreBigint(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		addTenant(t, db, "tn_a")
		const ts int64 = 1 << 40
		mustExec(t, db, insertFeedback, "fb_1", "tn_a", "ch_1", "hi", "k", ts, ts)
		var got int64
		if err := db.SQL.QueryRow("SELECT received_at FROM feedback WHERE id = 'fb_1'").Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != ts {
			t.Errorf("received_at = %d, want %d", got, ts)
		}
	})
}
