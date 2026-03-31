package pipeline

import (
	"context"
	"log"
	"time"

	"umberrelay/internal/classify"
	"umberrelay/internal/device"
	"umberrelay/internal/dns"
	"umberrelay/internal/store"
)

// Config controls writer batching behavior.
type Config struct {
	BatchSize     int
	FlushInterval time.Duration
}

// Writer reads DNS query records from a channel, enriches them, and writes to the store.
type Writer struct {
	in       <-chan dns.QueryRecord
	db       *store.DB
	tracker  *device.Tracker
	classify *classify.Manager
	cfg      Config
}

// NewWriter creates an async writer.
func NewWriter(in <-chan dns.QueryRecord, db *store.DB, tracker *device.Tracker, classify *classify.Manager, cfg Config) *Writer {
	return &Writer{in: in, db: db, tracker: tracker, classify: classify, cfg: cfg}
}

// Run processes records until ctx is cancelled, then drains remaining records.
func (w *Writer) Run(ctx context.Context) {
	buf := make([]store.Query, 0, w.cfg.BatchSize)
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Drain remaining records from channel
			for {
				select {
				case rec, ok := <-w.in:
					if !ok {
						w.flush(buf)
						return
					}
					buf = append(buf, w.enrich(rec))
				default:
					w.flush(buf)
					return
				}
			}

		case rec, ok := <-w.in:
			if !ok {
				w.flush(buf)
				return
			}
			buf = append(buf, w.enrich(rec))
			if len(buf) >= w.cfg.BatchSize {
				w.flush(buf)
				buf = buf[:0]
			}

		case <-ticker.C:
			if len(buf) > 0 {
				w.flush(buf)
				buf = buf[:0]
			}
		}
	}
}

func (w *Writer) enrich(rec dns.QueryRecord) store.Query {
	mac := w.tracker.ResolveIP(rec.SourceIP)
	category := w.classify.Classify(rec.Domain)
	return store.Query{
		DeviceMAC: mac,
		Domain:    rec.Domain,
		QueryType: rec.QueryType,
		Category:  category,
		Timestamp: rec.Timestamp,
	}
}

func (w *Writer) flush(buf []store.Query) {
	if len(buf) == 0 {
		return
	}
	if err := w.db.WriteQueries(buf); err != nil {
		log.Printf("flush queries: %v", err)
	}
}
