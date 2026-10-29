package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

func TestInspectorSearchAndDirection(t *testing.T) {
	a := New()
	a.loadDemo()
	tt := ui.NewTester(a.view, 1380, 860)
	if !tt.HasText("示例会话") || !tt.HasText("GET /api/projects/portglass") {
		t.Fatal("demo session or inspector not visible", tt.Texts())
	}
	if err := tt.Click("Headers"); err != nil {
		t.Fatal(err)
	}
	if a.format != 1 {
		t.Fatal("format selection did not change")
	}
	if err := tt.Click("搜索流量"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/api/health")
	if len(a.filtered()) != 1 || a.filtered()[0].Path != "/api/health" {
		t.Fatal("search failed", a.query, a.filtered())
	}
	if err := tt.Click("清空"); err != nil {
		t.Fatal(err)
	}
	if len(a.entries) != 0 || a.selected != 0 {
		t.Fatal("clear failed")
	}
}

func TestInvalidPortNeverStartsAuthorization(t *testing.T) {
	a := New()
	a.ports = "7010; touch /tmp/invalid"
	tt := ui.NewTester(a.view, 1100, 680)
	if err := tt.Click("开始监听"); err != nil {
		t.Fatal(err)
	}
	if a.busy || a.running || a.cancel != nil || a.errorText == "" {
		t.Fatal("invalid port started capture")
	}
}

func TestRedesignedControls(t *testing.T) {
	a := New()
	a.appearance = 0
	a.loadDemo()
	tt := ui.NewTester(a.view, 1100, 680)
	if err := tt.Click("GET /api/metrics"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if a.selected != 5 {
		t.Fatal("keyboard request navigation failed", a.selected)
	}
	if err := tt.Click("GET /api/projects/portglass"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"请求 / 输入", "Raw", "复制"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.side != 0 || !strings.Contains(tt.Clipboard(), "GET /api/projects/portglass HTTP/1.1") {
		t.Fatal("request/raw/copy flow failed", a.side, tt.Clipboard())
	}
	if err := tt.Click("TCP / TLS"); err != nil {
		t.Fatal(err)
	}
	if len(a.filtered()) != 0 || !tt.HasText("没有匹配的流量") {
		t.Fatal("protocol navigation failed")
	}
	if err := tt.Click("全部流量"); err != nil {
		t.Fatal(err)
	}
	if len(a.filtered()) != 7 {
		t.Fatal("all-traffic navigation failed")
	}
	for i, label := range []string{"浅色外观", "深色外观", "跟随系统"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if a.appearance != (i+1)%3 {
			t.Fatal("appearance control failed")
		}
	}
	for _, label := range []string{"监听端口", "网络接口", "开始监听", "导出会话", "内容查看器"} {
		r, ok := tt.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 1101 || r.Y+r.H > 681 {
			t.Fatalf("control %q outside compact window: %v", label, r)
		}
	}
	tt.SetPreferences(ui.Preferences{HighContrast: true, ReduceMotion: true})
	if !a.reducedTransparency {
		t.Fatal("high-contrast fallback not applied")
	}
}

func TestHistoryBoundAndSelection(t *testing.T) {
	a := New()
	for i := 0; i < traffic.MaxEntries+20; i++ {
		a.upsert(traffic.Exchange{ID: int64(i + 1), Path: fmt.Sprint(i)})
	}
	if len(a.entries) != traffic.MaxEntries || a.evicted != 20 {
		t.Fatal(len(a.entries), a.evicted)
	}
	if _, ok := a.index[a.selected]; !ok {
		t.Fatal("selection points to evicted exchange")
	}
	for i, x := range a.entries {
		if a.index[x.ID] != i {
			t.Fatal("index is inconsistent")
		}
	}
}

// PORTGLASS_SCREENSHOTS is opt-in so ordinary tests never write artifacts.
func TestVisualLayouts(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{{"wide", 1380, 860}, {"compact", 1100, 680}} {
		for _, dark := range []bool{false, true} {
			a := New()
			a.appearance = 0
			a.loadDemo()
			tt := ui.NewTester(a.view, size.width, size.height)
			tt.SetDark(dark)
			if !tt.HasText("导出会话") || !tt.HasText("内容格式") {
				t.Fatal("essential controls missing")
			}
			if out := os.Getenv("PORTGLASS_SCREENSHOTS"); out != "" {
				if err := os.MkdirAll(out, 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(out, fmt.Sprintf("%s-dark-%t.png", size.name, dark))
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
