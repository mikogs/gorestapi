package urlpath

import "testing"

func TestID(t *testing.T) {
	tests := map[string]struct {
		path   string
		wantID string
		wantOK bool
	}{
		"no ID":               {"", "", true},
		"no ID with a slash":  {"/", "", true},
		"number":              {"123", "123", true},
		"number with slashes": {"123///", "123", true},
		"UUID":                {"0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d", "0190f5c3-6d1e-7b3a-9c4d-2e5f6a7b8c9d", true},
		"text":                {"abc", "abc", true},
		"two segments":        {"1/2", "", false},
		"leading slash":       {"/123", "", false},
		"other characters":    {"12;drop", "", false},
		"encoded characters":  {"12%20", "", false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if gotID, gotOK := ID(test.path); gotID != test.wantID || gotOK != test.wantOK {
				t.Fatalf("ID(%q) returned %q, %v, want %q, %v", test.path, gotID, gotOK, test.wantID, test.wantOK)
			}
		})
	}
}
