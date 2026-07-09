// runtime.go 负责桌面端共享核心服务和 HTTP 适配生命周期。
package desktop

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"nexusbridge/internal/config"
	"nexusbridge/internal/core"
	"nexusbridge/internal/logging"
	"nexusbridge/internal/runtimeconfig"
	"nexusbridge/internal/server"
)

// Options 描述桌面端启动时允许的数据与配置覆盖项。
type Options struct {
	DataDir        string
	ConfigPath     string
	ExecutablePath string
}

// PreparedData 表示桌面端已经准备好的持久化路径与配置。
type PreparedData = runtimeconfig.Result

// Runtime 管理桌面端共享核心、HTTP Handler 与清理流程。
type Runtime struct {
	DataDir    string
	ConfigPath string
	Config     config.Config
	handler    http.Handler
	app        *core.App
	cancel     context.CancelFunc
	cleanupLog func() error
	closeOnce  sync.Once
	closeErr   error
}

// HandlerSlot 在 Wails 单实例检查完成后接入共享后端 Handler。
type HandlerSlot struct {
	mu      sync.RWMutex
	handler http.Handler
}

// PrepareData 使用统一运行配置策略准备桌面数据。
func PrepareData(opts Options) (PreparedData, error) {
	return runtimeconfig.Load(runtimeconfig.Options{
		ExplicitConfig: opts.ConfigPath,
		DataDir:        opts.DataDir,
		ExecutablePath: opts.ExecutablePath,
	})
}

// Start 创建桌面端共享核心并返回与浏览器版一致的 HTTP Handler。
func Start(parent context.Context, opts Options) (*Runtime, error) {
	prepared, err := PrepareData(opts)
	if err != nil {
		return nil, err
	}
	cleanupLog, err := logging.Setup(prepared.Config.Logging)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	app, err := core.NewApp(ctx, prepared.Config)
	if err != nil {
		cancel()
		_ = cleanupLog()
		return nil, err
	}
	return &Runtime{
		DataDir: prepared.DataDir, ConfigPath: prepared.ConfigPath, Config: prepared.Config,
		handler: server.New(prepared.Config, app, "").Handler(), app: app, cancel: cancel, cleanupLog: cleanupLog,
	}, nil
}

// Handler 返回供 Wails AssetServer 转发的完整应用 API。
func (r *Runtime) Handler() http.Handler {
	return r.handler
}

// Set 更新 HandlerSlot 当前转发的后端 Handler。
func (s *HandlerSlot) Set(handler http.Handler) {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
}

// ServeHTTP 将请求转发给已经就绪的桌面后端。
func (s *HandlerSlot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	handler := s.handler
	s.mu.RUnlock()
	if handler == nil {
		http.Error(w, "desktop backend is starting", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

// Close 停止桌面后台任务并安全关闭数据库和日志。
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.cancel()
		r.closeErr = errors.Join(r.app.Close(), r.cleanupLog())
	})
	return r.closeErr
}

// APIMiddleware 将桌面端 API 和 RSS 请求交给共享 HTTP Server。
func APIMiddleware(api http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isApplicationRoute(r.URL.Path) {
				api.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isApplicationRoute 判断请求是否属于共享后端路由。
func isApplicationRoute(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") || path == "/rss" || strings.HasPrefix(path, "/rss/")
}
