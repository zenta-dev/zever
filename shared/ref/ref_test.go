package ref

import "testing"

func TestRefNamed(t *testing.T) {
	t.Parallel()

	r := Named[int, string]("db", "analytics")
	if r.Service != "db" || r.Name != "analytics" {
		t.Fatalf("Named = %+v, want {db analytics}", r)
	}
}

func TestRefDefault(t *testing.T) {
	t.Parallel()

	r := Default[int, string]("db")
	if r.Service != "db" || r.Name != DefaultName {
		t.Fatalf("Default = %+v, want {db default}", r)
	}
}

func TestRefWithName(t *testing.T) {
	t.Parallel()

	r := Default[int, string]("db").WithName("analytics")
	if r.Service != "db" || r.Name != "analytics" {
		t.Fatalf("WithName = %+v, want {db analytics}", r)
	}
}

func TestRefComparable(t *testing.T) {
	t.Parallel()

	a := Named[int, string]("db", "x")
	b := Named[int, string]("db", "x")
	c := Named[int, string]("db", "y")

	m := map[Ref[int, string]]int{a: 1}
	if m[b] != 1 {
		t.Fatalf("map lookup with equal Ref = %d, want 1", m[b])
	}

	if _, ok := m[c]; ok {
		t.Fatalf("map lookup with different Ref hit, want miss")
	}
}

func TestRefString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  Ref[int, string]
		want string
	}{
		{name: "default", ref: Default[int, string]("db"), want: "db"},
		{name: "named", ref: Named[int, string]("db", "analytics"), want: "db/analytics"},
	}

	for _, tt := range tests {
		if got := tt.ref.String(); got != tt.want {
			t.Errorf("String(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
