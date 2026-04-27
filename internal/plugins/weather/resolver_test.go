package weather

import "testing"

func TestResolverResolve(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()

	cases := []struct {
		input      string
		wantName   string
		wantAdcode string
	}{
		{input: "上海", wantName: "上海市", wantAdcode: "310000"},
		{input: "上海市", wantName: "上海市", wantAdcode: "310000"},
		{input: "北京", wantName: "北京市", wantAdcode: "110000"},
		{input: "110101", wantName: "110101", wantAdcode: "110101"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got, ok := resolver.Resolve(tc.input)
			if !ok {
				t.Fatalf("Resolve(%q) ok = false", tc.input)
			}
			if got.Name != tc.wantName {
				t.Fatalf("Resolve(%q) name = %q, want %q", tc.input, got.Name, tc.wantName)
			}
			if got.Adcode != tc.wantAdcode {
				t.Fatalf("Resolve(%q) adcode = %q, want %q", tc.input, got.Adcode, tc.wantAdcode)
			}
		})
	}
}

func TestResolverRejectsUnknownCity(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	if _, ok := resolver.Resolve("火星市"); ok {
		t.Fatal("Resolve(火星市) ok = true, want false")
	}
}
