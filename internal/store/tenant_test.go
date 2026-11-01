package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func TestCreateTenantAndChannel(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		d := db.Dialect

		if _, err := store.FirstTenant(ctx, db.SQL, d); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("FirstTenant empty err = %v", err)
		}
		if n, err := store.CountTenants(ctx, db.SQL, d); err != nil || n != 0 {
			t.Fatalf("count = %d %v", n, err)
		}
		a, err := store.CreateTenant(ctx, db.SQL, d, "older", 1, 100)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(a.ID, "tn_") {
			t.Errorf("id = %q", a.ID)
		}
		got, err := store.GetTenant(ctx, db.SQL, d, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got != a || got.PlanID != nil || got.BillingMonthStart != 1 {
			t.Errorf("got %+v want %+v", got, a)
		}
		if n, _ := store.CountTenants(ctx, db.SQL, d); n != 1 {
			t.Errorf("count = %d", n)
		}
		if _, err := store.CreateTenant(ctx, db.SQL, d, "newer", 15, 200); err != nil {
			t.Fatal(err)
		}
		first, err := store.FirstTenant(ctx, db.SQL, d)
		if err != nil || first.ID != a.ID {
			t.Errorf("first = %+v err=%v", first, err)
		}
		if _, err := store.GetTenant(ctx, db.SQL, d, "tn_nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetTenant unknown err = %v", err)
		}

		ch, err := store.CreateChannel(ctx, db.SQL, d, a.ID, "web", 300)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(ch.ID, "ch_") || ch.TenantID != a.ID || ch.Name != "web" || ch.CreatedAt != 300 {
			t.Errorf("channel = %+v", ch)
		}
		if _, err := store.CreateChannel(ctx, db.SQL, d, a.ID, "web", 301); err == nil {
			t.Error("duplicate channel name accepted")
		}
	})
}
