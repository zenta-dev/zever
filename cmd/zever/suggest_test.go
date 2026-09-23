package main

import (
	"slices"
	"testing"
)

func TestClosest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		candidates []string
		want       string
	}{
		{"exact", "compile", allTopLevel, "compile"},
		{"one deletion", "compil", allTopLevel, "compile"},
		{"one insertion", "compilee", allTopLevel, "compile"},
		{"transposition", "comiple", allTopLevel, "compile"},
		{"case insensitive", "COMPILE", allTopLevel, "compile"},
		{"mixed case typo", "Generat", allTopLevel, "generate"},
		{"colon command typo", "queue:wrk", allTopLevel, "queue:work"},
		{"hyphenated", "check-boundarie", allTopLevel, "check-boundaries"},
		{"empty input", "", allTopLevel, ""},
		{"no candidates", "compile", nil, ""},
		{"empty candidates", "compile", []string{}, ""},
		{"short input no near match", "zz", allTopLevel, ""},
		{"short input exact still wins", "db", allTopLevel, "db"},
		{"far input", "zzzqqqxxx", allTopLevel, ""},
		{"generate sub typo", "entit", allGenerate, "entity"},
		{"db sub typo", "migrat", allDB, "migrate"},
		{"backend typo", "proo", allBackends, "proto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := closest(tc.input, tc.candidates); got != tc.want {
				t.Fatalf("closest(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDamerauLevenshtein(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want int
	}{
		{"identical", "compile", "compile", 0},
		{"empty a", "", "abc", 3},
		{"empty b", "abc", "", 3},
		{"both empty", "", "", 0},
		{"single substitution", "a", "b", 1},
		{"transposition costs one", "ab", "ba", 1},
		{"classic kitten sitting", "kitten", "sitting", 3},
		{"prefix insertion", "compile", "recompile", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := damerauLevenshtein(tc.a, tc.b); got != tc.want {
				t.Fatalf("damerauLevenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCandidateLists_complete(t *testing.T) {
	wantTop := []string{
		"new", "compile", "check", "fmt", "doctor", "config", "routes", "explain",
		"check-boundaries", "check:boundaries", "graph", "generate", "extract", "serve", "dev",
		"queue:work", "schedule:run", "tinker", "db", "help",
	}
	if !slices.Equal(allTopLevel, wantTop) {
		t.Fatalf("allTopLevel = %v, want %v", allTopLevel, wantTop)
	}

	wantGen := []string{"module", "entity", "job", "schedule", "server", "worker", "seed", "tinker", "adapter"}
	if !slices.Equal(allGenerate, wantGen) {
		t.Fatalf("allGenerate = %v, want %v", allGenerate, wantGen)
	}

	wantDB := []string{"migrate", "rollback", "seed"}
	if !slices.Equal(allDB, wantDB) {
		t.Fatalf("allDB = %v, want %v", allDB, wantDB)
	}

	wantBackends := []string{"proto", "zenorm", "atlas", "openapi", "gogen", "protogogen"}
	if !slices.Equal(allBackends, wantBackends) {
		t.Fatalf("allBackends = %v, want %v", allBackends, wantBackends)
	}
}

func TestClosestPrefixBonus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		candidates []string
		want       string
	}{
		{"prefix bonus hit still far", "gen", []string{"generate"}, ""},
		{"prefix bonus selects", "prot", []string{"proto", "zenorm"}, "proto"},
		{"short prefix rejected", "ge", allTopLevel, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := closest(tc.input, tc.candidates); got != tc.want {
				t.Fatalf("closest(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
