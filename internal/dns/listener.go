package dns

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	mdns "github.com/miekg/dns"
)

// QueryRecord is emitted for each DNS query received.
type QueryRecord struct {
	SourceIP  string
	Domain    string
	QueryType string
	Timestamp time.Time
}

// Listener handles DNS queries on UDP and TCP, forwards to upstream, and emits records.
type Listener struct {
	addr     string
	upstream []string
	out      chan<- QueryRecord
	ready    chan struct{}
}

// NewListener creates a DNS listener bound to the given address.
func NewListener(addr string, upstream []string, out chan<- QueryRecord) (*Listener, error) {
	l := &Listener{
		addr:     addr,
		upstream: upstream,
		out:      out,
		ready:    make(chan struct{}),
	}
	return l, nil
}

// Run starts both UDP and TCP DNS servers. Blocks until ctx is cancelled.
func (l *Listener) Run(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", l.addr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}

	// Resolve the OS-assigned address from the UDP listener
	boundAddr := pc.LocalAddr().String()

	ln, err := net.Listen("tcp", boundAddr)
	if err != nil {
		pc.Close()
		return fmt.Errorf("listen tcp: %w", err)
	}

	l.addr = boundAddr
	close(l.ready)

	mux := mdns.NewServeMux()
	mux.HandleFunc(".", l.handleQuery)

	udpServer := &mdns.Server{PacketConn: pc, Handler: mux}
	tcpServer := &mdns.Server{Listener: ln, Handler: mux}

	go func() {
		<-ctx.Done()
		udpServer.Shutdown()
		tcpServer.Shutdown()
	}()

	errCh := make(chan error, 2)
	go func() { errCh <- udpServer.ActivateAndServe() }()
	go func() { errCh <- tcpServer.ActivateAndServe() }()

	// Wait for the first server to exit (on shutdown, both will stop)
	return <-errCh
}

// Addr returns the listener's bound address. Blocks until the listener is ready.
func (l *Listener) Addr() string {
	<-l.ready
	return l.addr
}

func (l *Listener) handleQuery(w mdns.ResponseWriter, r *mdns.Msg) {
	// Extract source IP
	sourceIP := ""
	switch addr := w.RemoteAddr().(type) {
	case *net.UDPAddr:
		sourceIP = addr.IP.String()
	case *net.TCPAddr:
		sourceIP = addr.IP.String()
	}

	// Determine upstream network from writer type
	upstreamNet := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		upstreamNet = "tcp"
	}

	// Forward to upstream with sequential fallback
	var resp *mdns.Msg
	client := &mdns.Client{Net: upstreamNet, Timeout: 2 * time.Second}
	for _, upstream := range l.upstream {
		var err error
		resp, _, err = client.Exchange(r, upstream)
		if err == nil && resp != nil {
			break
		}
	}

	if resp == nil {
		// All upstreams failed — return SERVFAIL
		log.Printf("all upstreams failed for %s", formatQuestion(r))
		m := new(mdns.Msg)
		m.SetRcode(r, mdns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}

	w.WriteMsg(resp)

	// Emit query record (non-blocking)
	if len(r.Question) > 0 {
		q := r.Question[0]
		rec := QueryRecord{
			SourceIP:  sourceIP,
			Domain:    q.Name,
			QueryType: mdns.TypeToString[q.Qtype],
			Timestamp: time.Now(),
		}
		select {
		case l.out <- rec:
		default:
			// Channel full — drop record rather than block DNS
		}
	}
}

func formatQuestion(r *mdns.Msg) string {
	if len(r.Question) == 0 {
		return "<empty>"
	}
	q := r.Question[0]
	return fmt.Sprintf("%s %s", q.Name, mdns.TypeToString[q.Qtype])
}
