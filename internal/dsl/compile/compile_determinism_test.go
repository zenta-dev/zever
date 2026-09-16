package compile

import (
	"reflect"
	"testing"
)

// resolveDeterminismFixture mirrors the shape used by the backend packages'
// own TestGenerateIsDeterministic fixtures: three modules, multiple entities
// per module, multiple fields per entity (including an enum field and a
// belongs_to relation), and services whose rpcs exercise errors:, auth:,
// and permission: — all of which the resolver turns into ir.Schema slices
// (and, in a few places, maps) that a real bug could shuffle between runs.
// In dir-derived mode, the three modules are represented as three separate
// files under schema/{billing,shipping,inventory}/.
var resolveDeterminismFiles = map[string]string{
	"schema/billing/billing.zen": `entity Invoice {
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
	}`,
	"schema/shipping/shipping.zen": `entity Shipment {
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
	}`,
	"schema/inventory/inventory.zen": `entity Warehouse {
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
	}`,
}

// TestResolveIsDeterministic compiles the identical source through the full
// parser -> resolver pipeline twice (via Compile, with no backends) and
// deep-compares the two resulting *ir.Schema values. This guards against a
// problem at a layer below any individual backend: if the resolver itself
// built any ir.Schema slice (Modules, Entities, Fields, Services, RPCs, ...)
// by ranging an unsorted map, the two schemas would already disagree on
// ordering before either backend ever saw them, and every backend's own
// determinism test could still pass by accident (each backend run would
// merely reproduce whatever order that single Generate call's process
// happened to observe from the map, self-consistently, without ever running
// the resolver twice within the same test to expose the shuffle).
func TestResolveIsDeterministic(t *testing.T) {
	files := resolveDeterminismFiles

	resultA, diagsA := WithSchemaDir(files, "schema")
	if diagsA.HasErrors() {
		t.Fatalf("Compile (first run): %v", diagsA)
	}

	resultB, diagsB := WithSchemaDir(files, "schema")
	if diagsB.HasErrors() {
		t.Fatalf("Compile (second run): %v", diagsB)
	}

	if !reflect.DeepEqual(resultA.Schema, resultB.Schema) {
		t.Fatalf("resolved schema differs between runs on identical input:\n--- run1 ---\n%+v\n--- run2 ---\n%+v",
			resultA.Schema, resultB.Schema)
	}
}
