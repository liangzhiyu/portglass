package app

import (
	"fmt"
	"github.com/liangzhiyu/portglass/internal/traffic"
	"time"
)

// Demonstration data is opt-in and always visibly labeled. It is never mixed
// with captured traffic: starting a capture clears the demonstration session.
func (a *App) loadDemo() {
	if a.running || a.busy {
		return
	}
	a.clear()
	a.demo = true
	a.notice = "示例会话 · 以下内容为演示数据"
	a.side = 1
	paths := []string{"/api/health", "/api/projects", "/api/projects/portglass", "/api/events", "/api/settings", "/api/metrics", "/api/projects/portglass"}
	for i, path := range paths {
		body := []byte("{\"status\":\"ok\"}")
		if i == 6 {
			body = []byte("{\n  \"id\": \"portglass\",\n  \"name\": \"PortGlass\",\n  \"status\": \"listening\",\n  \"interface\": \"lo0\",\n  \"ports\": [7010, 8080],\n  \"capture\": {\n    \"protocol\": \"HTTP/1.1\",\n    \"requests\": 128,\n    \"dropped\": 0\n  },\n  \"message\": \"让每一次请求，都清晰可见。\"\n}")
		}
		x := traffic.Exchange{ID: int64(i + 1), Time: time.Date(2026, 10, 29, 10, 42, 12+i, 0, time.Local), Client: "127.0.0.1:53412", Server: "127.0.0.1:7010", Protocol: "HTTP/1", Method: "GET", Path: path, Status: 200, DurationMS: float64(12 + i*3), Complete: true, Bytes: 242 + len(body)}
		if i == 3 {
			x.Method = "POST"
			x.Status = 201
		}
		x.Request = traffic.Message{StartLine: x.Method + " " + path + " HTTP/1.1", Headers: "Host: localhost:7010\r\nAccept: application/json\r\nUser-Agent: PortGlass-Demo/1.0\r\n", Raw: []byte(x.Method + " " + path + " HTTP/1.1\r\nHost: localhost:7010\r\n\r\n")}
		x.Response = traffic.Message{StartLine: "HTTP/1.1 200 OK", Headers: fmt.Sprintf("Content-Type: application/json; charset=utf-8\r\nContent-Length: %d\r\nX-Request-Id: pg_demo_00%d\r\n", len(body), i+1), Body: body, Raw: append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"), body...)}
		a.upsert(x)
	}
	a.selected = 7
}
