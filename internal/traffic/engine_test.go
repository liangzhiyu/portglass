package traffic

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

type fixture struct {
	t       *testing.T
	e       *Engine
	entries map[int64]Exchange
	seq     [2]uint32
	at      time.Time
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, entries: map[int64]Exchange{}, seq: [2]uint32{101, 501}, at: time.Unix(1800000000, 0)}
	f.e = NewEngine([]uint16{7010}, func(x Exchange) { f.entries[x.ID] = x })
	f.packet(0, 100, nil, true, false)
	f.packet(1, 500, nil, true, false)
	return f
}

func (f *fixture) packet(dir int, seq uint32, data []byte, syn, fin bool) {
	f.t.Helper()
	src, dst := net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.1")
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
	sport, dport := layers.TCPPort(43000), layers.TCPPort(7010)
	if dir == 1 {
		sport, dport = dport, sport
	}
	tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq, SYN: syn, ACK: dir == 1 || !syn, FIN: fin, Window: 65535}
	_ = tcp.SetNetworkLayerForChecksum(ip)
	b := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, &layers.Loopback{Family: layers.ProtocolFamilyIPv4}, ip, tcp, gopacket.Payload(data))
	if err != nil {
		f.t.Fatal(err)
	}
	f.at = f.at.Add(10 * time.Millisecond)
	f.e.Packet(0, b.Bytes(), f.at)
}
func (f *fixture) send(dir int, s string) {
	f.packet(dir, f.seq[dir], []byte(s), false, false)
	f.seq[dir] += uint32(len(s))
}
func (f *fixture) close() {
	for i := 0; i < 2; i++ {
		f.packet(i, f.seq[i], nil, false, true)
	}
	f.e.Close()
}

func TestHTTPReassemblyRetransmissionAndPipelining(t *testing.T) {
	f := newFixture(t)
	req := "POST /echo HTTP/1.1\r\nHost: localhost:7010\r\nContent-Length: 7\r\n\r\n{\"x\":1}"
	// A later segment arrives first, then an overlapping retransmission.
	f.packet(0, 121, []byte(req[20:]), false, false)
	f.packet(0, 101, []byte(req[:25]), false, false)
	f.packet(0, 101, []byte(req[:20]), false, false)
	f.seq[0] += uint32(len(req))
	f.send(0, "GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")
	f.send(1, "HTTP/1.1 200 OK\r\nContent-Length: 7\r\nContent-Type: application/json\r\n\r\n{\"y\":2}HTTP/1.1 204 No Content\r\n\r\n")
	f.close()
	if len(f.entries) != 2 {
		t.Fatalf("want 2 exchanges, got %#v", f.entries)
	}
	x := f.entries[1]
	if x.Method != "POST" || x.Path != "/echo" || x.Status != 200 || string(x.Request.Body) != "{\"x\":1}" || string(x.Response.Body) != "{\"y\":2}" {
		t.Fatalf("wrong paired exchange: %#v", x)
	}
	if f.entries[2].Status != 204 || f.entries[2].Path != "/health" {
		t.Fatal(f.entries[2])
	}
}

func TestChunkedGzipAndInformationalResponse(t *testing.T) {
	f := newFixture(t)
	f.send(0, "GET /gzip HTTP/1.1\r\nHost: localhost\r\n\r\n")
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	_, _ = zw.Write([]byte("{\"message\":\"你好\"}"))
	_ = zw.Close()
	f.send(1, "HTTP/1.1 103 Early Hints\r\nLink: </a>\r\n\r\nHTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Encoding: gzip\r\n\r\n")
	body := fmt.Sprintf("%x\r\n", z.Len()) + z.String() + "\r\n0\r\n\r\n"
	for _, piece := range []string{body[:8], body[8 : len(body)-3], body[len(body)-3:]} {
		f.send(1, piece)
	}
	f.close()
	x := f.entries[1]
	if len(f.entries) != 1 || x.Status != 200 || string(x.Response.Body) != "{\"message\":\"你好\"}" {
		t.Fatalf("bad chunked gzip: %#v", f.entries)
	}
}

func TestHEADAndCloseDelimitedResponse(t *testing.T) {
	f := newFixture(t)
	f.send(0, "HEAD /a HTTP/1.1\r\nHost: local\r\n\r\nGET /b HTTP/1.1\r\nHost: local\r\n\r\n")
	f.send(1, "HTTP/1.1 200 OK\r\nContent-Length: 900\r\n\r\nHTTP/1.0 200 OK\r\nConnection: close\r\n\r\nhello")
	if f.entries[2].Complete {
		t.Fatal("close-delimited response completed before EOF")
	}
	f.close()
	if len(f.entries[1].Response.Body) != 0 || string(f.entries[2].Response.Body) != "hello" {
		t.Fatal(f.entries)
	}
}

func TestResponsePreviewBeforeStreamCompletes(t *testing.T) {
	f := newFixture(t)
	f.send(0, "GET /stream HTTP/1.1\r\nHost: local\r\n\r\n")
	f.send(1, "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nhello")
	if f.entries[1].Complete || string(f.entries[1].Response.Body) != "hello" {
		t.Fatal("in-progress response is not visible", f.entries)
	}
	f.send(1, "world")
	if !f.entries[1].Complete || string(f.entries[1].Response.Body) != "helloworld" {
		t.Fatal("response did not finish", f.entries)
	}
	f.close()
}

func TestTLSAndRawBidirectionalStreams(t *testing.T) {
	for _, tc := range []struct{ name, request, response, proto string }{{"TLS", "\x16\x03\x03\x00\x01x", "\x17\x03\x03encrypted", "TLS"}, {"TCP", "PING\r\n", "PONG\r\n", "TCP"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.send(0, tc.request)
			f.send(1, tc.response)
			f.close()
			if len(f.entries) != 1 || f.entries[1].Protocol != tc.proto || string(f.entries[1].Response.Raw) != tc.response {
				t.Fatal(f.entries)
			}
		})
	}
}

func TestPortsAndPayloadBounds(t *testing.T) {
	for _, bad := range []string{"", "0", "65536", "80;rm -rf /", "-1"} {
		if _, err := ParsePorts(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	p, err := ParsePorts("8080,7010，8080")
	if err != nil || Filter(p) != "tcp and (port 7010 or port 8080)" {
		t.Fatal(p, err)
	}
	f := newFixture(t)
	for i := 0; i < 6; i++ {
		f.send(0, strings.Repeat("X", 20000))
	}
	f.close()
	x := f.entries[1]
	if len(x.Request.Raw) != MaxBody || !x.Request.Truncated || x.Bytes != 120000 {
		t.Fatal(x.Bytes, len(x.Request.Raw), x.Request.Truncated)
	}
}
