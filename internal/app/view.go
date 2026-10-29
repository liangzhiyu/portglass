package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

func (a *App) view(c *ui.Context) {
	p := a.theme(c)
	c.Root().Background(p.canvas)
	ui.Box(c).Fill().Clip().Draw(func(painter *ui.Painter, r ui.Rect) {
		// Paint behind the content, never in a pointer-catching overlay. These
		// still gradients need no animation or full-window blur.
		// Glass samples this opaque in-window backdrop. Sampling a transparent
		// Metal target would turn light glass gray before the OS composites it.
		painter.FillGradient(r, ui.LinearGradient{From: p.glowA.Alpha(0.50), To: p.canvas, Angle: 135}, 0)
		painter.FillGradient(r, ui.LinearGradient{From: p.glowB.Alpha(0.26), To: p.glowB.Alpha(0), Angle: 180}, 0)
	}).Children(func() {
		ui.Column(c).Fill().Padding(0, 20, 18).Children(func() {
			a.titlebar(c, p)
			ui.Row(c).Grow(1).FillWidth().Gap(18).AlignItems(ui.Stretch).Children(func() {
				a.sidebar(c, p)
				ui.Column(c).Grow(1).MinWidth(0).Gap(14).Children(func() {
					a.captureBar(c, p)
					ui.Row(c).Grow(1).FillWidth().Gap(12).AlignItems(ui.Stretch).Children(func() {
						a.trafficList(c, p)
						a.inspector(c, p)
					})
				})
			})
		})
	})
}

func (a *App) titlebar(c *ui.Context, p palette) {
	bar := c.TitleBar()
	ui.Row(c).Height(58).Padding(0, 2, 0, max(bar.Left-6, 76)).Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, "PortGlass").FontSize(17).FontWeight(650).LetterSpacing(-0.5)
		ui.Text(c, "/").FontSize(15).TextColor(p.muted.Alpha(0.5)).Margin(0, 6)
		ui.Text(c, "网络检查器").FontSize(12).TextColor(p.muted)
		ui.Box(c).Grow(1)
		label, color := "准备就绪", p.muted
		if a.demo {
			label, color = "示例会话", p.accent
		} else if a.running {
			label, color = "正在监听", p.success
		} else if a.busy {
			label, color = "等待授权", p.accent
		}
		ui.Row(c).Gap(6).Padding(6, 10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(5, 5).Radius(3).Background(color)
			ui.Text(c, label).FontSize(11).TextColor(color)
		})
		quietButton(c, "download", "导出会话").Disabled(len(a.entries) == 0).OnClick(a.export)
	})
}

func (a *App) sidebar(c *ui.Context, p palette) {
	w, _ := c.Size()
	width := float32(184)
	if w < 1200 {
		width = 158
	}
	ui.Column(c).Width(width).Padding(18, 4, 4, 0).Gap(24).Children(func() {
		ui.Column(c).Padding(0, 12).Gap(7).Children(func() {
			ui.Text(c, "工作区").FontSize(10).TextColor(p.muted).LetterSpacing(1.1)
			ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
				glyph(c, "network", 19, p.accent)
				ui.Text(c, "本地流量").FontSize(17).FontWeight(600).LetterSpacing(-0.3)
			})
		})
		counts := [3]int{len(a.entries), 0, 0}
		for _, x := range a.entries {
			if x.Protocol == "HTTP/1" {
				counts[1]++
			} else {
				counts[2]++
			}
		}
		ui.Column(c).Gap(6).FillWidth().Label("协议过滤").Children(func() {
			for i, label := range []string{"全部流量", "HTTP", "TCP / TLS"} {
				i := i
				b := ui.ButtonBase(c.Key(label)).Label(label).FillWidth().Height(40).Padding(0, 12).Radius(11).Gap(10).AlignItems(ui.Center)
				color := p.muted
				if a.protocol == i {
					a.glass(c, b, p.accent, true)
					color = c.Theme().Text
				} else if b.Hovered() {
					b.Background(p.accent.Alpha(0.07))
				}
				b.OnClick(func() { a.protocol = i }).Children(func() {
					glyph(c, []string{"layers", "braces", "cable"}[i], 15, color)
					ui.Text(c, label).FontSize(12).TextColor(color).Grow(1)
					ui.Text(c, strconv.Itoa(counts[i])).FontSize(10).Font("monospace").TextColor(p.muted)
				})
			}
		})
		ui.Column(c).FillWidth().Padding(14, 12).Gap(16).BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.8)).Children(func() {
			ui.Text(c, "会话概览").FontSize(10).TextColor(p.muted).LetterSpacing(0.5)
			ui.Row(c).AlignItems(ui.Baseline).Gap(7).Children(func() {
				ui.Text(c, strconv.Itoa(len(a.entries))).FontSize(38).FontWeight(400).LetterSpacing(-1.5)
				ui.Text(c, "条记录").FontSize(10).TextColor(p.muted)
			})
			a.metric(c, "数据包", fmt.Sprint(a.packets), p)
			a.metric(c, "内核丢包", fmt.Sprint(a.dropped), p)
			if a.evicted > 0 {
				ui.Text(c, fmt.Sprintf("已移除 %d 条旧记录", a.evicted)).FontSize(10).TextColor(p.muted).MaxLines(2)
			}
		})
		ui.Box(c).Grow(1)
		ui.Column(c).FillWidth().Gap(12).Padding(0, 10).Children(func() {
			if a.errorText != "" {
				ui.Text(c, a.errorText).FontSize(11).TextColor(c.Theme().Danger).MaxLines(6).Selectable()
			} else if a.notice != "" && !a.demo {
				ui.Text(c, a.notice).FontSize(11).TextColor(p.muted).MaxLines(3)
			}
			quietButton(c, "play", "载入示例").FillWidth().Disabled(a.running || a.busy).OnClick(a.loadDemo)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				glyph(c, "shield-check", 13, p.muted)
				ui.Text(c, "仅在本机处理").FontSize(10).TextColor(p.muted)
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				for i, name := range []string{"monitor", "sun", "moon"} {
					i := i
					label := []string{"跟随系统", "浅色外观", "深色外观"}[i]
					b := ui.ButtonBase(c.Key(name)).Label(label).Tooltip(label).Size(30, 28).Radius(8).Justify(ui.Center).AlignItems(ui.Center)
					if a.appearance == i {
						b.Background(p.panel.Alpha(0.8)).Border(1, p.line.Alpha(0.8))
					} else if b.Hovered() {
						b.Background(p.accent.Alpha(0.08))
					}
					b.OnClick(func() { a.setAppearance(i) }).Children(func() { glyph(c, name, 13, p.muted) })
				}
				ui.Box(c).Grow(1)
				ui.Text(c, "0.1.0").FontSize(9).TextColor(p.muted.Alpha(0.7))
			})
		})
	})
}

func (a *App) metric(c *ui.Context, label, value string, p palette) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(p.muted).Grow(1)
		ui.Text(c, value).FontSize(11).Font("monospace")
	})
}

func (a *App) captureBar(c *ui.Context, p palette) {
	w, _ := c.Size()
	a.glass(c, ui.Row(c).FillWidth().Height(68).Padding(9, 18).Gap(16).Radius(20).AlignItems(ui.Center), p.canvas, false).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			glyph(c, "radio", 21, p.accent)
			if w >= 1250 {
				ui.Column(c).Gap(3).Children(func() {
					ui.Text(c, "监听目标").FontSize(12).FontWeight(600)
					ui.Text(c, "捕获请求与返回").FontSize(10).TextColor(p.muted)
				})
			}
		})
		ui.Box(c).Width(1).Height(30).Background(p.line)
		ui.Column(c).Gap(3).Width(148).Children(func() {
			ui.Text(c, "端口").FontSize(10).TextColor(p.muted)
			ui.TextInput(c, &a.ports).Label("监听端口").Placeholder("7010, 8080").Tooltip("多个端口用逗号分隔").Font("monospace").FontSize(13).Height(28).FillWidth().Radius(7).Background(p.panel.Alpha(0.55)).Border(1, p.line.Alpha(0.6)).Disabled(a.running || a.busy)
		})
		ui.Column(c).Gap(3).Width(136).Children(func() {
			ui.Text(c, "网络接口").FontSize(10).TextColor(p.muted)
			ui.Select(c, &a.iface, a.interfaces).Label("网络接口").Height(28).FillWidth().FontSize(12).Radius(7).Background(p.panel.Alpha(0.55)).Border(1, p.line.Alpha(0.6)).Disabled(a.running || a.busy)
		})
		ui.Box(c).Grow(1)
		if w >= 1250 {
			desc := "网络接口"
			if a.iface == "lo0" {
				desc = "本机回环"
			}
			ui.Text(c, desc).FontSize(11).TextColor(p.muted)
		}
		label, name := "开始监听", "play"
		if a.running || a.busy {
			label, name = "停止监听", "square"
		}
		b := ui.ButtonBase(c).Label(label).Height(38).Padding(0, 20).Gap(8).Radius(19).AlignItems(ui.Center).Justify(ui.Center)
		tint := ui.Hex("#327EB4")
		if a.running || a.busy {
			tint = ui.Hex("#975F59")
		}
		if a.reducedTransparency {
			b.Background(tint)
		} else {
			b.Material(glassButton(tint))
		}
		b.Children(func() {
			glyph(c, name, 13, ui.Hex("#FFFFFF"))
			ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
		})
		if a.running || a.busy {
			b.OnClick(a.Stop)
		} else {
			b.OnClick(a.Start)
		}
	})
}

func (a *App) trafficList(c *ui.Context, p palette) {
	w, _ := c.Size()
	width := float32(364)
	if w < 1200 {
		width = 310
	}
	ui.Column(c).Width(width).Radius(18).Background(p.panel.Alpha(0.84)).Border(1, p.line.Alpha(0.7)).Clip().Children(func() {
		ui.Column(c).FillWidth().Padding(18, 18, 14).Gap(14).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "请求").FontSize(16).FontWeight(600).Grow(1)
				quietButton(c, "trash", "清空").Disabled(len(a.entries) == 0).OnClick(a.clear)
			})
			ui.Row(c).Height(34).Gap(8).Padding(0, 10).AlignItems(ui.Center).Radius(9).Background(p.inset).Border(1, p.line.Alpha(0.7)).Children(func() {
				glyph(c, "search", 14, p.muted)
				ui.TextInputBase(c, &a.query).Label("搜索流量").Placeholder("搜索路径、状态或内容").Grow(1).MinWidth(0).FontSize(11)
			})
		})
		items := a.filtered()
		ui.Row(c).FillWidth().Padding(0, 20, 10).Children(func() {
			ui.Text(c, "请求 / 连接").FontSize(10).TextColor(p.muted).Grow(1)
			ui.Text(c, "状态").FontSize(10).TextColor(p.muted)
		})
		a.list.Key = func(i int) any { return items[i].ID }
		a.list.Label = func(i int) string { return traffic.Description(items[i]) }
		ui.List(c, &a.list, len(items), func(i int) { a.requestRow(c, items[i], p) }).Label("请求列表").Focusable().
			OnShortcut(0, ui.KeyDown, func() { a.moveSelection(items, 1) }).
			OnShortcut(0, ui.KeyUp, func() { a.moveSelection(items, -1) }).
			Grow(1).FillWidth().Padding(0, 8).Children(func() {
			if len(items) == 0 {
				title, desc := "等待第一条请求", "输入端口并开始监听，\n流量会自动出现在这里。"
				if a.running {
					desc = "监听已开始，\n向所选端口发送请求即可查看。"
				} else if a.query != "" || a.protocol != 0 {
					title, desc = "没有匹配的流量", "试试其他关键词或切换协议。"
				}
				ui.Column(c).FillWidth().Padding(40, 16).Gap(14).AlignItems(ui.Center).Children(func() {
					glyph(c, "activity", 30, p.accent.Alpha(0.7))
					ui.Text(c, title).FontSize(13).FontWeight(500)
					ui.Text(c, desc).FontSize(11).TextColor(p.muted).LineHeight(1.7)
				})
			}
		})
		ui.Row(c).FillWidth().Height(38).Padding(0, 18).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.65)).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 条可见", len(items))).FontSize(10).TextColor(p.muted).Grow(1)
			glyph(c, "arrow-down", 11, p.muted)
			ui.Text(c, "最新在前").FontSize(10).TextColor(p.muted).Margin(0, 0, 0, 4)
		})
	})
}

func (a *App) requestRow(c *ui.Context, x traffic.Exchange, p palette) {
	selected := x.ID == a.selected
	row := ui.Column(c.Key(x.ID)).FillWidth().Padding(10, 11).Margin(2, 0).Gap(6).Radius(10).Label(traffic.Description(x))
	if selected {
		row.Gradient(p.accent.Alpha(0.17), p.accent.Alpha(0.055), 110).Border(1, p.accent.Alpha(0.24))
	} else {
		row.Border(1, ui.Transparent)
		if row.Hovered() {
			row.Background(p.inset)
		}
	}
	row.OnClick(func() { a.selected = x.ID; a.list.Focus(c) }).Children(func() {
		method := x.Method
		if method == "" {
			method = x.Protocol
		}
		color := p.success
		if method == "POST" {
			color = p.accent
		} else if x.Protocol == "TLS" {
			color = ui.Hex("#A58FCD")
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, method).FontSize(9).FontWeight(700).TextColor(color).Width(35)
			path := x.Path
			if path == "" {
				path = x.Server
			}
			ui.Text(c, path).FontSize(12).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
			status := "…"
			if x.Status > 0 {
				status = strconv.Itoa(x.Status)
			} else if x.Protocol == "TLS" {
				status = "TLS"
			} else if x.Protocol == "TCP" {
				status = "TCP"
			}
			statusColor := p.success
			if x.Status >= 400 {
				statusColor = c.Theme().Danger
			}
			ui.Text(c, status).FontSize(10).Font("monospace").TextColor(statusColor)
		})
		ui.Row(c).Gap(6).Padding(0, 0, 0, 43).Children(func() {
			ui.Text(c, x.Server).FontSize(10).TextColor(p.muted).Grow(1).MinWidth(0).SingleLine()
			label := bytesLabel(x.Bytes)
			if x.Status > 0 {
				label = fmt.Sprintf("%.0f ms", x.DurationMS)
			}
			ui.Text(c, label).FontSize(10).Font("monospace").TextColor(p.muted)
		})
		ui.Text(c, x.Time.Format("15:04:05.000")).FontSize(9).Font("monospace").TextColor(p.muted.Alpha(0.8)).Padding(0, 0, 0, 43)
	})
}

func (a *App) moveSelection(items []traffic.Exchange, delta int) {
	if len(items) == 0 {
		return
	}
	index := 0
	for i, x := range items {
		if x.ID == a.selected {
			index = min(max(i+delta, 0), len(items)-1)
			break
		}
	}
	a.selected = items[index].ID
	a.list.ScrollIntoView(index)
}

func (a *App) inspector(c *ui.Context, p palette) {
	ui.Column(c).Grow(1).MinWidth(0).Radius(18).Background(p.panel).Border(1, p.line.Alpha(0.7)).Clip().Children(func() {
		idx, ok := a.index[a.selected]
		if !ok {
			ui.Column(c).Fill().Gap(16).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				a.glass(c, ui.Row(c).Size(68, 68).Radius(22).AlignItems(ui.Center).Justify(ui.Center), p.accent, false).Children(func() { glyph(c, "arrow-up-down", 28, p.accent) })
				ui.Text(c, "每一次往返，清晰可见").FontSize(19).FontWeight(500).LetterSpacing(-0.3)
				ui.Text(c, "选择一条请求，查看输入与输出。").FontSize(12).TextColor(p.muted)
				ui.Text(c, "Body   /   Headers   /   Raw   /   Hex").FontSize(10).Font("monospace").TextColor(p.muted.Alpha(0.8)).Margin(8, 0)
			})
			return
		}
		x := a.entries[idx]
		ui.Column(c).FillWidth().Padding(16, 24, 12).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				glyph(c, "activity", 13, p.muted)
				ui.Text(c, "连接详情").FontSize(10).TextColor(p.muted).Grow(1)
				ui.Text(c, x.Protocol).FontSize(10).Font("monospace").TextColor(p.muted)
			})
			ui.Text(c, traffic.Description(x)).FontSize(19).FontWeight(600).LetterSpacing(-0.4).SingleLine().FillWidth()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, x.Client).FontSize(10).Font("monospace").TextColor(p.muted).MinWidth(0).SingleLine()
				glyph(c, "arrow-right", 12, p.muted)
				ui.Text(c, x.Server).FontSize(10).Font("monospace").TextColor(p.muted).MinWidth(0).SingleLine()
			})
			ui.Row(c).Gap(20).AlignItems(ui.Center).Children(func() {
				status := "传输中"
				if x.Complete {
					status = "已完成"
				}
				if x.Status > 0 {
					status = fmt.Sprintf("%d %s", x.Status, http.StatusText(x.Status))
				}
				color := p.success
				if x.Status >= 400 {
					color = c.Theme().Danger
				}
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() { glyph(c, "check", 12, color); ui.Text(c, status).FontSize(11).TextColor(color).FontWeight(500) })
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					glyph(c, "clock", 12, p.muted)
					ui.Text(c, fmt.Sprintf("%.1f ms", x.DurationMS)).FontSize(11).TextColor(p.muted)
				})
				ui.Text(c, bytesLabel(x.Bytes)).FontSize(11).TextColor(p.muted)
			})
		})
		ui.Row(c).FillWidth().Padding(0, 24, 12).Children(func() {
			g := ui.SegmentedBase(c.Key("direction"), &a.side, 2)
			g.Track.Label("流量方向").FillWidth().Padding(4).Gap(4).Radius(12).Background(p.inset).Border(1, p.line.Alpha(0.65)).Children(func() {
				for i, label := range []string{"请求 / 输入", "响应 / 输出"} {
					s := g.Segment(i).Label(label).Grow(1).Height(28).Radius(9).Gap(7).AlignItems(ui.Center).Justify(ui.Center)
					color := p.muted
					if a.side == i {
						color = c.Theme().Text
						a.glass(c, s, p.panel, true)
					}
					s.Children(func() {
						glyph(c, []string{"arrow-up", "arrow-down"}[i], 13, color)
						ui.Text(c, label).FontSize(12).TextColor(color).FontWeight(500)
					})
				}
			})
		})
		ui.Row(c).FillWidth().Height(38).Padding(0, 16).Gap(2).AlignItems(ui.Center).BorderWidth(1, 0, 1, 0).BorderColor(p.line.Alpha(0.6)).Children(func() {
			tabs := ui.SegmentedBase(c.Key("format"), &a.format, 4)
			tabs.Track.Label("内容格式").Gap(3).Children(func() {
				for i, label := range []string{"Body", "Headers", "Raw", "Hex"} {
					s := tabs.Segment(i).Label(label).Height(28).Padding(0, 11).Radius(7).AlignItems(ui.Center)
					color := p.muted
					if i == a.format {
						color = p.accent
						s.Background(p.accent.Alpha(0.09))
					}
					s.Children(func() { ui.Text(c, label).FontSize(11).TextColor(color).FontWeight(500) })
				}
			})
			ui.Box(c).Grow(1)
			quietButton(c, "copy", "复制").OnClick(func() { c.WriteClipboard(a.currentText(x)) })
		})
		content := a.currentText(x)
		if content != a.codeText {
			a.codeText = content
			a.code.Buffer.Reset(content)
			lang := "text.txt"
			if a.format == 0 && (strings.HasPrefix(strings.TrimSpace(content), "{") || strings.HasPrefix(strings.TrimSpace(content), "[")) {
				lang = "body.json"
			}
			a.code.Buffer.SetLanguage(editor.LanguageFor(lang))
		}
		a.code.Theme = codeTheme(c.Theme(), p)
		ui.Box(c).Grow(1).FillWidth().MinWidth(0).Padding(14, 10).Children(func() { editor.View(c, a.code).Fill().MinWidth(0) })
		m := x.Request
		if a.side == 1 {
			m = x.Response
		}
		if m.Truncated {
			ui.Text(c, "内容显示上限为 64 KB；此条内容已截断。").FontSize(10).TextColor(c.Theme().Warning).Padding(8, 20)
		}
		if x.Note != "" {
			ui.Text(c, x.Note).FontSize(10).TextColor(p.muted).Padding(8, 20).MaxLines(3)
		}
		ui.Row(c).FillWidth().Height(38).Padding(0, 20).Gap(6).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.line.Alpha(0.65)).Children(func() {
			glyph(c, "braces", 12, p.muted)
			label := "可选择文本"
			if a.demo {
				label = "示例数据 · 非实际捕获"
			}
			ui.Text(c, label).FontSize(10).TextColor(p.muted).Grow(1)
			ui.Text(c, "⌘C 复制   ⌘F 查找").FontSize(10).TextColor(p.muted)
		})
	})
}

func (a *App) currentText(x traffic.Exchange) string {
	if x.Protocol == "TLS" && a.format == 0 {
		return "此连接使用 TLS 加密。\n\nPortGlass 当前显示连接信息和加密字节，\n无法直接读取 HTTPS 请求或响应正文。\n\n切换到 Raw / Hex 可查看捕获的字节。"
	}
	m := x.Request
	if a.side == 1 {
		m = x.Response
	}
	text := m.Text([]string{"Body", "Headers", "Raw", "Hex"}[a.format])
	if strings.TrimSpace(text) == "" {
		if a.side == 1 && !x.Complete {
			return "等待响应…"
		}
		return "此方向没有正文。"
	}
	return text
}
