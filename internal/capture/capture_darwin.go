//go:build darwin

// Package capture isolates packet capture from the desktop UI. It uses the
// system libpcap through purego: no Homebrew, ngrep, C compiler or driver.
package capture

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

type Event struct {
	Kind    string    `json:"kind"`
	Nonce   string    `json:"nonce,omitempty"`
	Link    int       `json:"link,omitempty"`
	At      time.Time `json:"at,omitempty"`
	Data    []byte    `json:"data,omitempty"`
	Message string    `json:"message,omitempty"`
	Dropped uint32    `json:"dropped,omitempty"`
}

type Options struct {
	Interface string
	Ports     string
}

type Session struct {
	conn    net.Conn
	decoder *json.Decoder
	cmd     *exec.Cmd
	dir     string
	once    sync.Once
	stop    chan struct{}
}

func Interfaces() []string {
	all, _ := net.Interfaces()
	result := []string{}
	for _, i := range all {
		if i.Flags&net.FlagUp != 0 {
			result = append(result, i.Name)
		}
	}
	return result
}

func validate(o Options) ([]uint16, error) {
	ports, err := traffic.ParsePorts(o.Ports)
	if err != nil {
		return nil, err
	}
	if _, err := net.InterfaceByName(o.Interface); err != nil {
		return nil, fmt.Errorf("网卡 %q 不存在", o.Interface)
	}
	return ports, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// Start requests authorization through macOS's own dialog when BPF devices are
// inaccessible. The UI never sees or stores the administrator's password.
func Start(ctx context.Context, o Options) (*Session, error) {
	if _, err := validate(o); err != nil {
		return nil, err
	}
	// Unix-domain paths must fit macOS's 104-byte sockaddr_un. /tmp is used only
	// for a private 0700 directory; packet data is never written there.
	dir, err := os.MkdirTemp("/tmp", "portglass-")
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(dir, "capture.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	defer l.Close()
	_ = os.Chmod(sock, 0600)
	nonceBytes := make([]byte, 32)
	if _, err = rand.Read(nonceBytes); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	exe, err := os.Executable()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	args := []string{"--capture-helper", sock, nonce, o.Interface, o.Ports, strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())}
	var cmd *exec.Cmd
	if syscall.Access("/dev/bpf0", 4) == nil || os.Geteuid() == 0 {
		cmd = exec.Command(exe, args...)
	} else {
		parts := []string{shellQuote(exe)}
		for _, a := range args {
			parts = append(parts, shellQuote(a))
		}
		script := "do shell script " + strconv.Quote(strings.Join(parts, " ")) + " with administrator privileges"
		cmd = exec.Command("/usr/bin/osascript", "-e", script)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	type acceptResult struct {
		c   net.Conn
		err error
	}
	accepted := make(chan acceptResult, 1)
	go func() { c, e := l.Accept(); accepted <- acceptResult{c, e} }()
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	var conn net.Conn
	acceptedRead := false
	select {
	case a := <-accepted:
		conn, err = a.c, a.err
		acceptedRead = true
	case e := <-done:
		err = fmt.Errorf("抓包授权未完成：%v %s", e, strings.TrimSpace(stderr.String()))
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = errors.New("等待系统授权超时，请重新开始")
	}
	if err != nil {
		_ = l.Close()
		_ = cmd.Process.Kill()
		os.RemoveAll(dir)
		// If Accept won the race with cancellation, close that socket too.
		if conn != nil {
			conn.Close()
		}
		if !acceptedRead {
			go func() {
				a := <-accepted
				if a.c != nil {
					a.c.Close()
				}
			}()
		}
		return nil, err
	}
	s := &Session{conn: conn, decoder: json.NewDecoder(bufio.NewReaderSize(conn, 128<<10)), cmd: cmd, dir: dir, stop: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-s.stop:
		}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello Event
	if err = s.decoder.Decode(&hello); err != nil || hello.Kind != "hello" || hello.Nonce != nonce {
		s.Close()
		return nil, fmt.Errorf("抓包进程握手失败：%v", err)
	}
	var ready Event
	if err = s.decoder.Decode(&ready); err != nil {
		s.Close()
		return nil, err
	}
	if ready.Kind != "ready" {
		s.Close()
		return nil, errors.New(ready.Message)
	}
	_ = conn.SetReadDeadline(time.Time{})
	return s, nil
}

func (s *Session) Read() (Event, error) {
	var event Event
	err := s.decoder.Decode(&event)
	return event, err
}

func (s *Session) Close() {
	s.once.Do(func() { close(s.stop); s.conn.Close(); _ = s.cmd.Process.Kill(); os.RemoveAll(s.dir) })
}

type bpfProgram struct {
	Length       uint32
	_            uint32
	Instructions uintptr
}
type packetHeader struct {
	Seconds  int64
	Micros   int32
	_        int32
	Captured uint32
	Length   uint32
}

type pcapAPI struct {
	create     func(string, *byte) uintptr
	snaplen    func(uintptr, int32) int32
	timeout    func(uintptr, int32) int32
	immediate  func(uintptr, int32) int32
	bufferSize func(uintptr, int32) int32
	activate   func(uintptr) int32
	compile    func(uintptr, *bpfProgram, string, int32, uint32) int32
	setFilter  func(uintptr, *bpfProgram) int32
	freeCode   func(*bpfProgram)
	nonblock   func(uintptr, int32, *byte) int32
	next       func(uintptr, *unsafe.Pointer, *unsafe.Pointer) int32
	link       func(uintptr) int32
	geterr     func(uintptr) string
	stats      func(uintptr, *uint32) int32
	close      func(uintptr)
}

func loadPCAP() (*pcapAPI, error) {
	h, err := purego.Dlopen("/usr/lib/libpcap.A.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	p := &pcapAPI{}
	for name, target := range map[string]any{
		"pcap_create": &p.create, "pcap_set_snaplen": &p.snaplen, "pcap_set_timeout": &p.timeout,
		"pcap_set_immediate_mode": &p.immediate, "pcap_set_buffer_size": &p.bufferSize,
		"pcap_activate": &p.activate, "pcap_compile": &p.compile, "pcap_setfilter": &p.setFilter,
		"pcap_freecode": &p.freeCode, "pcap_setnonblock": &p.nonblock, "pcap_next_ex": &p.next,
		"pcap_datalink": &p.link, "pcap_geterr": &p.geterr, "pcap_stats": &p.stats, "pcap_close": &p.close,
	} {
		purego.RegisterLibFunc(target, h, name)
	}
	return p, nil
}

// RunHelper opens the capture descriptor, then irrevocably drops root before
// forwarding any packets. EOF from the UI socket ends the helper immediately.
func RunHelper(args []string) error {
	if len(args) != 6 {
		return errors.New("invalid capture-helper arguments")
	}
	sock, nonce := args[0], args[1]
	uid, err := strconv.Atoi(args[4])
	if err != nil || uid < 0 {
		return errors.New("invalid uid")
	}
	gid, err := strconv.Atoi(args[5])
	if err != nil || gid < 0 {
		return errors.New("invalid gid")
	}
	if len(nonce) != 64 {
		return errors.New("invalid nonce")
	}
	info, err := os.Lstat(filepath.Dir(sock))
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || stat.Uid != uint32(uid) {
		return errors.New("unsafe capture socket directory")
	}
	o := Options{Interface: args[2], Ports: args[3]}
	ports, err := validate(o)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	encoder := json.NewEncoder(conn)
	if err = encoder.Encode(Event{Kind: "hello", Nonce: nonce}); err != nil {
		return err
	}
	fail := func(err error) error { _ = encoder.Encode(Event{Kind: "error", Message: err.Error()}); return err }
	p, err := loadPCAP()
	if err != nil {
		return fail(err)
	}
	var errbuf [256]byte
	h := p.create(o.Interface, &errbuf[0])
	if h == 0 {
		return fail(fmt.Errorf("pcap_create: %s", strings.TrimRight(string(errbuf[:]), "\x00")))
	}
	defer p.close(h)
	for _, code := range []int32{p.snaplen(h, 65535), p.timeout(h, 100), p.immediate(h, 1), p.bufferSize(h, 4<<20)} {
		if code < 0 {
			return fail(errors.New(p.geterr(h)))
		}
	}
	if p.activate(h) < 0 {
		return fail(fmt.Errorf("无法打开网卡：%s", p.geterr(h)))
	}
	var program bpfProgram
	if p.compile(h, &program, traffic.Filter(ports), 1, 0xffffffff) < 0 {
		return fail(errors.New(p.geterr(h)))
	}
	filterResult := p.setFilter(h, &program)
	p.freeCode(&program)
	if filterResult < 0 {
		return fail(errors.New(p.geterr(h)))
	}
	if p.nonblock(h, 1, &errbuf[0]) < 0 {
		return fail(errors.New(p.geterr(h)))
	}
	if os.Geteuid() == 0 && uid != 0 {
		if err = syscall.Setgroups([]int{gid}); err != nil {
			return fail(err)
		}
		if err = syscall.Setgid(gid); err != nil {
			return fail(err)
		}
		if err = syscall.Setuid(uid); err != nil {
			return fail(err)
		}
		if os.Geteuid() != uid {
			return fail(errors.New("failed to drop capture privileges"))
		}
	}
	link := int(p.link(h))
	if err = encoder.Encode(Event{Kind: "ready", Link: link}); err != nil {
		return err
	}
	stopped := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, conn); close(stopped) }()
	lastStats := time.Now()
	for {
		select {
		case <-stopped:
			return nil
		default:
		}
		var headerPtr, dataPtr unsafe.Pointer
		rc := p.next(h, &headerPtr, &dataPtr)
		switch rc {
		case 1:
			header := *(*packetHeader)(headerPtr)
			if header.Captured > 65535 {
				return fail(errors.New("invalid packet size"))
			}
			data := append([]byte(nil), unsafe.Slice((*byte)(dataPtr), int(header.Captured))...)
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err = encoder.Encode(Event{Kind: "packet", Link: link, At: time.Unix(header.Seconds, int64(header.Micros)*1000), Data: data}); err != nil {
				return err
			}
		case 0:
			time.Sleep(5 * time.Millisecond)
		case -2:
			return nil
		default:
			return fail(errors.New(p.geterr(h)))
		}
		if time.Since(lastStats) > time.Second {
			var stats [16]uint32
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if p.stats(h, &stats[0]) == 0 {
				if err = encoder.Encode(Event{Kind: "stats", Dropped: stats[1]}); err != nil {
					return err
				}
			}
			lastStats = time.Now()
		}
	}
}
