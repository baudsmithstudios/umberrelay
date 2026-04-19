package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"umberrelay/internal/classify"
	"umberrelay/internal/config"
	"umberrelay/internal/demo"
	"umberrelay/internal/device"
	"umberrelay/internal/dns"
	"umberrelay/internal/pipeline"
	"umberrelay/internal/store"
	"umberrelay/internal/web"
)

const (
	runtimeName       = "umberrelay"
	defaultConfigPath = "/etc/umberrelay/config.toml"
	defaultDBName     = "umberrelay.db"
)

var version = "dev"

func main() {
	configPath := flag.String("config", defaultConfigPath, "path to config file")
	demoData := flag.Bool("demo-data", false, "seed demo data into an empty database for local UI review")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if err := run(*configPath, *demoData); err != nil {
		log.Printf("%s failed: %v", runtimeName, err)
		os.Exit(1)
	}
}

func run(configPath string, demoData bool) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, defaultDBName)
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if demoData {
		if err := demo.Seed(db, time.Now()); err != nil {
			return fmt.Errorf("seed demo data: %w", err)
		}
		log.Printf("demo data ready in %s", dbPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		log.Println("shutting down...")
		cancel()
	}()

	// Device tracker
	oui := device.DefaultOUIDB()
	tracker := device.NewTracker(db, oui)
	if !demoData {
		go tracker.Run(ctx)
	}

	// Classification manager
	mgr := classify.NewManager(db)

	if err := mgr.LoadOverrides(); err != nil {
		log.Printf("load overrides: %v", err)
	}

	cached, err := mgr.LoadFromCache()
	if err != nil {
		log.Printf("load list cache: %v", err)
	}

	sources := defaultListSources(db)
	if cached == 0 && !demoData {
		log.Println("no list cache found, fetching lists immediately")
		mgr.Refresh(ctx, sources)
	} else {
		log.Printf("loaded %d domains from list cache", cached)
	}
	refreshHours := 24
	if val, err := db.GetConfig("list_refresh_hours"); err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			refreshHours = n
		}
	}
	if !demoData {
		go mgr.Run(ctx, sources, time.Duration(refreshHours)*time.Hour)
	}

	// Web server
	srv := web.NewServer(db, mgr)

	// DNS listener + async writer
	records := make(chan dns.QueryRecord, 4096)
	errCh := make(chan error, 2)
	if !demoData {
		listener, err := dns.NewListener(cfg.Listen, cfg.Upstream, records)
		if err != nil {
			return fmt.Errorf("create dns listener: %w", err)
		}
		go func() {
			if err := listener.Run(ctx); err != nil && ctx.Err() == nil {
				select {
				case errCh <- fmt.Errorf("dns listener: %w", err):
				default:
				}
				cancel()
			}
		}()
	}

	writer := pipeline.NewWriter(records, db, tracker, mgr, pipeline.Config{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		OnFlush:       srv.NotifyNewQueries,
	})
	if !demoData {
		go writer.Run(ctx)
	}

	// Purge goroutine
	if !demoData {
		go runPurge(ctx, db)
	}

	go func() {
		addr := httpAddr(cfg.HTTPListen, cfg.HTTPPort)
		if shouldWarnHTTPExposure(cfg.HTTPListen) {
			log.Printf(
				"warning: web ui/api exposed on %s without built-in auth; use a reverse proxy with auth or bind http_listen to localhost",
				addr,
			)
		}
		log.Printf("web ui: http://%s", addr)
		if err := srv.ListenAndServe(ctx, addr); err != nil && ctx.Err() == nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			select {
			case errCh <- fmt.Errorf("web server: %w", err):
			default:
			}
			cancel()
		}
	}()

	if demoData {
		log.Printf("%s demo mode started (http=%s)", runtimeName, httpAddr(cfg.HTTPListen, cfg.HTTPPort))
	} else {
		log.Printf("%s started (dns=%s, upstream=%v)", runtimeName, cfg.Listen, cfg.Upstream)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errCh:
	}
	cancel()
	select {
	case err := <-errCh:
		if runErr == nil {
			runErr = err
		}
	default:
	}
	log.Println("shutdown complete")
	return runErr
}

func httpAddr(listen string, port int) string {
	return net.JoinHostPort(listen, strconv.Itoa(port))
}

func shouldWarnHTTPExposure(listen string) bool {
	host := strings.TrimSpace(listen)
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return !ip.IsLoopback()
}

func runPurge(ctx context.Context, db *store.DB) {
	purge(db)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge(db)
		}
	}
}

func purge(db *store.DB) {
	const purgeBatchSize = 5000

	days := 30
	val, err := db.GetConfig("retention_days")
	if err == nil && val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			days = n
		}
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var totalDeleted int64
	for {
		deleted, err := db.PurgeQueriesOlderThanChunk(cutoff, purgeBatchSize)
		if err != nil {
			log.Printf("purge: %v", err)
			return
		}
		totalDeleted += deleted
		if deleted < purgeBatchSize {
			break
		}
	}
	if totalDeleted > 0 {
		log.Printf("purge: deleted %d rows", totalDeleted)
	}
}

func defaultListSources(db *store.DB) []classify.ListSource {
	lists, err := db.ListLists()
	if err != nil || len(lists) == 0 {
		defaults := []struct {
			url, name, category string
		}{
			{
				"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
				"Steven Black Unified",
				"tracking",
			},
			{
				"https://v.firebog.net/hosts/Easyprivacy.txt",
				"EasyPrivacy",
				"analytics",
			},
			{
				"https://s3.amazonaws.com/lists.disconnect.me/simple_tracking.txt",
				"Disconnect.me Tracking",
				"tracking",
			},
		}
		for _, d := range defaults {
			_, err := db.AddList(d.url, d.name, d.category)
			if err != nil {
				log.Printf("seed list %s: %v", d.name, err)
				continue
			}
		}
	}
	enabledLists, err := db.ListEnabledLists()
	if err != nil {
		log.Printf("load enabled lists: %v", err)
		return nil
	}
	return classify.SourcesFromListEntries(enabledLists)
}
