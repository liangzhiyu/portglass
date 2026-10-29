package traffic

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/tcpassembly"
)

// Engine is owned by one goroutine. Both TCP directions share a conversation;
// tcpassembly orders segments and removes retransmissions before HTTP framing.
type Engine struct {
	ports         map[uint16]bool
	conversations map[string]*conversation
	assembler     *tcpassembly.Assembler
	emit          func(Exchange)
	next          int64
	lastFlush     time.Time
}

type conversation struct {
	key, client, server string
	pending             []*Exchange
	stream              *Exchange
	request, response   *direction
	last                time.Time
	closed              int
}

type direction struct {
	preview     *Exchange
	requestSize int
	previewNote string
	e           *Engine
	c           *conversation
	request     bool
	buf         []byte
	mode        string
	seen        time.Time
	closed      bool
	gap         bool
}

func NewEngine(ports []uint16, emit func(Exchange)) *Engine {
	e := &Engine{ports: map[uint16]bool{}, conversations: map[string]*conversation{}, emit: emit}
	for _, p := range ports {
		e.ports[p] = true
	}
	e.assembler = tcpassembly.NewAssembler(tcpassembly.NewStreamPool(e))
	e.assembler.MaxBufferedPagesPerConnection = 128
	e.assembler.MaxBufferedPagesTotal = 2048
	return e
}

func endpoints(n, t gopacket.Flow) (string, string) {
	return net.JoinHostPort(n.Src().String(), t.Src().String()), net.JoinHostPort(n.Dst().String(), t.Dst().String())
}

func flowKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

func (e *Engine) New(n, t gopacket.Flow) tcpassembly.Stream {
	src, dst := endpoints(n, t)
	key := flowKey(src, dst)
	c := e.conversations[key]
	if c == nil {
		port, _ := strconv.Atoi(t.Dst().String())
		client, server := src, dst
		if !e.ports[uint16(port)] {
			client, server = dst, src
		}
		c = &conversation{key: key, client: client, server: server}
		e.conversations[key] = c
	}
	d := &direction{e: e, c: c, request: src == c.client}
	if d.request {
		c.request = d
	} else {
		c.response = d
	}
	return d
}

func (e *Engine) Packet(link int, data []byte, at time.Time) {
	p := gopacket.NewPacket(data, layers.LinkType(link), gopacket.DecodeOptions{Lazy: false, NoCopy: true})
	if p.NetworkLayer() == nil {
		return
	}
	tcp, ok := p.TransportLayer().(*layers.TCP)
	if !ok {
		return
	}
	if !e.ports[uint16(tcp.SrcPort)] && !e.ports[uint16(tcp.DstPort)] {
		return
	}
	src, dst := endpoints(p.NetworkLayer().NetworkFlow(), tcp.TransportFlow())
	key := flowKey(src, dst)
	if tcp.SYN && !tcp.ACK {
		if old := e.conversations[key]; old == nil {
			e.conversations[key] = &conversation{key: key, client: src, server: dst, last: at}
		}
	}
	// A bounded number of connections keeps a busy port from exhausting memory.
	if len(e.conversations) > 256 {
		e.assembler.FlushAll()
		e.conversations = map[string]*conversation{}
	}
	e.assembler.AssembleWithTimestamp(p.NetworkLayer().NetworkFlow(), tcp, at)
	if at.Sub(e.lastFlush) > time.Second {
		e.Tick(at)
	}
}

func (e *Engine) Tick(at time.Time) {
	e.assembler.FlushWithOptions(tcpassembly.FlushOptions{T: at.Add(-2 * time.Second)})
	e.assembler.FlushOlderThan(at.Add(-60 * time.Second))
	e.lastFlush = at
	for key, c := range e.conversations {
		if at.Sub(c.last) > 90*time.Second {
			delete(e.conversations, key)
		}
	}
}

func (e *Engine) Close() { e.assembler.FlushAll() }

func (e *Engine) entry(c *conversation, at time.Time, proto string) *Exchange {
	e.next++
	return &Exchange{ID: e.next, Time: at, Client: c.client, Server: c.server, Protocol: proto}
}

func (e *Engine) publish(x *Exchange) { e.emit(*x) }

func (d *direction) Reassembled(parts []tcpassembly.Reassembly) {
	for _, part := range parts {
		d.seen = part.Seen
		d.c.last = part.Seen
		if part.Skip != 0 {
			d.gap = true
			if part.Skip > 0 {
				d.mode = "TCP"
				d.buf = nil
				d.c.pending = nil
			}
		}
		if len(part.Bytes) == 0 {
			continue
		}
		if d.mode == "TCP" || d.mode == "TLS" || d.mode == "HTTP/2" {
			d.raw(part.Bytes)
			continue
		}
		if len(d.buf)+len(part.Bytes) > MaxMessage {
			d.mode = "TCP"
			d.raw(d.buf)
			d.buf = nil
			d.raw(part.Bytes)
			continue
		}
		d.buf = append(d.buf, part.Bytes...)
		d.parse(false)
	}
}

func (d *direction) ReassemblyComplete() {
	d.closed = true
	d.parse(true)
	if len(d.buf) > 0 {
		d.mode = "TCP"
		d.raw(d.buf)
		d.buf = nil
	}
	d.c.closed++
	if d.c.closed >= 2 {
		for _, x := range d.c.pending {
			x.Note = "连接结束，响应未完整捕获"
			d.e.publish(x)
		}
		if d.c.stream != nil {
			d.c.stream.Complete = true
			d.e.publish(d.c.stream)
		}
		delete(d.e.conversations, d.c.key)
	}
}

func isHTTPPrefix(b []byte, request bool) bool {
	if !request {
		return bytes.HasPrefix(b, []byte("HTTP/")) || bytes.HasPrefix([]byte("HTTP/"), b)
	}
	for _, m := range []string{"GET ", "POST ", "PUT ", "PATCH ", "DELETE ", "HEAD ", "OPTIONS ", "CONNECT ", "TRACE ", "PRI "} {
		if bytes.HasPrefix(b, []byte(m)) || bytes.HasPrefix([]byte(m), b) {
			return true
		}
	}
	return false
}

func (d *direction) parse(eof bool) {
	for len(d.buf) > 0 {
		if len(d.buf) < 5 && !eof && d.buf[0] >= 20 && d.buf[0] <= 23 {
			return
		}
		if len(d.buf) >= 3 && d.buf[0] >= 20 && d.buf[0] <= 23 && d.buf[1] == 3 {
			d.mode = "TLS"
			d.raw(d.buf)
			d.buf = nil
			return
		}
		if bytes.HasPrefix(d.buf, []byte("PRI * HTTP/2.0")) {
			d.mode = "HTTP/2"
			d.raw(d.buf)
			d.buf = nil
			return
		}
		if !isHTTPPrefix(d.buf, d.request) {
			d.mode = "TCP"
			d.raw(d.buf)
			d.buf = nil
			return
		}
		if bytes.Index(d.buf, []byte("\r\n\r\n")) < 0 {
			return
		}
		reader := bytes.NewReader(d.buf)
		br := bufio.NewReader(reader)
		if d.request {
			r, err := http.ReadRequest(br)
			if err != nil {
				d.fallback()
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, MaxMessage+1))
			_ = r.Body.Close()
			if err != nil {
				if eof {
					d.fallback()
				}
				return
			}
			if len(body) > MaxMessage {
				d.fallback()
				return
			}
			n := len(d.buf) - reader.Len() - br.Buffered()
			x := d.e.entry(d.c, d.seen, "HTTP/1")
			x.Method = r.Method
			x.Path = r.URL.RequestURI()
			header := r.Header.Clone()
			if r.Host != "" {
				header.Set("Host", r.Host)
			}
			x.Request = message(r.Method+" "+x.Path+" "+r.Proto, header, body, d.buf[:n])
			x.Bytes = n
			if d.gap {
				x.Note = "从连接中途开始捕获"
			}
			d.c.pending = append(d.c.pending, x)
			d.e.publish(x)
			if len(d.c.pending) > 128 {
				d.c.pending = d.c.pending[1:]
			}
			d.buf = bytes.Clone(d.buf[n:])
			if d.c.response != nil && len(d.c.response.buf) > 0 {
				d.c.response.parse(false)
			}
		} else {
			var req *http.Request
			if len(d.c.pending) > 0 {
				req = &http.Request{Method: d.c.pending[0].Method}
			}
			r, err := http.ReadResponse(br, req)
			if err != nil {
				d.fallback()
				return
			}
			upgrade := r.StatusCode == 101 || (req != nil && req.Method == "CONNECT" && r.StatusCode >= 200 && r.StatusCode < 300)
			headerLength := len(d.buf) - reader.Len() - br.Buffered()
			var body []byte
			if !upgrade {
				body, err = io.ReadAll(io.LimitReader(r.Body, MaxMessage+1))
				_ = r.Body.Close()
			}
			if len(body) > MaxMessage {
				d.fallback()
				return
			}
			n := len(d.buf) - reader.Len() - br.Buffered()
			if upgrade {
				n = headerLength
			}
			if n <= 0 {
				return
			}
			if r.StatusCode >= 100 && r.StatusCode < 200 && r.StatusCode != 101 {
				d.buf = bytes.Clone(d.buf[n:])
				continue
			}
			complete := upgrade || (err == nil && (r.ContentLength >= 0 || len(r.TransferEncoding) > 0 || eof))
			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				d.fallback()
				return
			}
			if d.preview == nil {
				if len(d.c.pending) > 0 {
					d.preview = d.c.pending[0]
				} else {
					d.preview = d.e.entry(d.c, d.seen, "HTTP/1")
					d.preview.Note = "未捕获对应请求"
				}
				d.requestSize = d.preview.Bytes
				d.previewNote = d.preview.Note
			}
			x := d.preview
			x.Response = message(r.Proto+" "+r.Status, r.Header, body, d.buf[:n])
			x.Status = r.StatusCode
			x.Bytes = d.requestSize + n
			x.Complete = complete
			x.Note = d.previewNote
			if !complete {
				x.Note = strings.TrimSpace(d.previewNote + " 响应接收中")
			}
			x.DurationMS = max(0, float64(d.seen.Sub(x.Time).Microseconds())/1000)
			d.e.publish(x)
			if !complete {
				if eof {
					x.Note = "连接已结束，响应内容不完整"
					d.e.publish(x)
					d.buf = nil
				}
				return
			}
			if len(d.c.pending) > 0 && d.c.pending[0] == x {
				d.c.pending = d.c.pending[1:]
			}
			d.preview = nil

			d.buf = bytes.Clone(d.buf[n:])
			if r.StatusCode == 101 || (req != nil && req.Method == "CONNECT" && r.StatusCode >= 200 && r.StatusCode < 300) {
				d.mode = "TCP"
				if d.c.request != nil {
					d.c.request.mode = "TCP"
				}
				if len(d.buf) > 0 {
					d.raw(d.buf)
					d.buf = nil
				}
				return
			}
		}
	}
}

func (d *direction) fallback() { d.mode = "TCP"; d.raw(d.buf); d.buf = nil }

func (d *direction) raw(data []byte) {
	x := d.c.stream
	if x == nil {
		x = d.e.entry(d.c, d.seen, d.mode)
		d.c.stream = x
		x.Path = d.c.server
	}
	if d.mode == "TLS" {
		x.Protocol = "TLS"
		x.Note = "HTTPS / TLS 内容已加密；此版本不解密"
	}
	if d.mode == "HTTP/2" {
		x.Protocol = "HTTP/2"
		x.Note = "HTTP/2 以原始字节显示"
	}
	if d.gap && x.Note == "" {
		x.Note = "捕获不完整或 TCP 存在缺失；以原始字节显示"
	}
	x.Bytes += len(data)
	m := &x.Response
	if d.request {
		m = &x.Request
	}
	available := MaxBody - len(m.Raw)
	if available < len(data) {
		m.Truncated = true
		data = data[:max(available, 0)]
	}
	m.Raw = append(bytes.Clone(m.Raw), data...)
	m.Body = bytes.Clone(m.Raw)
	if x.Protocol == "TCP" && x.Note == "" {
		x.Note = "TCP 字节流 · 输入与输出按所选服务端口区分"
	}
	d.e.publish(x)
}

func Description(x Exchange) string {
	if x.Method != "" {
		return x.Method + " " + x.Path
	}
	return strings.TrimSpace(fmt.Sprintf("%s · %s", x.Protocol, x.Server))
}
