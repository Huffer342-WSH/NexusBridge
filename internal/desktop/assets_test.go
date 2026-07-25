package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSPAAssetFallback(t *testing.T) {
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<main>NexusBridge</main>"))
		case "/assets/app.js":
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte("export {}"))
		default:
			http.NotFound(w, r)
		}
	})
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	handler := APIMiddleware(api)(SPAAssetFallback(assets))

	tests := []struct {
		name        string
		path        string
		accept      string
		status      int
		contentType string
		body        string
	}{
		{
			name: "history route falls back to index",
			path: "/settings/network", accept: "text/html",
			status: http.StatusOK, contentType: "text/html; charset=utf-8", body: "<main>NexusBridge</main>",
		},
		{
			name: "existing asset is served directly",
			path: "/assets/app.js", accept: "*/*",
			status: http.StatusOK, contentType: "text/javascript", body: "export {}",
		},
		{
			name: "missing asset remains not found",
			path: "/assets/missing.js", accept: "text/html",
			status: http.StatusNotFound, contentType: "text/plain; charset=utf-8", body: "404 page not found\n",
		},
		{
			name: "api bypasses asset fallback",
			path: "/api/health", accept: "text/html",
			status: http.StatusOK, contentType: "application/json", body: `{"status":"ok"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("Accept", tt.accept)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != tt.contentType {
				t.Fatalf("Content-Type = %q, want %q", contentType, tt.contentType)
			}
			if body := response.Body.String(); body != tt.body {
				t.Fatalf("body = %q, want %q", body, tt.body)
			}
		})
	}
}
