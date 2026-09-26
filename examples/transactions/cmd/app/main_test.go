package main

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	gen "github.com/zenta-dev/zever/examples/transactions/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func openTestDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return ctx, conn
}

func testBalances(ctx context.Context, t *testing.T, conn db.DB) map[string]int64 {
	t.Helper()

	rows, err := orm.From(gen.Wallets).OrderBy(gen.WalletCols.ID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("list wallets: %v", err)
	}

	got := make(map[string]int64, len(rows))
	for _, w := range rows {
		got[w.ID] = w.BalanceCents
	}

	return got
}

func assertBalances(t *testing.T, got, want map[string]int64) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("balances = %v, want %v", got, want)
	}

	for id, w := range want {
		if got[id] != w {
			t.Fatalf("balances = %v, want %v", got, want)
		}
	}
}

func testTransferCount(ctx context.Context, t *testing.T, conn db.DB) (total, failed int64) {
	t.Helper()

	var err error

	total, err = orm.From(gen.Transfers).Count(ctx, conn)
	if err != nil {
		t.Fatalf("count transfers: %v", err)
	}

	failed, err = orm.From(gen.Transfers).Where(gen.TransferCols.Note.Eq("simulated-failure")).Count(ctx, conn)
	if err != nil {
		t.Fatalf("count failed transfers: %v", err)
	}

	return total, failed
}

// TestTopLevelTransfer moves funds once and checks the same balances the
// demo prints after its first transfer.
func TestTopLevelTransfer(t *testing.T) {
	ctx, conn := openTestDB(t)

	if err := transfer(ctx, conn, "w-ada", "w-grace", 1500, "lunch"); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	assertBalances(t, testBalances(ctx, t, conn), map[string]int64{
		"w-ada":      8500,
		"w-grace":    21500,
		"w-margaret": 30000,
	})

	if total, failed := testTransferCount(ctx, t, conn); total != 1 || failed != 0 {
		t.Fatalf("transfers = %d total, %d failed; want 1 total, 0 failed", total, failed)
	}
}

// TestNestedBatchSavepointRollback replays the demo's nested batch: the
// first inner transfer commits, the second rolls back to its savepoint,
// and the outer transaction still commits.
func TestNestedBatchSavepointRollback(t *testing.T) {
	ctx, conn := openTestDB(t)

	if err := transfer(ctx, conn, "w-ada", "w-grace", 1500, "lunch"); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	if err := runNestedBatch(ctx, conn); err != nil {
		t.Fatalf("nested batch: %v", err)
	}

	assertBalances(t, testBalances(ctx, t, conn), map[string]int64{
		"w-ada":      8500,
		"w-grace":    19000,
		"w-margaret": 32500,
	})

	if total, failed := testTransferCount(ctx, t, conn); total != 2 || failed != 0 {
		t.Fatalf("transfers = %d total, %d failed; want 2 total, 0 failed", total, failed)
	}
}

// TestInsufficientFundsRollsBack proves the app-level funds check fails the
// whole top-level transaction, leaving every balance untouched.
func TestInsufficientFundsRollsBack(t *testing.T) {
	ctx, conn := openTestDB(t)

	err := transfer(ctx, conn, "w-ada", "w-grace", 999999, "broke")
	if err == nil {
		t.Fatal("transfer with insufficient funds succeeded, want error")
	}

	if !strings.Contains(err.Error(), "insufficient funds") {
		t.Fatalf("error = %q, want it to mention insufficient funds", err)
	}

	assertBalances(t, testBalances(ctx, t, conn), map[string]int64{
		"w-ada":      10000,
		"w-grace":    20000,
		"w-margaret": 30000,
	})

	if total, _ := testTransferCount(ctx, t, conn); total != 0 {
		t.Fatalf("transfers = %d, want 0 after rolled-back transfer", total)
	}
}
