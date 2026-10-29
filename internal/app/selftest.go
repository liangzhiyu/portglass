package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/liangzhiyu/portglass/internal/capture"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

// LiveSelfTest captures only two temporary loopback server ports. Its synthetic
// traffic exercises the actual permission dialog, helper, BPF and HTTP parser.
func LiveSelfTest() error {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-PortGlass-Test", "loopback")
		fmt.Fprintf(w, "{\"received\":%q,\"ok\":true}", string(body))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	server2 := httptest.NewUnstartedServer(handler)
	ipv6, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return fmt.Errorf("IPv6 loopback: %w", err)
	}
	server2.Listener = ipv6
	server2.Start()
	defer server2.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	_, port2, _ := net.SplitHostPort(server2.Listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fmt.Println("Requesting loopback capture authorization for temporary test ports…")
	s, err := capture.Start(ctx, capture.Options{Interface: "lo0", Ports: port + "," + port2})
	if err != nil {
		return err
	}
	defer s.Close()
	fmt.Println("Capture ready; sending HTTP test requests.")
	ports, _ := traffic.ParsePorts(port + "," + port2)
	complete := map[int64]traffic.Exchange{}
	engine := traffic.NewEngine(ports, func(x traffic.Exchange) {
		if x.Complete && x.Status == 200 {
			complete[x.ID] = x
		}
	})
	defer engine.Close()
	type read struct {
		event capture.Event
		err   error
	}
	packets := make(chan read, 64)
	go func() {
		for {
			ev, err := s.Read()
			select {
			case packets <- read{ev, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	requestErrors := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true, Proxy: nil}}
		for _, url := range []string{server.URL, server2.URL} {
			response, err := client.Post(url+"/portglass-live-test", "application/json", strings.NewReader("hello-portglass"))
			if err != nil {
				requestErrors <- err
				return
			}
			_, err = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if err != nil {
				requestErrors <- err
				return
			}
		}
		requestErrors <- nil
	}()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case result := <-packets:
			if result.err != nil {
				return result.err
			}
			if result.event.Kind == "packet" {
				engine.Packet(result.event.Link, result.event.Data, result.event.At)
			}
		case err := <-requestErrors:
			if err != nil {
				return err
			}
		case now := <-tick.C:
			engine.Tick(now)
		case <-timeout.C:
			return fmt.Errorf("live capture timed out: %d / 2 exchanges paired", len(complete))
		case <-ctx.Done():
			return ctx.Err()
		}
		if len(complete) >= 2 {
			for _, x := range complete {
				if string(x.Request.Body) != "hello-portglass" || !strings.Contains(string(x.Response.Body), "hello-portglass") {
					return fmt.Errorf("live payload mismatch: request=%q response=%q", x.Request.Body, x.Response.Body)
				}
			}
			fmt.Printf("PASS: %d real HTTP exchanges captured and paired across two loopback ports.\n", len(complete))
			return nil
		}
	}
}
