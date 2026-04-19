package dns

import (
	"context"
	"net"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

func TestListenerForwardsUDPQuery(t *testing.T) {
	requirePacketConn(t, "udp", "127.0.0.1:0")

	upstreamAddr := startFakeUpstream(t, "udp")

	records := make(chan QueryRecord, 10)
	l, err := NewListener("127.0.0.1:0", []string{upstreamAddr}, records)
	if err != nil {
		t.Fatalf("NewListener: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx)

	// Wait for listener to be ready
	addr := l.Addr()
	if addr == "" {
		t.Fatal("listener addr is empty")
	}

	// Send a DNS query over UDP
	c := &mdns.Client{Net: "udp"}
	m := new(mdns.Msg)
	m.SetQuestion("example.com.", mdns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.Rcode != mdns.RcodeSuccess {
		t.Errorf("Rcode = %d, want success", resp.Rcode)
	}

	// Verify a record was emitted
	select {
	case rec := <-records:
		if rec.Domain != "example.com." {
			t.Errorf("Domain = %q, want example.com.", rec.Domain)
		}
		if rec.QueryType != "A" {
			t.Errorf("QueryType = %q, want A", rec.QueryType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for query record")
	}
}

func TestListenerForwardsTCPQuery(t *testing.T) {
	requireListener(t, "tcp", "127.0.0.1:0")

	upstreamAddr := startFakeUpstream(t, "tcp")

	records := make(chan QueryRecord, 10)
	l, err := NewListener("127.0.0.1:0", []string{upstreamAddr}, records)
	if err != nil {
		t.Fatalf("NewListener: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx)

	addr := l.Addr()
	if addr == "" {
		t.Fatal("listener addr is empty")
	}

	// Send a DNS query over TCP
	c := &mdns.Client{Net: "tcp"}
	m := new(mdns.Msg)
	m.SetQuestion("tcp.example.com.", mdns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.Rcode != mdns.RcodeSuccess {
		t.Errorf("Rcode = %d, want success", resp.Rcode)
	}

	select {
	case rec := <-records:
		if rec.Domain != "tcp.example.com." {
			t.Errorf("Domain = %q, want tcp.example.com.", rec.Domain)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for query record")
	}
}

func TestListenerUpstreamFailure(t *testing.T) {
	requirePacketConn(t, "udp", "127.0.0.1:0")

	records := make(chan QueryRecord, 10)
	// Point to a non-existent upstream
	l, err := NewListener("127.0.0.1:0", []string{"127.0.0.1:1"}, records)
	if err != nil {
		t.Fatalf("NewListener: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go l.Run(ctx)

	c := &mdns.Client{Net: "udp"}
	c.Timeout = 3 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion("fail.example.com.", mdns.TypeA)
	resp, _, err := c.Exchange(m, l.Addr())
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if resp.Rcode != mdns.RcodeServerFailure {
		t.Errorf("Rcode = %d, want SERVFAIL (%d)", resp.Rcode, mdns.RcodeServerFailure)
	}
}

func TestFormatQuestionForLogOmitsDomainName(t *testing.T) {
	msg := new(mdns.Msg)
	msg.SetQuestion("sensitive.example.com.", mdns.TypeAAAA)

	got := formatQuestionForLog(msg)
	if got != "AAAA" {
		t.Fatalf("formatQuestionForLog() = %q, want AAAA", got)
	}
}

func requirePacketConn(t *testing.T, network, address string) {
	t.Helper()

	pc, err := net.ListenPacket(network, address)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	pc.Close()
}

func requireListener(t *testing.T, network, address string) {
	t.Helper()

	ln, err := net.Listen(network, address)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	ln.Close()
}

// startFakeUpstream runs a DNS server on the given network that responds with 127.0.0.1 to any A query.
func startFakeUpstream(t *testing.T, network string) string {
	t.Helper()

	handler := mdns.HandlerFunc(func(w mdns.ResponseWriter, r *mdns.Msg) {
		m := new(mdns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		if len(r.Question) > 0 && r.Question[0].Qtype == mdns.TypeA {
			m.Answer = append(m.Answer, &mdns.A{
				Hdr: mdns.RR_Header{Name: r.Question[0].Name, Rrtype: mdns.TypeA, Class: mdns.ClassINET, Ttl: 60},
				A:   net.IPv4(127, 0, 0, 1),
			})
		}
		w.WriteMsg(m)
	})

	switch network {
	case "udp":
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := &mdns.Server{PacketConn: pc, Handler: handler}
		go server.ActivateAndServe()
		t.Cleanup(func() { server.Shutdown() })
		return pc.LocalAddr().String()
	case "tcp":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		server := &mdns.Server{Listener: ln, Handler: handler}
		go server.ActivateAndServe()
		t.Cleanup(func() { server.Shutdown() })
		return ln.Addr().String()
	default:
		t.Fatalf("unsupported network: %s", network)
		return ""
	}
}
