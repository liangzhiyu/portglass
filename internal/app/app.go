package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/liangzhiyu/portglass/internal/capture"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

type App struct {
	win                    *mygo.Window
	ports, iface, query    string
	interfaces             []string
	running, busy, demo    bool
	errorText, notice      string
	cancel                 context.CancelFunc
	generation             uint64
	entries                []traffic.Exchange
	index                  map[int64]int
	historyBytes, evicted  int
	packets                int64
	dropped                uint32
	selected               int64
	protocol, side, format int
	list                   ui.ListState
	code                   *editor.State
	codeText               string
	appearance             int
	reducedTransparency    bool
}

func New() *App {
	a := &App{ports: "7010", iface: "lo0", index: map[int64]int{}, interfaces: capture.Interfaces()}
	a.code = &editor.State{Buffer: editor.NewBuffer(""), ReadOnly: true, SoftWrap: true, NoScrollPastEnd: true, NoActiveLine: true, Label: "内容查看器"}
	a.code.Font = ui.Font{Family: "monospace", Size: 13}
	a.code.LineHeight = 1.65
	a.loadConfig()
	return a
}

func (a *App) Open(demo bool, screenshot string) {
	mygo.Theme.SetSource([]mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark}[a.appearance])
	if demo {
		a.loadDemo()
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "PortGlass", Width: 1380, Height: 860, MinWidth: 1100, MinHeight: 680,
		TitleBarStyle: mygo.TitleBarHidden,
		StateKey:      "main", Content: ui.View(a.view),
	})
	a.win.OnClosed(func() { a.Stop(); mygo.App.Quit() })
	if screenshot != "" {
		go func() {
			time.Sleep(2 * time.Second)
			data, err := a.win.CapturePage()
			if err == nil {
				err = os.WriteFile(screenshot, data, 0600)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			mygo.App.Quit()
		}()
	}
}

func (a *App) update(f func()) {
	if a.win != nil {
		a.win.Update(f)
	} else {
		f()
	}
}

func (a *App) Start() {
	if a.running || a.busy {
		return
	}
	ports, err := traffic.ParsePorts(a.ports)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.clear()
	a.demo = false
	a.busy = true
	a.errorText = ""
	a.notice = "正在等待系统抓包授权…"
	a.generation++
	generation := a.generation
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	o := capture.Options{Interface: a.iface, Ports: a.ports}
	a.saveConfig()
	go a.capture(ctx, o, ports, generation)
}

func (a *App) Stop() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *App) capture(ctx context.Context, o capture.Options, ports []uint16, generation uint64) {
	session, err := capture.Start(ctx, o)
	if err != nil {
		a.update(func() {
			if a.generation != generation {
				return
			}
			a.busy = false
			a.running = false
			a.notice = ""
			if !errors.Is(err, context.Canceled) {
				a.errorText = err.Error()
			}
		})
		return
	}
	defer session.Close()
	a.update(func() {
		if a.generation == generation {
			a.running = true
			a.busy = false
			a.notice = ""
		}
	})
	type received struct {
		event capture.Event
		err   error
	}
	events := make(chan received, 256)
	go func() {
		defer close(events)
		for {
			event, err := session.Read()
			select {
			case events <- received{event, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	batch := map[int64]traffic.Exchange{}
	engine := traffic.NewEngine(ports, func(x traffic.Exchange) { batch[x.ID] = x })
	var packets int64
	var dropped uint32
	flush := func() {
		list := make([]traffic.Exchange, 0, len(batch))
		for _, x := range batch {
			list = append(list, x)
		}
		batch = map[int64]traffic.Exchange{}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		p, d := packets, dropped
		a.update(func() {
			if a.generation != generation {
				return
			}
			a.packets = p
			a.dropped = d
			for _, x := range list {
				a.upsert(x)
			}
		})
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var captureErr error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case value, ok := <-events:
			if !ok {
				break loop
			}
			if value.err != nil {
				if ctx.Err() == nil && !errors.Is(value.err, io.EOF) {
					captureErr = value.err
				}
				break loop
			}
			switch value.event.Kind {
			case "packet":
				packets++
				engine.Packet(value.event.Link, value.event.Data, value.event.At)
			case "stats":
				dropped = value.event.Dropped
			case "error":
				captureErr = errors.New(value.event.Message)
				break loop
			}
		case now := <-ticker.C:
			engine.Tick(now)
			flush()
		}
	}
	engine.Close()
	flush()
	a.update(func() {
		if a.generation != generation {
			return
		}
		a.running = false
		a.busy = false
		a.notice = "监听已停止"
		if captureErr != nil {
			a.errorText = captureErr.Error()
		}
	})
}

func (a *App) clear() {
	a.entries = nil
	a.index = map[int64]int{}
	a.historyBytes = 0
	a.evicted = 0
	a.selected = 0
	a.packets = 0
	a.dropped = 0
	a.list = ui.ListState{}
	a.codeText = ""
	a.code.Buffer.Reset("")
}

func (a *App) upsert(x traffic.Exchange) {
	if idx, ok := a.index[x.ID]; ok {
		a.historyBytes -= a.entries[idx].Size()
		a.entries[idx] = x
	} else {
		a.index[x.ID] = len(a.entries)
		a.entries = append(a.entries, x)
	}
	a.historyBytes += x.Size()
	for len(a.entries) > traffic.MaxEntries || a.historyBytes > traffic.MaxHistoryBytes {
		old := a.entries[0]
		a.historyBytes -= old.Size()
		delete(a.index, old.ID)
		copy(a.entries, a.entries[1:])
		a.entries[len(a.entries)-1] = traffic.Exchange{}
		a.entries = a.entries[:len(a.entries)-1]
		a.evicted++
		for i, e := range a.entries {
			a.index[e.ID] = i
		}
	}
	if _, exists := a.index[a.selected]; !exists && len(a.entries) > 0 {
		a.selected = a.entries[len(a.entries)-1].ID
	}
}

func (a *App) filtered() []traffic.Exchange {
	items := make([]traffic.Exchange, 0, len(a.entries))
	for i := len(a.entries) - 1; i >= 0; i-- {
		x := a.entries[i]
		if !x.Matches(a.query) {
			continue
		}
		if a.protocol == 1 && x.Protocol != "HTTP/1" {
			continue
		}
		if a.protocol == 2 && x.Protocol == "HTTP/1" {
			continue
		}
		items = append(items, x)
	}
	return items
}

func (a *App) export() {
	entries := append([]traffic.Exchange(nil), a.entries...)
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "导出捕获会话", DefaultPath: "PortGlass-" + time.Now().Format("20060102-150405") + ".json"})
		if err == nil && path != "" {
			var data []byte
			data, err = json.MarshalIndent(struct {
				Version string             `json:"version"`
				Entries []traffic.Exchange `json:"entries"`
			}{"1", entries}, "", "  ")
			if err == nil {
				err = os.WriteFile(path, data, 0600)
			}
		}
		a.update(func() {
			if err != nil {
				a.errorText = err.Error()
			} else if path != "" {
				a.notice = "会话已导出"
			}
		})
	}()
}

func configPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "PortGlass", "settings.json")
}
func (a *App) loadConfig() {
	var c struct {
		Ports, Interface string
		Appearance       int
	}
	data, err := os.ReadFile(configPath())
	if err != nil || json.Unmarshal(data, &c) != nil {
		return
	}
	if _, err = traffic.ParsePorts(c.Ports); err == nil {
		a.ports = c.Ports
	}
	if c.Appearance >= 0 && c.Appearance <= 2 {
		a.appearance = c.Appearance
	}
	for _, name := range a.interfaces {
		if name == c.Interface {
			a.iface = name
		}
	}
}
func (a *App) saveConfig() {
	p := configPath()
	if p == "" {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0700) != nil {
		return
	}
	b, _ := json.Marshal(struct {
		Ports, Interface string
		Appearance       int
	}{a.ports, a.iface, a.appearance})
	_ = os.WriteFile(p, b, 0600)
}

func bytesLabel(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
