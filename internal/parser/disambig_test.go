package parser

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/config"
)

func TestLetterForAndIndexForLetter(t *testing.T) {
	for i := 0; i < 26; i++ {
		l := LetterFor(i)
		if got := IndexForLetter(l); got != i {
			t.Errorf("round-trip %d: letter %q → %d", i, l, got)
		}
	}
	if LetterFor(-1) != "" {
		t.Errorf("negative index should return empty")
	}
	if LetterFor(26) != "" {
		t.Errorf("index >= 26 should return empty")
	}
	if IndexForLetter("") != -1 || IndexForLetter("ZZ") != -1 || IndexForLetter("1") != -1 {
		t.Errorf("invalid inputs should return -1")
	}
	if IndexForLetter("a") != 0 {
		t.Errorf("lowercase letters should be accepted")
	}
}

func TestFormatCandidateList(t *testing.T) {
	got := FormatCandidateList([]config.Candidate{
		{Name: "web", RelativePath: "apps/web"},
		{Name: "api", RelativePath: "apps/api"},
	})
	if !strings.Contains(got, "A: web (apps/web)") {
		t.Errorf("missing first entry: %s", got)
	}
	if !strings.Contains(got, "B: api (apps/api)") {
		t.Errorf("missing second entry: %s", got)
	}
}

func TestFormatCandidateList_EmptyAndCollapsedRelPath(t *testing.T) {
	if got := FormatCandidateList(nil); got != "" {
		t.Errorf("nil should be empty, got %q", got)
	}
	// When name == relative path, render once.
	got := FormatCandidateList([]config.Candidate{
		{Name: "web", RelativePath: "web"},
	})
	if strings.Contains(got, "(web)") {
		t.Errorf("name == path should not duplicate: %s", got)
	}
	if !strings.Contains(got, "A: web") {
		t.Errorf("expected single-name entry: %s", got)
	}
}
