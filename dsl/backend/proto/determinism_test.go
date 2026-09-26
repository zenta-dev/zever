package proto

import (
	"sort"
	"testing"
)

// determinismFixture is a three-module schema deliberately wide enough to
// exercise every map-shaped intermediate a backend might range over without
// sorting first: multiple entities per module, multiple fields per entity
// (including an enum field and a belongs_to relation), and multiple
// operations per service — with errors:, auth:, and permission: each
// appearing on at least one rpc so those resolved slices/maps are exercised
// too.
const determinismFixture = `
	entity Invoice {
		id: uuid @primary
		number: string @unique
		amount_cents: int64
		status: enum(draft, sent, paid, void) @default(draft)
		created_at: timestamp @default(now())
	}

	entity LineItem {
		id: uuid @primary
		invoice_id: uuid
		description: string
		quantity: int32
		unit_price_cents: int64

		belongs_to invoice: Invoice @foreign_key(invoice_id)
	}

	service BillingService {
		rpc GetInvoice(id: uuid) -> Invoice {
			http: GET "/v1/invoices/{id}"
			auth: required(roles: {owner, admin})
		}

		rpc CreateInvoice(number: string, amount_cents: int64) -> Invoice {
			http: POST "/v1/invoices"
			auth: required(roles: {admin})
			permission: check("owns_invoice", resource: Invoice, owner_field: number)
			errors: { not_found, invalid_argument("amount must be positive") }
		}

		rpc VoidInvoice(id: uuid) -> Invoice {
			http: DELETE "/v1/invoices/{id}"
			auth: required
			errors: { not_found }
		}
	}



	entity Shipment {
		id: uuid @primary
		tracking_code: string @unique
		carrier: string
		delivered: bool
	}

	service ShippingService {
		rpc GetShipment(id: uuid) -> Shipment {
			http: GET "/v1/shipments/{id}"
			auth: required
		}

		rpc CreateShipment(tracking_code: string, carrier: string) -> Shipment {
			http: POST "/v1/shipments"
			auth: required(roles: {owner})
		}
	}



	entity Warehouse {
		id: uuid @primary
		name: string
		region: string
	}

	entity StockItem {
		id: uuid @primary
		warehouse_id: uuid
		sku: string @unique
		quantity: int32

		belongs_to warehouse: Warehouse @foreign_key(warehouse_id)
	}

	service InventoryService {
		rpc GetStockItem(id: uuid) -> StockItem {
			http: GET "/v1/stock-items/{id}"
			auth: required(roles: {owner, admin})
			permission: check("owns_stock_item", resource: StockItem, owner_field: sku)
		}

		rpc AdjustStock(id: uuid, quantity: int32) -> StockItem {
			http: POST "/v1/stock-items/{id}/adjust"
			auth: required
			errors: { not_found, invalid_argument("quantity delta invalid") }
		}
	}
`

// TestGenerateIsDeterministic re-resolves determinismFixture from scratch
// twice and runs Generate over each resulting schema, asserting the two
// outputs share the same set of filenames and byte-identical content per
// file — proving no unsorted map iteration (imports, message names, ...)
// leaks into this backend's output.
func TestGenerateIsDeterministic(t *testing.T) {
	schemaA := compileSchema(t, determinismFixture)
	schemaB := compileSchema(t, determinismFixture)

	outA, err := New().Generate(schemaA)
	if err != nil {
		t.Fatalf("Generate (first run): %v", err)
	}

	outB, err := New().Generate(schemaB)
	if err != nil {
		t.Fatalf("Generate (second run): %v", err)
	}

	assertOutputsIdentical(t, outA, outB)
}

// assertOutputsIdentical compares two Generate outputs key-by-key in sorted
// order, so a genuine key-set mismatch is reported deterministically
// instead of depending on Go's own randomized map iteration in this test.
func assertOutputsIdentical(t *testing.T, a, b map[string][]byte) {
	t.Helper()

	keysA := sortedKeysOf(a)
	keysB := sortedKeysOf(b)

	if len(keysA) != len(keysB) {
		t.Fatalf("output key sets differ in size: run1=%v run2=%v", keysA, keysB)
	}

	for i, k := range keysA {
		if keysB[i] != k {
			t.Fatalf("output key sets differ: run1=%v run2=%v", keysA, keysB)
		}
	}

	for _, k := range keysA {
		if string(a[k]) != string(b[k]) {
			t.Fatalf("output for %q differs between runs:\n--- run1 ---\n%s\n--- run2 ---\n%s", k, a[k], b[k])
		}
	}
}

func sortedKeysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
