package parser

import "testing"

func TestParseFeatureRef(t *testing.T) {
	cases := []struct {
		in       string
		wantErr  bool
		prefix   string
		slug     string
		kind     FeatureRefKind
	}{
		{in: "@feat", slug: "feat", kind: RefKindBare},
		{in: "feat", slug: "feat", kind: RefKindBare},
		{in: "@init/feat", slug: "init/feat", kind: RefKindBare},
		{in: "init/feat", slug: "init/feat", kind: RefKindBare},
		{in: "web:@feat", prefix: "web", slug: "feat", kind: RefKindPrefixed},
		{in: "web:@parlay-tool/multi-root", prefix: "web", slug: "parlay-tool/multi-root", kind: RefKindPrefixed},
		{in: "web:feat", prefix: "web", slug: "feat", kind: RefKindPrefixed},
		// Bad prefix (uppercase) — colon is treated as slug content, not a prefix marker.
		{in: "Web:@feat", slug: "Web:@feat", kind: RefKindBare},
		// Empty cases.
		{in: "", wantErr: true},
		{in: "@", wantErr: true},
		{in: "web:@", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseFeatureRef(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %+v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got.RootPrefix != tc.prefix {
				t.Errorf("prefix: want %q, got %q", tc.prefix, got.RootPrefix)
			}
			if got.FeatureSlug != tc.slug {
				t.Errorf("slug: want %q, got %q", tc.slug, got.FeatureSlug)
			}
			if got.Kind != tc.kind {
				t.Errorf("kind: want %q, got %q", tc.kind, got.Kind)
			}
			if got.Raw != tc.in {
				t.Errorf("raw: want %q, got %q", tc.in, got.Raw)
			}
		})
	}
}

func TestFeatureSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"@feat", "feat"},
		{"feat", "feat"},
		{"@init/feat", "init/feat"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := FeatureSlug(tc.in); got != tc.want {
			t.Errorf("FeatureSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateNoCrossRootRefsInContent(t *testing.T) {
	body := []byte("Some line.\n" +
		"This refers to web:@some-feature for context.\n" +
		"Plain @feat is fine and should be ignored.\n" +
		"And here: api:@another-feature/intent — bad.\n" +
		"URL like https://example.com/foo:@bar should not match (covered by anchor).\n")
	errs := ValidateNoCrossRootRefsInContent("/some/file.md", body)
	if len(errs) != 2 {
		t.Fatalf("expected 2 violations, got %d: %+v", len(errs), errs)
	}
	if errs[0].Line != 2 || errs[0].Ref != "web:@some-feature" {
		t.Errorf("first violation: %+v", errs[0])
	}
	if errs[1].Line != 4 || errs[1].Ref != "api:@another-feature/intent" {
		t.Errorf("second violation: %+v", errs[1])
	}
	if errs[0].File != "/some/file.md" {
		t.Errorf("file path not propagated: %s", errs[0].File)
	}
}

func TestValidateNoCrossRootRefsInContent_EmptyBody(t *testing.T) {
	if errs := ValidateNoCrossRootRefsInContent("x.md", nil); errs != nil {
		t.Errorf("expected nil, got %+v", errs)
	}
	if errs := ValidateNoCrossRootRefsInContent("x.md", []byte{}); errs != nil {
		t.Errorf("expected nil, got %+v", errs)
	}
}
