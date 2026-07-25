package desktop

import (
	"net/http"
	"path"
	"strings"
)

// SPAAssetFallback 为 Wails 静态资源服务器补充前端 History 路由回退。
func SPAAssetFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isHTMLNavigation(r) {
			next.ServeHTTP(w, r)
			return
		}

		originalHeader := w.Header().Clone()
		probe := &notFoundProbeWriter{ResponseWriter: w}
		next.ServeHTTP(probe, r)
		if !probe.notFound {
			return
		}

		restoreHeader(w.Header(), originalHeader)
		fallback := r.Clone(r.Context())
		fallback.URL.Path = "/"
		fallback.URL.RawPath = ""
		next.ServeHTTP(w, fallback)
	})
}

func isHTMLNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if path.Ext(r.URL.Path) != "" {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

type notFoundProbeWriter struct {
	http.ResponseWriter
	wroteHeader bool
	notFound    bool
}

func (w *notFoundProbeWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *notFoundProbeWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if status == http.StatusNotFound {
		w.notFound = true
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *notFoundProbeWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.notFound {
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func restoreHeader(target, original http.Header) {
	for key := range target {
		target.Del(key)
	}
	for key, values := range original {
		target[key] = append([]string(nil), values...)
	}
}
