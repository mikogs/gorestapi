package urlpath

import (
	"testing"
)

func TestID(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantID     string
		wantExists bool
	}{
		{
			name:       "empty path",
			path:       "",
			wantID:     "",
			wantExists: true,
		},
		{
			name:       "root path",
			path:       "/",
			wantID:     "",
			wantExists: true,
		},
		{
			name:       "numeric ID",
			path:       "/users/123",
			wantID:     "123",
			wantExists: true,
		},
		{
			name:       "numeric ID with trailing slash",
			path:       "/users/123/",
			wantID:     "123",
			wantExists: true,
		},
		{
			name:       "non-numeric ID",
			path:       "/users/abc",
			wantID:     "",
			wantExists: false,
		},
		{
			name:       "alphanumeric ID",
			path:       "/users/123abc",
			wantID:     "",
			wantExists: false,
		},
		{
			name:       "single numeric segment",
			path:       "456",
			wantID:     "456",
			wantExists: true,
		},
		{
			name:       "single non-numeric segment",
			path:       "test",
			wantID:     "",
			wantExists: false,
		},
		{
			name:       "zero ID",
			path:       "/items/0",
			wantID:     "0",
			wantExists: true,
		},
		{
			name:       "large numeric ID",
			path:       "/posts/999999999",
			wantID:     "999999999",
			wantExists: true,
		},
		{
			name:       "multiple trailing slashes",
			path:       "/users/789///",
			wantID:     "789",
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotExists := ID(tt.path)
			if gotID != tt.wantID {
				t.Errorf("ID() gotID = %v, want %v", gotID, tt.wantID)
			}
			if gotExists != tt.wantExists {
				t.Errorf("ID() gotExists = %v, want %v", gotExists, tt.wantExists)
			}
		})
	}
}
