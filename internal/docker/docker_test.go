package docker

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

// newTestClient returns a client for a fake daemon answering image pulls with the given stream
func newTestClient(t *testing.T, stream string) *client.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("Api-Version", client.MaxAPIVersion)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, stream)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cli, err := client.New(client.WithHost("tcp://" + strings.TrimPrefix(server.URL, "http://")))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	return cli
}

func TestTryPullWithOptions(t *testing.T) {
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
			name:    "invalid json",
			stream:  `{"status":`,
			wantErr: "unexpected EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := newTestClient(t, tt.stream)

			err := tryPullWithOptions(cli, "nginx:latest", client.ImagePullOptions{})
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
