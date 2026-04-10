package web

import (
	"log"
	"sync"

	"umberrelay/internal/store"
)

type queryFeedFetcher func(afterID int64, limit int) ([]store.Query, error)

type queryStreamHub struct {
	fetch     queryFeedFetcher
	batchSize int

	mu          sync.Mutex
	subscribers map[int]chan store.Query
	nextSubID   int
	lastID      int64
	running     bool
	stop        chan struct{}
	wake        chan struct{}
}

func newQueryStreamHub(fetch queryFeedFetcher, batchSize int) *queryStreamHub {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &queryStreamHub{
		fetch:       fetch,
		batchSize:   batchSize,
		subscribers: make(map[int]chan store.Query),
		stop:        make(chan struct{}),
		wake:        make(chan struct{}, 1),
	}
}

func (h *queryStreamHub) startLocked() {
	if h.running {
		return
	}
	h.running = true
	go h.run()
}

func (h *queryStreamHub) run() {
	for {
		select {
		case <-h.wake:
			h.pollOnce()
		case <-h.stop:
			h.closeAllSubscribers()
			return
		}
	}
}

func (h *queryStreamHub) closeAllSubscribers() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subscribers {
		close(ch)
		delete(h.subscribers, id)
	}
}

func (h *queryStreamHub) Close() {
	select {
	case <-h.stop:
		return
	default:
		close(h.stop)
	}
}

func (h *queryStreamHub) Subscribe() (<-chan store.Query, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.startLocked()

	id := h.nextSubID
	h.nextSubID++
	ch := make(chan store.Query, 256)
	h.subscribers[id] = ch
	return ch, func() {
		h.unsubscribe(id)
	}
}

func (h *queryStreamHub) unsubscribe(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.subscribers[id]
	if !ok {
		return
	}
	delete(h.subscribers, id)
	close(ch)
}

func (h *queryStreamHub) AdvanceCursor(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if id > h.lastID {
		h.lastID = id
	}
}

func (h *queryStreamHub) NotifyNewQueries() {
	select {
	case <-h.stop:
		return
	default:
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *queryStreamHub) pollOnce() {
	h.mu.Lock()
	lastID := h.lastID
	hasSubscribers := len(h.subscribers) > 0
	h.mu.Unlock()

	if !hasSubscribers {
		return
	}

	queries, err := h.fetch(lastID, h.batchSize)
	if err != nil {
		log.Printf("query stream poll: %v", err)
		return
	}
	if len(queries) == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, query := range queries {
		if query.ID <= h.lastID {
			continue
		}
		h.lastID = query.ID
		for id, subscriber := range h.subscribers {
			select {
			case subscriber <- query:
			default:
				close(subscriber)
				delete(h.subscribers, id)
			}
		}
	}
}
