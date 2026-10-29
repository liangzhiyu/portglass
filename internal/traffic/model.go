package traffic

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxBody         = 64 << 10
	MaxMessage      = 2 << 20
	MaxEntries      = 1500
	MaxHistoryBytes = 32 << 20
)

type Message struct {
	StartLine string `json:"startLine,omitempty"`
	Headers   string `json:"headers,omitempty"`
	Body      []byte `json:"body,omitempty"`
	Raw       []byte `json:"raw,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type Exchange struct {
	ID         int64     `json:"id"`
	Time       time.Time `json:"time"`
	Client     string    `json:"client"`
	Server     string    `json:"server"`
	Protocol   string    `json:"protocol"`
	Method     string    `json:"method,omitempty"`
	Path       string    `json:"path,omitempty"`
	Status     int       `json:"status,omitempty"`
	DurationMS float64   `json:"durationMs,omitempty"`
	Request    Message   `json:"request"`
	Response   Message   `json:"response"`
	Bytes      int       `json:"bytes"`
	Note       string    `json:"note,omitempty"`
	Complete   bool      `json:"complete"`
}

func ParsePorts(value string) ([]uint16, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == ';' || r == '\n' })
	if len(parts) == 0 {
		return nil, fmt.Errorf("请输入至少一个端口，例如 7010,8080")
	}
	if len(parts) > 32 {
		return nil, fmt.Errorf("最多监听 32 个端口")
	}
	seen := map[uint16]bool{}
	ports := make([]uint16, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("端口 %q 无效，范围为 1–65535", p)
		}
		if !seen[uint16(n)] {
			ports = append(ports, uint16(n))
			seen[uint16(n)] = true
		}
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	return ports, nil
}

func Filter(ports []uint16) string {
	p := make([]string, len(ports))
	for i, port := range ports {
		p[i] = fmt.Sprintf("port %d", port)
	}
	return "tcp and (" + strings.Join(p, " or ") + ")"
}

func (m Message) Text(mode string) string {
	switch mode {
	case "Headers":
		return m.StartLine + "\n" + m.Headers
	case "Raw":
		return strings.ToValidUTF8(string(m.Raw), "�")
	case "Hex":
		return hex.Dump(m.Raw)
	default:
		var buf bytes.Buffer
		if json.Indent(&buf, m.Body, "", "  ") == nil {
			return buf.String()
		}
		if !utf8.Valid(m.Body) {
			return hex.Dump(m.Body)
		}
		return string(m.Body)
	}
}

func limited(data []byte) []byte {
	if len(data) > MaxBody {
		data = data[:MaxBody]
	}
	return bytes.Clone(data)
}

func message(start string, header http.Header, body, raw []byte) Message {
	m := Message{StartLine: start, Body: limited(body), Raw: limited(raw), Truncated: len(body) > MaxBody || len(raw) > MaxBody}
	var b strings.Builder
	_ = header.Write(&b)
	m.Headers = b.String()
	if strings.EqualFold(header.Get("Content-Encoding"), "gzip") {
		if zr, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			decoded, err := io.ReadAll(io.LimitReader(zr, MaxBody+1))
			_ = zr.Close()
			if err == nil {
				m.Body = limited(decoded)
				m.Truncated = m.Truncated || len(decoded) > MaxBody
			}
		}
	}
	return m
}

func (e Exchange) Matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.Join([]string{e.Method, e.Path, e.Client, e.Server, e.Protocol, strconv.Itoa(e.Status), e.Request.Headers, e.Response.Headers, string(e.Request.Body), string(e.Response.Body)}, " ")), q)
}

func (e Exchange) Size() int {
	return len(e.Request.Raw) + len(e.Request.Body) + len(e.Response.Raw) + len(e.Response.Body) + len(e.Request.Headers) + len(e.Response.Headers)
}
