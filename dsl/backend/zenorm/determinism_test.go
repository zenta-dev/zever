package zenorm

import (
	"sort"
	"testing"
)

// determinismFixture is a multi-entity, multi-field schema deliberately
// wide enough to exercise every map-shaped intermediate a backend might
// range over without sorting first: multiple entities, multiple fields per
// entity (including an enum field, a nullable field and a timestamp
// field), and unrelated services/messages this backend never reads.
const determinismFixture = `
	entity Invoice {
		id: uuid @primary
		number: string @unique
		amount_cents: int64
		status: enum(draft, sent, paid, void) @default(draft)
		created_at: timestamp @default(now())
		notes: string?
	}

	entity LineItem {
		id: uuid @primary
		invoice_id: uuid
		description: string
		quantity: int32
		unit_price_cents: int64
	}

	service BillingService {
		rpc GetInvoice(id: uuid) -> Invoice {
			http: GET "/v1/invoices/{id}"
			auth: required
		}
	}

	entity Shipment {
		id: uuid @primary
		tracking_code: string @unique
		carrier: string
		delivered: bool
	}

	entity Warehouse {
		id: uuid @primary
		name: string
		region: string?
	}
`

// TestGenerateIsDeterministic re-resolves determinismFixture from scratch
// twice and runs Generate over each resulting schema, asserting the two
// outputs share the same set of filenames and byte-identical content per
// file.
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
