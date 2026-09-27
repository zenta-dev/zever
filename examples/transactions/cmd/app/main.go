// Command app is the transactions example: money movement between wallets
// (schema/transactions.zen, codegen'd by the zenorm backend into
// generated/zenorm/orm/gen/app) run through orm.WithNestedTx.
//
// WithNestedTx is a transparent savepoint wrapper around db.WithTx: called
// with no transaction in the context it behaves exactly like db.WithTx
// (Begin, fn, Commit/Rollback); called from inside another WithNestedTx it
// issues a SAVEPOINT instead of failing with db's "nested transaction not
// supported" error, and RELEASE/RollbackTo that savepoint on success/failure
// without ever touching the outer transaction. That makes any transfer
// function safe to call both on its own and as a unit of a larger batch --
// the savepoint rollback undoes only the failed step's own writes.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	gen "github.com/zenta-dev/zever/examples/transactions/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func main() {
	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := seed(ctx, conn); err != nil {
		die(err)
	}

	// 1. A single transfer at the top level: WithNestedTx is a plain
	//    db.WithTx here (Begin/Commit/Rollback).
	if err := transfer(ctx, conn, "w-ada", "w-grace", 1500, "lunch"); err != nil {
		die(err)
	}

	if err := printBalances(ctx, conn, "after one top-level transfer (Ada -> Grace 1500):"); err != nil {
		die(err)
	}

	// 2. A nested demo: an outer WithNestedTx runs two transfers. The first
	//    commits; the second writes its debit + transfer row + credit and
	//    then fails, so its inner SAVEPOINT is rolled back -- the outer
	//    transaction still commits, keeping only the first transfer.
	if err := runNestedBatch(ctx, conn); err != nil {
		die(err)
	}

	if err := printBalances(ctx, conn, "after nested batch (Grace -> Margaret 2500 committed, Margaret -> Grace 1000 rolled back):"); err != nil {
		die(err)
	}

	if err := verifyFailedTransfer(ctx, conn); err != nil {
		die(err)
	}

	// 3. A genuinely failing top-level transfer: insufficient funds rolls
	//    back the whole transaction, leaving every balance untouched.
	if err := transfer(ctx, conn, "w-ada", "w-grace", 999999, "broke"); err == nil {
		die(errors.New("expected insufficient-funds error, got nil"))
	} else {
		fmt.Printf("top-level transfer of 999999 correctly failed: %v\n", err)
	}

	if err := printBalances(ctx, conn, "after the failed top-level transfer (unchanged):"); err != nil {
		die(err)
	}
}

// seed creates the wallets/transfers tables and three wallets.
func seed(ctx context.Context, conn db.DB) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE wallets (id text, owner text, balance_cents integer)`); err != nil {
		return fmt.Errorf("create wallets: %w", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE transfers (id text, from_wallet_id text, to_wallet_id text, amount_cents integer, note text)`); err != nil {
		return fmt.Errorf("create transfers: %w", err)
	}

	wallet := []struct {
		id, owner string
		balance   int64
	}{
		{"w-ada", "Ada", 10000},
		{"w-grace", "Grace", 20000},
		{"w-margaret", "Margaret", 30000},
	}

	for _, w := range wallet {
		insert := orm.InsertInto(gen.Wallets).Values(
			orm.Set(gen.WalletCols.ID, w.id),
			orm.Set(gen.WalletCols.Owner, w.owner),
			orm.Set(gen.WalletCols.BalanceCents, w.balance),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert wallet %s: %w", w.id, err)
		}
	}

	return nil
}

// transfer moves amount cents from the wallet fromID to the wallet toID
// and records a Transfer row, all inside one WithNestedTx. It reads both
// balances inside the transaction, debits, inserts the record, credits,
// and finally -- when note is "simulated-failure" -- returns an error
// AFTER every write, so a caller that catches the error proves the
// savepoint (or whole-transaction, at the top level) rollback undid the
// partial work.
func transfer(ctx context.Context, exec db.DB, fromID, toID string, amount int64, note string) error {
	return orm.WithNestedTx(ctx, exec, func(ctx context.Context, tx db.Tx) error {
		from, ok, err := orm.From(gen.Wallets).Where(gen.WalletCols.ID.Eq(fromID)).First(ctx, tx)
		if err != nil {
			return fmt.Errorf("read from wallet %s: %w", fromID, err)
		}

		if !ok {
			return fmt.Errorf("wallet %s not found", fromID)
		}

		if from.BalanceCents < amount {
			return fmt.Errorf("insufficient funds: %s has %d cents, needs %d", from.Owner, from.BalanceCents, amount)
		}

		to, ok, err := orm.From(gen.Wallets).Where(gen.WalletCols.ID.Eq(toID)).First(ctx, tx)
		if err != nil {
			return fmt.Errorf("read to wallet %s: %w", toID, err)
		}

		if !ok {
			return fmt.Errorf("wallet %s not found", toID)
		}

		if _, err := orm.UpdateTable(gen.Wallets).
			Set(orm.Set(gen.WalletCols.BalanceCents, from.BalanceCents-amount)).
			Where(gen.WalletCols.ID.Eq(fromID)).
			Exec(ctx, tx); err != nil {
			return fmt.Errorf("debit %s: %w", fromID, err)
		}

		insert := orm.InsertInto(gen.Transfers).Values(
			orm.Set(gen.TransferCols.ID, fmt.Sprintf("t-%s-%s-%d", fromID, toID, amount)),
			orm.Set(gen.TransferCols.FromWalletID, fromID),
			orm.Set(gen.TransferCols.ToWalletID, toID),
			orm.Set(gen.TransferCols.AmountCents, amount),
			orm.Set(gen.TransferCols.Note, note),
		)

		if err := insert.Exec(ctx, tx); err != nil {
			return fmt.Errorf("insert transfer: %w", err)
		}

		if _, err := orm.UpdateTable(gen.Wallets).
			Set(orm.Set(gen.WalletCols.BalanceCents, to.BalanceCents+amount)).
			Where(gen.WalletCols.ID.Eq(toID)).
			Exec(ctx, tx); err != nil {
			return fmt.Errorf("credit %s: %w", toID, err)
		}

		if note == "simulated-failure" {
			return errors.New("simulated failure after debit, transfer row and credit")
		}

		return nil
	})
}

// runNestedBatch runs one outer WithNestedTx containing two transfers. The
// second transfer fails after its writes; the failure is caught here (not
// propagated), so the outer transaction commits with only the first
// transfer's work. The inner WithNestedTx degrades to a SAVEPOINT --
// db.WithTx would have rejected the nesting outright.
func runNestedBatch(ctx context.Context, conn db.DB) error {
	err := orm.WithNestedTx(ctx, conn, func(ctx context.Context, tx db.Tx) error {
		if err := transfer(ctx, tx, "w-grace", "w-margaret", 2500, "rent"); err != nil {
			return err
		}

		// This inner transfer rolls back to its savepoint: the debit, the
		// transfer row and the credit are all undone, and the error is
		// caught here so the outer transaction can still commit the "rent"
		// transfer above.
		if err := transfer(ctx, tx, "w-margaret", "w-grace", 1000, "simulated-failure"); err != nil {
			fmt.Printf("inner transfer failed and rolled back to its savepoint: %v\n", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("outer nested batch: %w", err)
	}

	return nil
}

// verifyFailedTransfer proves the rolled-back transfer left no Transfer row
// and that only the two successful transfers are recorded.
func verifyFailedTransfer(ctx context.Context, conn db.DB) error {
	total, err := orm.From(gen.Transfers).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("count transfers: %w", err)
	}

	failed, err := orm.From(gen.Transfers).Where(gen.TransferCols.Note.Eq("simulated-failure")).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("count failed transfers: %w", err)
	}

	fmt.Printf("transfer rows: %d total, %d with note %q (the savepoint rollback removed the failed one)\n", total, failed, "simulated-failure")

	return nil
}

func printBalances(ctx context.Context, conn db.DB, heading string) error {
	fmt.Println(heading)

	rows, err := orm.From(gen.Wallets).OrderBy(gen.WalletCols.ID.Asc()).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("list wallets: %w", err)
	}

	for _, w := range rows {
		fmt.Printf("  %-10s %8s %7d cents\n", w.ID, w.Owner, w.BalanceCents)
	}

	return nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
