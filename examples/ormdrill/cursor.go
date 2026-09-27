package ormdrill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// DemoCursorPagination walks all orders with a typed CursorKey, proves the
// Encode -> DecodeCursor round-trip (and that a token cannot be replayed
// against the wrong column), then shows the OffsetPage number-based
// alternative. The first page runs uncursored; every later page ANDs the
// decoded cursor's keyset predicate in via NextPage.
func DemoCursorPagination(ctx context.Context, conn db.DB) error {
	fmt.Println("== CursorKey: keyset walk over orders (created_at DESC, page size 3)")

	const pageSize = 3

	// direction carries only the ORDER BY direction; cursor is the real
	// keyset position, zero until the first page supplies it.
	direction := orm.NewCursorKey(OrderCols.CreatedAt, time.Time{}).Desc()

	var cursor orm.CursorKey[Order, time.Time]

	for pageNo := 1; ; pageNo++ {
		q := orm.From(Orders).OrderBy(direction.OrderTerm()).Limit(pageSize)

		if pageNo > 1 {
			q = orm.NextPage(q, cursor)
		}

		page, err := q.All(ctx, conn)
		if err != nil {
			return fmt.Errorf("keyset page %d: %w", pageNo, err)
		}

		if len(page) == 0 {
			break
		}

		fmt.Printf("  page %d:", pageNo)

		for _, o := range page {
			fmt.Printf(" %s", o.ID)
		}

		fmt.Println()

		last := page[len(page)-1]

		// Round-trip the token on the way to the next cursor: encode,
		// decode against the SAME column, and only keep the result if it
		// reproduces the cursor just built.
		next := orm.NewCursorKey(OrderCols.CreatedAt, last.CreatedAt).Desc()

		token, err := next.Encode()
		if err != nil {
			return fmt.Errorf("encode cursor: %w", err)
		}

		decoded, ok, err := orm.DecodeCursor(token, OrderCols.CreatedAt)
		if err != nil {
			return fmt.Errorf("decode cursor: %w", err)
		}

		if !ok || decoded.Value() != next.Value() || decoded.Descending() != next.Descending() {
			return fmt.Errorf("cursor round-trip mismatch: %+v vs %+v", decoded, next)
		}

		cursor = decoded

		// A token carries its column, so replaying it against the wrong
		// column is a typed error, never a silent wrong page.
		_, _, wrongColErr := orm.DecodeCursor(token, OrderCols.ID)
		if wrongColErr == nil {
			return errors.New("expected wrong-column cursor decode to fail")
		}

		fmt.Printf("  next cursor %s; replay against OrderCols.ID rejected: %v\n", token, wrongColErr)

		if len(page) < pageSize {
			break
		}
	}

	// The number-based alternative: LIMIT pageSize OFFSET (page-1)*pageSize.
	paged, err := orm.OffsetPage(orm.From(Orders).OrderBy(OrderCols.CreatedAt.Desc()), 2, pageSize)
	if err != nil {
		return fmt.Errorf("offset page: %w", err)
	}

	rows, err := paged.All(ctx, conn)
	if err != nil {
		return fmt.Errorf("offset page all: %w", err)
	}

	ids := make([]string, len(rows))

	for i, o := range rows {
		ids[i] = o.ID
	}

	fmt.Printf("  OffsetPage(page 2, size %d) = %v\n\n", pageSize, ids)

	return nil
}
