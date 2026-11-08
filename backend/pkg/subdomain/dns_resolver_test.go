package subdomain

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hephaestus-backend/pkg/types"

	"github.com/miekg/dns"
)

// Serve both transports on one local address, without any public DNS access.
func serveTestDNS(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udp, err := net.ListenPacket("udp", tcp.Addr().String())
	if err != nil {
		tcp.Close()
		t.Fatal(err)
	}
	for _, server := range []*dns.Server{{Listener: tcp, Handler: handler}, {PacketConn: udp, Handler: handler}} {
		ready := make(chan struct{})
		server.NotifyStartedFunc = func() { close(ready) }
		go func() { _ = server.ActivateAndServe() }()
		<-ready
		t.Cleanup(func() { _ = server.Shutdown() })
	}
	return tcp.Addr().String()
}

func dnsAddressAnswer(w dns.ResponseWriter, request *dns.Msg) {
	response := new(dns.Msg)
	response.SetReply(request)
	if request.Question[0].Qtype == dns.TypeA {
		response.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.ParseIP("203.0.113.17")}}
	}
	_ = w.WriteMsg(response)
}

func dnsFailure(rcode int, calls *atomic.Int32) dns.HandlerFunc {
	return func(w dns.ResponseWriter, request *dns.Msg) {
		if calls != nil {
			calls.Add(1)
		}
		response := new(dns.Msg)
		response.SetRcode(request, rcode)
		_ = w.WriteMsg(response)
	}
}

func TestRunnerDefaultDNSRetriesThirdServerAndRetainsFailure(t *testing.T) {
	first := serveTestDNS(t, dnsFailure(dns.RcodeServerFailure, nil))
	second := serveTestDNS(t, dnsFailure(dns.RcodeRefused, nil))
	var successfulQueries atomic.Int32
	third := serveTestDNS(t, func(w dns.ResponseWriter, request *dns.Msg) {
		successfulQueries.Add(1)
		if request.Question[0].Name == "failed.example.test." {
			dnsFailure(dns.RcodeNameError, nil)(w, request)
			return
		}
		dnsAddressAnswer(w, request)
	})
	original := DefaultDnsServers
	DefaultDnsServers = []string{first, second, third}
	t.Cleanup(func() { DefaultDnsServers = original })
	var results []SubdomainResult
	// This is the no-dnsServers path used by an AI call without DNS parameters.
	engine := NewSubdomainEngine(context.Background(), types.SubdomainOption{Thread: 1, Timeout: 1}, func(event string, data interface{}) {
		if event == "subdomainLoading" {
			results = append(results, data.(SubdomainResult))
		}
	})
	engine.RunnerWithSources(context.Background(), "example.test", []string{"ok.example.test", "failed.example.test"}, "Chaos", nil)
	if len(results) != 2 {
		t.Fatalf("must retain successes and failed candidates, got %#v", results)
	}
	for _, result := range results {
		if result.Subdomain == "ok.example.test" {
			if len(result.IPs) != 1 || result.IPs[0] != "203.0.113.17" || result.ResolutionError != "" {
				t.Fatalf("third resolver did not recover the IP: %#v", result)
			}
		} else {
			for _, want := range []string{first, second, third, "SERVFAIL", "REFUSED", "NXDOMAIN", result.Subdomain} {
				if !strings.Contains(result.ResolutionError, want) {
					t.Fatalf("missing real error %q: %#v", want, result)
				}
			}
			if len(result.IPs) != 0 || result.IsWildcard {
				t.Fatalf("failed DNS must not produce an address or wildcard match: %#v", result)
			}
		}
	}
	if successfulQueries.Load() == 0 {
		t.Fatal("third DNS server was never queried")
	}
}

func TestDNSRetryHonorsConfiguredServersAndStopsOnSuccess(t *testing.T) {
	var firstCalls, secondCalls, thirdCalls atomic.Int32
	first := serveTestDNS(t, dnsFailure(dns.RcodeNameError, &firstCalls))
	second := serveTestDNS(t, func(w dns.ResponseWriter, request *dns.Msg) {
		secondCalls.Add(1)
		dnsAddressAnswer(w, request)
	})
	third := serveTestDNS(t, dnsFailure(dns.RcodeRefused, &thirdCalls))
	ips, _, err := resolveDNSWithRetry("example.test", []string{first, first, second, third}, 1)
	if err != nil || len(ips) != 1 || firstCalls.Load() != 2 || secondCalls.Load() != 2 || thirdCalls.Load() != 0 {
		t.Fatalf("unexpected failover: ips=%v err=%v calls=%d/%d/%d", ips, err, firstCalls.Load(), secondCalls.Load(), thirdCalls.Load())
	}
}

func TestDNSRetryCapsDistinctAttemptsAndPreservesTypedError(t *testing.T) {
	var extraCalls atomic.Int32
	first := serveTestDNS(t, dnsFailure(dns.RcodeNameError, nil))
	second := serveTestDNS(t, dnsFailure(dns.RcodeServerFailure, nil))
	third := serveTestDNS(t, dnsFailure(dns.RcodeRefused, nil))
	extra := serveTestDNS(t, dnsFailure(dns.RcodeNameError, &extraCalls))
	ips, _, err := resolveDNSWithRetry("example.test", []string{first, first, second, third, extra}, 1)
	var dnsErr *net.DNSError
	if len(ips) != 0 || !errors.As(err, &dnsErr) || !dnsErr.IsNotFound || extraCalls.Load() != 0 {
		t.Fatalf("expected NXDOMAIN details and max three distinct attempts: ips=%v err=%v extra=%d", ips, err, extraCalls.Load())
	}
	for _, want := range []string{first, second, third, "NXDOMAIN", "SERVFAIL", "REFUSED"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("lost DNS failure %q: %v", want, err)
		}
	}
}

func TestDNSUDPTruncationFallsBackToTCP(t *testing.T) {
	var udpCalls, tcpCalls atomic.Int32
	server := serveTestDNS(t, func(w dns.ResponseWriter, request *dns.Msg) {
		if w.LocalAddr().Network() == "udp" {
			udpCalls.Add(1)
			response := new(dns.Msg)
			response.SetReply(request)
			response.Truncated = true
			_ = w.WriteMsg(response)
			return
		}
		tcpCalls.Add(1)
		dnsAddressAnswer(w, request)
	})
	ips, _, err := resolveDNSWithRetry("example.test", []string{server}, 1)
	if err != nil || len(ips) != 1 || udpCalls.Load() != 2 || tcpCalls.Load() != 2 {
		t.Fatalf("UDP truncation fallback failed: ips=%v err=%v udp=%d tcp=%d", ips, err, udpCalls.Load(), tcpCalls.Load())
	}
}

func TestDNSAddressFailureCannotBeMaskedByCNAME(t *testing.T) {
	server := serveTestDNS(t, func(w dns.ResponseWriter, request *dns.Msg) {
		response := new(dns.Msg)
		response.SetReply(request)
		response.Answer = []dns.RR{&dns.CNAME{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET}, Target: "missing.example.test."}}
		_ = w.WriteMsg(response)
	})
	ips, _, err := resolveDNSWithRetry("example.test", []string{server}, 1)
	if err == nil || len(ips) != 0 || !strings.Contains(err.Error(), "no A or AAAA records") {
		t.Fatalf("CNAME-only response masked missing IPs: ips=%v err=%v", ips, err)
	}
}

func TestDNSWorkingAddressFamilySurvivesOtherFamilyFailure(t *testing.T) {
	for _, family := range []uint16{dns.TypeA, dns.TypeAAAA} {
		t.Run(dns.TypeToString[family], func(t *testing.T) {
			server := serveTestDNS(t, func(w dns.ResponseWriter, request *dns.Msg) {
				response := new(dns.Msg)
				response.SetReply(request)
				if request.Question[0].Qtype != family {
					response.Rcode = dns.RcodeServerFailure
				} else if family == dns.TypeA {
					dnsAddressAnswer(w, request)
					return
				} else {
					response.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET}, AAAA: net.ParseIP("2001:db8::17")}}
				}
				_ = w.WriteMsg(response)
			})
			ips, _, err := resolveDNSWithRetry("example.test", []string{server}, 1)
			if err != nil || len(ips) != 1 {
				t.Fatalf("working family discarded: ips=%v err=%v", ips, err)
			}
		})
	}
}

func TestDNSTimeoutRemainsAnError(t *testing.T) {
	server := serveTestDNS(t, func(dns.ResponseWriter, *dns.Msg) {})
	_, _, err := resolveDNSAtServer("example.test", server, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "example.test") || !strings.Contains(err.Error(), server) {
		t.Fatalf("timeout lost its lookup context: %v", err)
	}
}
