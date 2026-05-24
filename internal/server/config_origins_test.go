package server

import (
	"reflect"
	"testing"
)

func TestClientOriginsFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		legacy  string
		want    []string
	}{
		{
			name: "default origins",
			want: []string{"http://localhost:3000", "http://localhost:3001"},
		},
		{
			name:   "legacy single origin",
			legacy: "https://legacy.example.com",
			want:   []string{"https://legacy.example.com"},
		},
		{
			name:    "comma separated origins are trimmed",
			origins: " https://app.example.com, ,https://admin.example.com ",
			want:    []string{"https://app.example.com", "https://admin.example.com"},
		},
		{
			name:    "origin list takes precedence",
			origins: "https://primary.example.com",
			legacy:  "https://legacy.example.com",
			want:    []string{"https://primary.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLIENT_ORIGIN_URLS", tt.origins)
			t.Setenv("CLIENT_ORIGIN_URL", tt.legacy)

			got := ClientOriginsFromEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ClientOriginsFromEnv() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
