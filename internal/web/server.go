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
	mux      *http.ServeMux
	pages    map[string]*template.Template
	now      func() time.Time
}

// NewServer creates an HTTP server with all routes registered.
func NewServer(db *store.DB, classify *classify.Manager) *Server {
	s := &Server{
		db:       db,
		classify: classify,
		mux:      http.NewServeMux(),
		pages:    parsePages(),
		now:      time.Now,
	}
	s.registerRoutes()
	return s
}

// parsePages builds a per-page template map: layout + one page template each.
func parsePages() map[string]*template.Template {
	layout := template.Must(template.ParseFS(static.FS, "templates/layout.html"))
	pageFiles := []string{"privacy", "settings"}
	pages := make(map[string]*template.Template, len(pageFiles))
	for _, name := range pageFiles {
		t := template.Must(template.Must(layout.Clone()).ParseFS(static.FS, "templates/"+name+".html"))
		pages[name] = t
	}
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
	s.mux.HandleFunc("GET /{$}", s.handlePrivacy)
	s.mux.HandleFunc("GET /devices", s.handlePrivacy)
	s.mux.HandleFunc("GET /devices/{mac}", s.handlePrivacy)
	s.mux.HandleFunc("GET /domains", s.handlePrivacy)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("GET /ui/privacy/device/{mac}", s.handlePrivacyDevice)
	s.mux.HandleFunc("GET /ui/privacy/device-all", s.handlePrivacyDeviceAll)
	s.mux.HandleFunc("POST /ui/settings", s.handleUIUpdateSettings)
	s.mux.HandleFunc("POST /ui/devices/{mac}/label", s.handleUIUpdateDeviceLabel)
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

// ListenAndServe starts the HTTP server, blocking until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	return srv.ListenAndServe()
}
