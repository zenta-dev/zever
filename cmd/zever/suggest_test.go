package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestClosest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		candidates []string
		want       string
	}{
		{"exact", "compile", allTopLevel(), "compile"},
		{"one deletion", "compil", allTopLevel(), "compile"},
		{"one insertion", "compilee", allTopLevel(), "compile"},
		{"transposition", "comiple", allTopLevel(), "compile"},
		{"case insensitive", "COMPILE", allTopLevel(), "compile"},
		{"mixed case typo", "Generat", allTopLevel(), "generate"},
		{"colon command typo", "queue:wrk", allTopLevel(), "queue:work"},
		{"hyphenated", "check-boundarie", allTopLevel(), "check-boundaries"},
		{"empty input", "", allTopLevel(), ""},
		{"no candidates", "compile", nil, ""},
		{"empty candidates", "compile", []string{}, ""},
		{"short input no near match", "zz", allTopLevel(), ""},
		{"short input exact still wins", "db", allTopLevel(), "db"},
		{"far input", "zzzqqqxxx", allTopLevel(), ""},
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
	// allTopLevel() derives from the Cobra tree (suggest.go): the shared
	// global and a fresh tree must agree, which fails if the global is
	// mutated or if a hand list is reintroduced. The deliberate
	// check-boundaries/check:boundaries alias pair must survive.
	if got, want := allTopLevel(), topLevelNames(newRootCmd()); !slices.Equal(got, want) {
		t.Fatalf("allTopLevel() = %v, fresh tree = %v, want identical", got, want)
	}

	for _, want := range []string{"docs", "completion", "check-boundaries", "check:boundaries", "help"} {
		if !slices.Contains(allTopLevel(), want) {
			t.Errorf("allTopLevel() missing %q", want)
		}
	}

	wantGen := []string{"module", "entity", "job", "schedule", "server", "worker", "seed", "tinker", "adapter"}
	if !slices.Equal(allGenerate, wantGen) {
		t.Fatalf("allGenerate = %v, want %v", allGenerate, wantGen)
	}

	wantDB := []string{"migrate", "rollback", "seed"}
	if !slices.Equal(allDB, wantDB) {
		t.Fatalf("allDB = %v, want %v", allDB, wantDB)
	}

	wantBackends := []string{"proto", "zenorm", "atlas", "openapi", "gogen", "protogogen", "mcp"}
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
		{"short prefix rejected", "ge", allTopLevel(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := closest(tc.input, tc.candidates); got != tc.want {
				t.Fatalf("closest(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestUnknownSubcommandErrorPutsTheErrorFirst pins the guidance shape the
// top-level unknown-subcommand path already uses (see run in main.go): the
// error is the first line, the closest match follows as a hint, and the
// command that lists the real choices comes last.
func TestUnknownSubcommandErrorPutsTheErrorFirst(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	err := unknownSubcommandError("zever config", "showw", []string{"show"}, "zever config -h")
	if err == nil {
		t.Fatal("unknownSubcommandError: got nil error")
	}

	lines := strings.Split(err.Error(), "\n")
	if !strings.HasPrefix(lines[0], `zever config: unknown subcommand "showw"`) {
		t.Fatalf("first line = %q, want the error", lines[0])
	}

	if !strings.Contains(err.Error(), `did you mean "show"?`) {
		t.Fatalf("error lacks the did-you-mean hint: %q", err.Error())
	}

	if !strings.Contains(err.Error(), "zever config -h") {
		t.Fatalf("error lacks the list command: %q", err.Error())
	}
}

// TestUnknownSubcommandErrorKeepsTheSentinel pins that the reworded error
// still satisfies errors.Is against ErrUnknownSubcommand, so a caller that
// branches on the sentinel is unaffected by the message change.
func TestUnknownSubcommandErrorKeepsTheSentinel(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	err := unknownSubcommandError("zever config", "showw", []string{"show"}, "zever config -h")
	if err == nil {
		t.Fatal("unknownSubcommandError: got nil error")
	}

	if !errors.Is(err, ErrUnknownSubcommand) {
		t.Fatalf("error %q does not match ErrUnknownSubcommand", err.Error())
	}
}

// TestUnknownSubcommandErrorSilencesHints pins ZEVER_NO_HINT: the
// did-you-mean and list-command lines are hints, so silencing hints leaves
// the failure alone.
func TestUnknownSubcommandErrorSilencesHints(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "1")

	err := unknownSubcommandError("zever config", "showw", []string{"show"}, "zever config -h")
	if err == nil {
		t.Fatal("unknownSubcommandError: got nil error")
	}

	msg := err.Error()
	if strings.Contains(msg, "hint:") {
		t.Fatalf("error carries a hint while ZEVER_NO_HINT is set:\n%s", msg)
	}

	if !strings.Contains(msg, `unknown subcommand "showw"`) {
		t.Fatalf("error = %q, want the failure itself", msg)
	}
}

// TestUnknownSubcommandErrorWithoutCloseMatch pins the no-close-match
// shape: the error still names the command that lists the choices.
func TestUnknownSubcommandErrorWithoutCloseMatch(t *testing.T) {
	t.Setenv("ZEVER_NO_HINT", "")

	err := unknownSubcommandError("zever db", "zzzzzz", allDB, "zever db -h")
	if err == nil {
		t.Fatal("unknownSubcommandError: got nil error")
	}

	if strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error suggests a match that does not exist: %q", err.Error())
	}

	if !strings.Contains(err.Error(), "zever db -h") {
		t.Fatalf("error lacks the list command: %q", err.Error())
	}
}

// TestConfigUnknownSubcommandSkipsUsageDump pins the terminal ergonomics:
// the error names the bad subcommand instead of burying it under the whole
// config usage block.
func TestConfigUnknownSubcommandSkipsUsageDump(t *testing.T) {
	var err error

	stderr := captureZeverStderr(t, func() {
		err = runConfig([]string{"showw"})
	})
	if err == nil {
		t.Fatal("runConfig showw: got nil error")
	}

	if !strings.Contains(err.Error(), `unknown subcommand "showw"`) {
		t.Fatalf("error = %q, want the unknown subcommand", err.Error())
	}

	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr carries a usage dump:\n%s", stderr)
	}
}

// TestDBUnknownSubcommandSkipsUsageDump is the db sibling of
// TestConfigUnknownSubcommandSkipsUsageDump.
func TestDBUnknownSubcommandSkipsUsageDump(t *testing.T) {
	var err error

	stderr := captureZeverStderr(t, func() {
		err = runDB([]string{"migrat"})
	})
	if err == nil {
		t.Fatal("runDB migrat: got nil error")
	}

	if !strings.Contains(err.Error(), `unknown subcommand "migrat"`) {
		t.Fatalf("error = %q, want the unknown subcommand", err.Error())
	}

	if !strings.Contains(err.Error(), `did you mean "migrate"?`) {
		t.Fatalf("error lacks the did-you-mean hint: %q", err.Error())
	}

	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr carries a usage dump:\n%s", stderr)
	}
}

// TestGenerateUnknownSubcommandSkipsUsageDump is the generate sibling of
// TestConfigUnknownSubcommandSkipsUsageDump.
func TestGenerateUnknownSubcommandSkipsUsageDump(t *testing.T) {
	var err error

	stderr := captureZeverStderr(t, func() {
		err = runGenerate([]string{"entitty"})
	})
	if err == nil {
		t.Fatal("runGenerate entitty: got nil error")
	}

	if !strings.Contains(err.Error(), `unknown subcommand "entitty"`) {
		t.Fatalf("error = %q, want the unknown subcommand", err.Error())
	}

	if !strings.Contains(err.Error(), `did you mean "entity"?`) {
		t.Fatalf("error lacks the did-you-mean hint: %q", err.Error())
	}

	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("stderr carries a usage dump:\n%s", stderr)
	}
}
