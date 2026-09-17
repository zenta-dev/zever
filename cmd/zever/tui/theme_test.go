package tui

import (
	"strings"
	"testing"
)

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

func TestNewThemePreservesContent(t *testing.T) {
	th := NewTheme()
	roles := map[string]string{
		"Title":   th.Title.Render("x"),
		"Cmd":     th.Cmd.Render("x"),
		"Dim":     th.Dim.Render("x"),
		"Success": th.Success.Render("x"),
		"Fail":    th.Fail.Render("x"),
		"Hint":    th.Hint.Render("x"),
		"Spinner": th.Spinner.Render("x"),
	}
	for role, got := range roles {
		t.Run(role, func(t *testing.T) {
			if stripANSI(got) != "x" {
				t.Fatalf("role %s render lost content: %q", role, got)
			}
		})
	}
}

func TestThemeBoxKeepsMultiline(t *testing.T) {
	th := NewTheme()
	got := stripANSI(th.Box.Render("a\nb"))
	if !containsStr(got, "a") || !containsStr(got, "b") {
		t.Fatalf("Box dropped content: %q", got)
	}
}
