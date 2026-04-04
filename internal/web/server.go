package web

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"umberrelay/internal/classify"
	"umberrelay/internal/store"
	"umberrelay/internal/web/static"
)

// Server provides the HTTP interface for the web UI and API.
type Server struct {
	db       *store.DB
	classify *classify.Manager
	queryHub *queryStreamHub
	mux      *http.ServeMux
	pages    map[string]*template.Template
	now      func() time.Time
}

// NewServer creates an HTTP server with all routes registered.
func NewServer(db *store.DB, classify *classify.Manager) *Server {
	s := &Server{
		db:       db,
		classify: classify,
		queryHub: newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
			return db.QueryFeed(afterID, store.QueryFeedFilter{}, limit)
		}, time.Second, 500),
		mux:   http.NewServeMux(),
		pages: parsePages(),
		now:   time.Now,
	}
	s.registerRoutes()
	return s
}

// parsePages builds a per-page template map: layout + one page template each.
func parsePages() map[string]*template.Template {
	base := template.Must(template.ParseFS(static.FS, "templates/layout.html", "templates/components.html"))
	pageFiles := []string{"home", "devices", "device_detail", "privacy", "settings"}
	pages := make(map[string]*template.Template, len(pageFiles)+1)
	for _, name := range pageFiles {
		t := template.Must(template.Must(base.Clone()).ParseFS(static.FS, "templates/"+name+".html"))
		pages[name] = t
	}
	pages["fragments"] = template.Must(base.Clone())
	return pages
}

func (s *Server) registerRoutes() {
	// Static assets
	staticFS, _ := fs.Sub(static.FS, ".")
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// API routes
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
	s.mux.HandleFunc("PUT /api/settings", s.handleAPIUpdateSettings)
	s.mux.HandleFunc("GET /api/lists", s.handleAPIListLists)
	s.mux.HandleFunc("POST /api/lists", s.handleAPIAddList)
	s.mux.HandleFunc("PUT /api/lists/{id}", s.handleAPIUpdateList)
	s.mux.HandleFunc("DELETE /api/lists/{id}", s.handleAPIDeleteList)
	s.mux.HandleFunc("POST /api/lists/refresh", s.handleAPIRefreshLists)
	s.mux.HandleFunc("PUT /api/overrides/{domain}", s.handleAPISetOverride)
	s.mux.HandleFunc("DELETE /api/overrides/{domain}", s.handleAPIDeleteOverride)

	// Page routes
	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /devices", s.handleDevices)
	s.mux.HandleFunc("GET /devices/{mac}", s.handleDeviceDetail)
	s.mux.HandleFunc("GET /domains", s.handlePrivacy)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("GET /ui/privacy/device/{mac}", s.handlePrivacyDevice)
	s.mux.HandleFunc("GET /ui/privacy/device-all", s.handlePrivacyDeviceAll)
	s.mux.HandleFunc("POST /ui/settings", s.handleUIUpdateSettings)
	s.mux.HandleFunc("POST /ui/devices/{mac}/label", s.handleUIUpdateDeviceLabel)
	s.mux.HandleFunc("POST /ui/sources/{ip}/label", s.handleUIUpdateSourceLabel)
	s.mux.HandleFunc("POST /ui/overrides/{domain}", s.handleUISetOverride)
	s.mux.HandleFunc("POST /ui/lists", s.handleUIAddList)
	s.mux.HandleFunc("POST /ui/lists/{id}/enabled", s.handleUIUpdateList)
	s.mux.HandleFunc("POST /ui/lists/{id}/delete", s.handleUIDeleteList)
	s.mux.HandleFunc("POST /ui/lists/refresh", s.handleUIRefreshLists)
}

// Handler returns the HTTP handler with all routes.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Close releases server-owned background resources.
func (s *Server) Close() {
	if s.queryHub != nil {
		s.queryHub.Close()
	}
}

// NotifyNewQueries wakes the live stream hub after successful query writes.
func (s *Server) NotifyNewQueries() {
	if s.queryHub != nil {
		s.queryHub.NotifyNewQueries()
	}
}

func (s *Server) httpServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Keep WriteTimeout disabled so SSE streams are not cut off.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
}

// ListenAndServe starts the HTTP server, blocking until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := s.httpServer(addr)
	go func() {
		<-ctx.Done()
		s.Close()
		srv.Close()
	}()
	return srv.ListenAndServe()
}
