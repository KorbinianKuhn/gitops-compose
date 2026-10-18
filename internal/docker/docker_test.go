package docker

import (
	"strings"
	"testing"
)

func TestReadPullResponse(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		wantErr string
	}{
		{
			name: "success",
			stream: `{"status":"Pulling from library/nginx","id":"latest"}
{"status":"Download complete","id":"abc"}
{"status":"Status: Downloaded newer image for nginx:latest"}
`,
		},
		{
			name: "error detail in stream",
			stream: `{"status":"Pulling from library/nginx","id":"latest"}
{"errorDetail":{"message":"toomanyrequests: rate limit exceeded"},"error":"toomanyrequests: rate limit exceeded"}
`,
			wantErr: "toomanyrequests: rate limit exceeded",
		},
		{
			name:    "error message only",
			stream:  `{"error":"manifest unknown"}`,
			wantErr: "manifest unknown",
		},
		{
			name:    "invalid json",
			stream:  `{"status":`,
			wantErr: "failed to decode docker pull response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := readPullResponse(strings.NewReader(tt.stream))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
