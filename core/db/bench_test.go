package db

import "testing"

func BenchmarkOpen(b *testing.B) {
	a := dbFreshAdapter()
	if err := Register(a, func(Options) (DB, error) { return &stubDB{dialect: "sqlite"}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := dbFreshAdapter()
	if err := Register(a, func(Options) (DB, error) { return &stubDB{dialect: "sqlite"}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkIsolationLevelString(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Serializable.String()
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{MaxConns: 10, MinConns: 1}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := opts.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}
