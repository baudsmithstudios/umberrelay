package web

import (
	"context"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"umberrelay/internal/app"
	"umberrelay/internal/classify"
	"umberrelay/internal/store"
	"umberrelay/internal/web/static"
)

const httpShutdownTimeout = 5 * time.Second
const maxMutationBodyBytes int64 = 1 << 20

type Server struct {
	db                     *store.DB
	classify               *classify.Manager
	queryHub               *queryStreamHub
	mux                    *http.ServeMux
	handler                http.Handler
	pages                  map[string]*template.Template
	now                    func() time.Time
	backgroundCtx          context.Context
	backgroundCancel       context.CancelFunc
	refreshRunning         atomic.Bool
	refreshJobs            sync.WaitGroup
	loadEnabledListSources func(*store.DB) ([]classify.ListSource, error)
	refreshListSources     func(context.Context, *classify.Manager, []classify.ListSource) error
}

func NewServer(db *store.DB, classify *classify.Manager) *Server {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	s := &Server{
		db:       db,
		classify: classify,
		queryHub: newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
			return db.QueryFeed(afterID, store.QueryFeedFilter{}, limit)
		}, 500),
		mux:                    http.NewServeMux(),
		pages:                  parsePages(),
		now:                    time.Now,
		backgroundCtx:          backgroundCtx,
		backgroundCancel:       backgroundCancel,
		loadEnabledListSources: app.EnabledListSources,
		refreshListSources:     refreshManagerSources,
	}
	s.registerRoutes()
	s.handler = withSecurityHeaders(withMutationOriginGuard(withMutationBodyLimit(s.mux)))
	return s
}

func refreshManagerSources(ctx context.Context, mgr *classify.Manager, sources []classify.ListSource) error {
	return mgr.Refresh(ctx, sources)
}

func parsePages() map[string]*template.Template {
	base := template.Must(template.ParseFS(static.FS, "templates/layout.html", "templates/components.html"))
	pageFiles := []string{"home", "devices", "device_detail", "settings"}
	pages := make(map[string]*template.Template, len(pageFiles)+1)
	for _, name := range pageFiles {
		t := template.Must(template.Must(base.Clone()).ParseFS(static.FS, "templates/"+name+".html"))
		pages[name] = t
	}
	pages["fragments"] = template.Must(base.Clone())
	return pages
}

func (s *Server) registerRoutes() {
	staticFS, _ := fs.Sub(static.FS, ".")
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/summary", s.handleAPISummary)
	s.mux.HandleFunc("GET /api/devices", s.handleAPIDevices)
	s.mux.HandleFunc("GET /api/actors", s.handleAPIActors)
	s.mux.HandleFunc("GET /api/devices/{mac}", s.handleAPIDevice)
	s.mux.HandleFunc("PUT /api/devices/{mac}", s.handleAPIUpdateDevice)
	s.mux.HandleFunc("GET /api/queries", s.handleAPIQueries)
	s.mux.HandleFunc("GET /api/queries/stream", s.handleAPIQueryStream)
	s.mux.HandleFunc("GET /api/activity", s.handleAPIActivity)
	s.mux.HandleFunc("GET /api/anomalies", s.handleAPIAnomalies)
	s.mux.HandleFunc("GET /api/bypass", s.handleAPIBypass)
	s.mux.HandleFunc("GET /api/domains", s.handleAPIDomains)
	s.mux.HandleFunc("GET /api/settings", s.handleAPIGetSettings)
	s.mux.HandleFunc("GET /api/lists/status", s.handleAPIListRefreshStatus)
	s.mux.HandleFunc("PUT /api/settings", s.handleAPIUpdateSettings)
	s.mux.HandleFunc("GET /api/lists", s.handleAPIListLists)
	s.mux.HandleFunc("POST /api/lists", s.handleAPIAddList)
	s.mux.HandleFunc("PUT /api/lists/{id}", s.handleAPIUpdateList)
	s.mux.HandleFunc("DELETE /api/lists/{id}", s.handleAPIDeleteList)
	s.mux.HandleFunc("POST /api/lists/refresh", s.handleAPIRefreshLists)
	s.mux.HandleFunc("PUT /api/overrides/{domain}", s.handleAPISetOverride)
	s.mux.HandleFunc("DELETE /api/overrides/{domain}", s.handleAPIDeleteOverride)

	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /devices", s.handleDevices)
	s.mux.HandleFunc("GET /devices/{mac}", s.handleDeviceDetail)
	s.mux.HandleFunc("GET /domains", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/devices", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("POST /ui/settings", s.handleUIUpdateSettings)
	s.mux.HandleFunc("POST /ui/devices/{mac}/label", s.handleUIUpdateDeviceLabel)
	s.mux.HandleFunc("POST /ui/sources/{ip}/label", s.handleUIUpdateSourceLabel)
	s.mux.HandleFunc("POST /ui/overrides/{domain}", s.handleUISetOverride)
	s.mux.HandleFunc("POST /ui/lists", s.handleUIAddList)
	s.mux.HandleFunc("POST /ui/lists/{id}/enabled", s.handleUIUpdateList)
	s.mux.HandleFunc("POST /ui/lists/{id}/delete", s.handleUIDeleteList)
	s.mux.HandleFunc("POST /ui/lists/refresh", s.handleUIRefreshLists)
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

// Close releases server-owned background resources.
func (s *Server) Close() {
	s.backgroundCancel()
	s.refreshJobs.Wait()
	s.queryHub.Close()
}

func (s *Server) NotifyNewQueries() {
	s.queryHub.NotifyNewQueries()
}

func (s *Server) httpServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Keep WriteTimeout disabled so SSE streams are not cut off.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func withMutationOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutationMethod(r.Method) || requestIsSameOrigin(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	})
}

func withMutationBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMutationMethod(r.Method) && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxMutationBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestIsSameOrigin(r *http.Request) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		parsedOrigin, err := url.Parse(origin)
		return err == nil && sameHost(parsedOrigin.Host, r.Host)
	}

	if fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); fetchSite == "cross-site" {
		return false
	}

	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		parsedReferer, err := url.Parse(referer)
		return err == nil && sameHost(parsedReferer.Host, r.Host)
	}

	return true
}

func sameHost(a, b string) bool {
	hostA, portA := splitHostPort(a)
	hostB, portB := splitHostPort(b)
	if hostA == "" || hostB == "" || !strings.EqualFold(hostA, hostB) {
		return false
	}
	if portA != "" && portB != "" && portA != portB {
		return false
	}
	return true
}

func splitHostPort(value string) (host, port string) {
	if h, p, err := net.SplitHostPort(value); err == nil {
		return h, p
	}
	return value, ""
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := s.httpServer(addr)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
		}
		s.Close()
	}()
	return srv.ListenAndServe()
}
