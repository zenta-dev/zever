package cdn

import "testing"

// TestRef pins the service key carried by Ref and DefaultRef: the
// container resolves these Refs, so a wrong key would wire the wrong
// instance.
func TestRef(t *testing.T) {
	t.Parallel()

	r := Ref("analytics")
	if r.Service != "cdn" || r.Name != "analytics" {
		t.Fatalf("Ref = %+v, want service cdn name analytics", r)
	}

	d := DefaultRef()
	if d.Service != "cdn" || d.Name != "default" {
		t.Fatalf("DefaultRef = %+v, want service cdn name default", d)
	}

	if r == d {
		t.Fatal("named Ref equals DefaultRef, want distinct keys")
	}
}
