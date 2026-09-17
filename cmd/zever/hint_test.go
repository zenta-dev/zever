package main

import (
	"strings"
	"testing"
)

func TestShouldShowHint(t *testing.T) {
	t.Run("shown by default", func(t *testing.T) {
		t.Setenv("ZEVER_NO_HINT", "")
		if !shouldShowHint() {
			t.Fatalf("shouldShowHint = false, want true when ZEVER_NO_HINT unset")
		}
	})

	t.Run("silenced when set", func(t *testing.T) {
		t.Setenv("ZEVER_NO_HINT", "1")
		if shouldShowHint() {
			t.Fatalf("shouldShowHint = true, want false when ZEVER_NO_HINT=1")
		}
	})

	t.Run("any non-empty value silences", func(t *testing.T) {
		t.Setenv("ZEVER_NO_HINT", "0")
		if shouldShowHint() {
			t.Fatalf("shouldShowHint = true, want false when ZEVER_NO_HINT=0")
		}
	})
}

func TestGeneralTips_retargeted(t *testing.T) {
	if len(generalTips) == 0 {
		t.Fatalf("generalTips empty")
	}
	for _, tip := range generalTips {
		if strings.Contains(tip, "zengo") || strings.Contains(tip, "zen-go") || strings.Contains(tip, "ZENGO") {
			t.Errorf("tip %q still references zengo", tip)
		}
		if !strings.Contains(tip, "zever") {
			t.Errorf("tip %q missing zever reference", tip)
		}
	}
}

func TestHintFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want string
	}{
		{"new", "new", "next: cd <app> && zever compile schema/app.zen && zever serve"},
		{"generate module", "generate module", "next: zever generate entity <module> <Name> --field title:string"},
		{"generate entity", "generate entity", "next: zever compile to validate schema"},
		{"generate job", "generate job", `next: zever generate schedule <module> <Name> --cron "*/5 * * * *" --dispatch <Job>`},
		{"generate schedule", "generate schedule", "next: verify with zever compile"},
		{"generate server", "generate server", "next: zever serve  •  or zever dev for watch-mode"},
		{"generate worker", "generate worker", "next: zever queue:work  •  or zever dev"},
		{"generate seed", "generate seed", "next: zever db seed"},
		{"generate tinker", "generate tinker", "next: zever tinker"},
		{"generate adapter", "generate adapter", "next: implement TODOs in the new adapter package"},
		{"extract", "extract", "next: cd <service> && go mod tidy && zever compile"},
		{"compile", "compile", "next: zever db migrate --adapter=sqlite --dsn=data/app.db schema/*.zen"},
		{"doctor", "doctor", "next: zever compile schema/app.zen"},
		{"routes", "routes", "tip: add http binding to an RPC to see more routes"},
		{"check", "check", "next: zever compile schema/*.zen --backend=proto,zenorm"},
		{"fmt", "fmt", "next: zever check schema/*.zen"},
		{"check-boundaries", "check-boundaries", "tip: keep modules isolated — move shared entities to their own module"},
		{"db migrate", "db migrate", "next: zever db seed  •  or zever serve"},
		{"db seed", "db seed", "next: zever serve"},
		{"unknown", "bogus", ""},
		{"empty", "", ""},
		{"case sensitive", "New", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hintFor(tc.cmd); got != tc.want {
				t.Fatalf("hintFor(%q) = %q, want %q", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestHintFor_noZengoReferences(t *testing.T) {
	cmds := []string{"new", "generate module", "compile", "doctor", "extract", "db migrate"}
	for _, c := range cmds {
		if got := hintFor(c); strings.Contains(got, "zengo") {
			t.Errorf("hintFor(%q) = %q, still references zengo", c, got)
		}
	}
}
