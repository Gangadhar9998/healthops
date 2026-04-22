package monitoring

import "testing"

func TestIsProtectedEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/v1/checks", want: true},
		{path: "/api/v1/checks/check-1", want: true},
		{path: "/api/v1/config", want: true},
		{path: "/api/v1/config/runtime", want: true},
		{path: "/api/v1/servers", want: true},
		{path: "/api/v1/notification-channels", want: true},
		{path: "/api/v1/summary", want: false},
		{path: "/healthz", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := isProtectedEndpoint(tc.path); got != tc.want {
				t.Fatalf("isProtectedEndpoint(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
