package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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

func main() {
	configPath := flag.String("config", defaultConfigPath, "path to config file")
	demoData := flag.Bool("demo-data", false, "seed demo data into an empty database for local UI review")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	dbPath := filepath.Join(cfg.DataDir, defaultDBName)
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if *demoData {
		if err := demo.Seed(db, time.Now()); err != nil {
			log.Fatalf("seed demo data: %v", err)
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
	if !*demoData {
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
	if cached == 0 && !*demoData {
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
	if !*demoData {
		go mgr.Run(ctx, sources, time.Duration(refreshHours)*time.Hour)
	}

	// Web server
	srv := web.NewServer(db, mgr)

	// DNS listener + async writer
	records := make(chan dns.QueryRecord, 4096)
	if !*demoData {
		listener, err := dns.NewListener(cfg.Listen, cfg.Upstream, records)
		if err != nil {
			log.Fatalf("create dns listener: %v", err)
		}
		go func() {
			if err := listener.Run(ctx); err != nil && ctx.Err() == nil {
				log.Fatalf("dns listener: %v", err)
			}
		}()
	}

	writer := pipeline.NewWriter(records, db, tracker, mgr, pipeline.Config{
		BatchSize:     100,
		FlushInterval: 1 * time.Second,
		OnFlush:       srv.NotifyNewQueries,
	})
	if !*demoData {
		go writer.Run(ctx)
	}

	// Purge goroutine
	if !*demoData {
		go runPurge(ctx, db)
	}

	go func() {
		addr := fmt.Sprintf(":%d", cfg.HTTPPort)
		log.Printf("web ui: http://0.0.0.0%s", addr)
		if err := srv.ListenAndServe(ctx, addr); err != nil && ctx.Err() == nil {
			log.Fatalf("web server: %v", err)
		}
	}()

	if *demoData {
		log.Printf("%s demo mode started (http=:%d)", runtimeName, cfg.HTTPPort)
	} else {
		log.Printf("%s started (dns=%s, upstream=%v)", runtimeName, cfg.Listen, cfg.Upstream)
	}
	<-ctx.Done()
	log.Println("shutdown complete")
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
		var sources []classify.ListSource
		for _, d := range defaults {
			id, err := db.AddList(d.url, d.name, d.category)
			if err != nil {
				log.Printf("seed list %s: %v", d.name, err)
				continue
			}
			sources = append(sources, classify.ListSource{
				ID: id, URL: d.url, Name: d.name, Category: d.category,
			})
		}
		return sources
	}

	var sources []classify.ListSource
	for _, l := range lists {
		if l.Enabled {
			sources = append(sources, classify.ListSource{
				ID: l.ID, URL: l.URL, Name: l.Name, Category: l.Category,
			})
		}
	}
	return sources
}
