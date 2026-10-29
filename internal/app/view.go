package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/liangzhiyu/portglass/internal/traffic"
)

var blue = ui.Hex("#3876F6")
var green = ui.Hex("#299A72")

func (a *App) view(c *ui.Context) {
	t := *c.Theme()
	t.Accent = blue
	t.Radius = 9
	c.SetTheme(&t)
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Column(c).Fill().Children(func() {
		a.titlebar(c)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			ui.Row(c).Grow(1).MinWidth(0).Padding(0, 14, 14, 0).Gap(12).AlignItems(ui.Stretch).Children(func() {
				a.trafficList(c)
				a.inspector(c)
			})
		})
	})
}

func (a *App) titlebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	ui.Row(c).Height(68).Padding(0, 24, 0, max(bar.Left+14, 96)).Gap(12).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, "PortGlass").FontSize(19).Bold().LetterSpacing(-0.4)
		ui.Text(c, "PORT INSPECTOR").FontSize(10).TextColor(t.TextMuted).LetterSpacing(1.4)
		ui.Box(c).Grow(1)
		status, col := "待机", t.TextMuted
		if a.demo {
			status = "示例会话"
			col = blue
		} else if a.running {
			status = "正在监听"
			col = green
		} else if a.busy {
			status = "等待授权"
			col = blue
		}
		ui.Row(c).Padding(6, 12).Radius(18).Background(col.Alpha(0.10)).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(col)
			ui.Text(c, status).FontSize(11).TextColor(col).Bold()
		})
		ui.Button(c, "导出会话").Disabled(len(a.entries) == 0).OnClick(a.export)
	})
}

func (a *App) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(224).Padding(8, 18, 20, 18).Gap(20).Children(func() {
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "看见每一次连接").FontSize(17).Bold().LetterSpacing(-0.3)
			ui.Text(c, "从端口，到请求与响应。").FontSize(11).TextColor(t.TextMuted)
		})
		ui.Column(c).FillWidth().Padding(15).Gap(12).Radius(16).Background(t.Background.Alpha(0.64)).Border(1, t.Border.Alpha(0.6)).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(blue)
				ui.Text(c, "监听目标").FontSize(12).Bold()
			})
			ui.Text(c, "端口").FontSize(11).TextColor(t.TextMuted)
			ui.TextInput(c, &a.ports).Label("监听端口").Placeholder("7010,8080").FillWidth().Font("monospace").Disabled(a.running || a.busy)
			ui.Text(c, "多个端口用逗号分隔").FontSize(10).TextColor(t.TextMuted)
			ui.Text(c, "网络接口").FontSize(11).TextColor(t.TextMuted)
			ui.Select(c, &a.iface, a.interfaces).Label("网络接口").FillWidth().Disabled(a.running || a.busy)
			desc := "监听该网卡的 TCP 流量"
			if a.iface == "lo0" {
				desc = "本机回环 · localhost"
			}
			ui.Text(c, desc).FontSize(10).TextColor(t.TextMuted)
			if a.running || a.busy {
				ui.Button(c, "■  停止监听").FillWidth().Height(36).OnClick(a.Stop)
			} else {
				ui.PrimaryButton(c, "开始监听").FillWidth().Height(36).OnClick(a.Start)
			}
		})
		ui.Column(c).Gap(12).FillWidth().Children(func() {
			ui.Text(c, "本次会话").FontSize(10).TextColor(t.TextMuted).LetterSpacing(1)
			a.metric(c, "已捕获", strconv.Itoa(len(a.entries))+" 条")
			a.metric(c, "数据包", fmt.Sprint(a.packets))
			a.metric(c, "内核丢包", fmt.Sprint(a.dropped))
		})
		if a.evicted > 0 {
			ui.Text(c, fmt.Sprintf("已移除 %d 条较早记录，限制内存占用。", a.evicted)).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Box(c).Grow(1)
		if a.errorText != "" {
			ui.Column(c).FillWidth().Padding(10).Radius(9).Background(ui.Hex("#D44747").Alpha(0.10)).Children(func() { ui.Text(c, a.errorText).FontSize(11).MaxLines(8).Selectable().TextColor(ui.Hex("#C64D4D")) })
		} else if a.notice != "" {
			ui.Text(c, a.notice).FontSize(11).TextColor(t.TextMuted).MaxLines(4)
		}
		ui.Column(c).Gap(9).Children(func() {
			ui.Button(c, "载入示例").FillWidth().Disabled(a.running || a.busy).OnClick(a.loadDemo)
			ui.Text(c, "本地处理 · 数据不上传").FontSize(10).TextColor(t.TextMuted)
			ui.Text(c, "PortGlass  0.1.0").FontSize(10).TextColor(t.TextMuted.Alpha(0.8))
		})
	})
}

func (a *App) metric(c *ui.Context, label, value string) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
		ui.Text(c, value).FontSize(12).Font("monospace").Bold()
	})
}

func (a *App) trafficList(c *ui.Context) {
	t := c.Theme()
	w, _ := c.Size()
	listWidth := float32(360)
	if w > 1250 {
		listWidth = 390
	}
	ui.Column(c).Width(listWidth).Radius(16).Background(t.Background.Alpha(0.94)).Border(1, t.Border.Alpha(0.7)).Clip().Children(func() {
		ui.Column(c).Padding(18, 18, 12, 18).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "流量").FontSize(18).Bold()
				ui.Text(c, strconv.Itoa(len(a.entries))).FontSize(10).TextColor(t.TextMuted).Padding(3, 7).Radius(6).Background(t.Surface)
				ui.Box(c).Grow(1)
				ui.Button(c, "清空").FontSize(11).Disabled(len(a.entries) == 0).OnClick(a.clear)
			})
			ui.TextInput(c, &a.query).Label("搜索流量").Placeholder("搜索路径、状态码或内容…").FillWidth()
			ui.Segmented(c, &a.protocol, "全部", "HTTP", "TCP / TLS").Label("协议过滤").FillWidth()
		})
		items := a.filtered()
		ui.Row(c).FillWidth().Padding(8, 18).Background(t.Surface.Alpha(0.6)).Children(func() {
			ui.Text(c, "请求 / 连接").FontSize(10).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "状态 · 耗时").FontSize(10).TextColor(t.TextMuted)
		})
		a.list.Key = func(i int) any { return items[i].ID }
		a.list.Label = func(i int) string { return traffic.Description(items[i]) }
		ui.List(c, &a.list, len(items), func(i int) { a.requestRow(c, items[i]) }).Grow(1).FillWidth().Children(func() {
			if len(items) == 0 {
				ui.Column(c).FillWidth().Padding(28, 24).Gap(12).Children(func() {
					title, desc := "等待第一条请求", "填写端口并开始监听，\n请求与响应将在这里出现。"
					if a.running {
						desc = "监听已开始。向所选端口发送请求，\n即可在这里查看。"
					} else if a.query != "" || a.protocol != 0 {
						title = "没有匹配的流量"
						desc = "试试其他关键词或切换协议。"
					}
					ui.Text(c, "↔").FontSize(34).TextColor(blue.Alpha(0.65))
					ui.Text(c, title).FontSize(14).Bold()
					ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).LineHeight(1.6)
				})
			}
		})
		ui.Row(c).FillWidth().Padding(10, 16).BorderWidth(1, 0, 0, 0).BorderColor(t.Border.Alpha(0.5)).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 条可见", len(items))).FontSize(10).TextColor(t.TextMuted).Grow(1)
			ui.Text(c, "最新在前").FontSize(10).TextColor(t.TextMuted)
		})
	})
}

func (a *App) requestRow(c *ui.Context, x traffic.Exchange) {
	t := c.Theme()
	selected := x.ID == a.selected
	row := ui.Column(c.Key(x.ID)).FillWidth().Padding(13, 16).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(t.Border.Alpha(0.4)).Label(traffic.Description(x))
	if selected {
		row.Background(blue.Alpha(0.085)).BorderWidth(0, 0, 1, 3).BorderColor(blue.Alpha(0.55))
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.OnClick(func() { a.selected = x.ID })
	row.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			method := x.Method
			if method == "" {
				method = x.Protocol
			}
			col := green
			if method == "POST" {
				col = blue
			}
			if x.Protocol == "TLS" {
				col = ui.Hex("#9B77CC")
			}
			ui.Text(c, method).FontSize(10).Bold().TextColor(col).Width(44)
			ui.Text(c, x.Path).FontSize(12).Bold().Grow(1).MinWidth(0).SingleLine()
			status := "…"
			if x.Status > 0 {
				status = strconv.Itoa(x.Status)
			} else if x.Protocol == "TLS" {
				status = "TLS"
			} else if x.Protocol == "TCP" {
				status = "TCP"
			}
			statusCol := green
			if x.Status >= 400 {
				statusCol = ui.Hex("#D1694F")
			}
			ui.Text(c, status).FontSize(10).Font("monospace").TextColor(statusCol).Padding(3, 6).Radius(5).Background(statusCol.Alpha(0.08))
		})
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, x.Time.Format("15:04:05.000")).FontSize(10).Font("monospace").TextColor(t.TextMuted)
			ui.Text(c, x.Server).FontSize(10).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine()
			label := bytesLabel(x.Bytes)
			if x.Status > 0 {
				label = fmt.Sprintf("%.0f ms", x.DurationMS)
			}
			ui.Text(c, label).FontSize(10).Font("monospace").TextColor(t.TextMuted)
		})
	})
}

func (a *App) inspector(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinWidth(0).Radius(16).Background(t.Background).Border(1, t.Border.Alpha(0.7)).Clip().Children(func() {
		idx, ok := a.index[a.selected]
		if !ok {
			ui.Column(c).Fill().Center().Gap(16).Padding(36).Children(func() {
				ui.Box(c).Size(68, 68).Radius(20).Gradient(blue.Alpha(0.13), ui.Hex("#AC94E9").Alpha(0.20), 135).Border(1, blue.Alpha(0.08)).Center().Children(func() { ui.Text(c, "↗").FontSize(32).TextColor(blue) })
				ui.Text(c, "连接的细节，一目了然").FontSize(21).Bold().LetterSpacing(-0.5)
				ui.Text(c, "选择一条请求，查看它的输入与输出。").FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					for _, label := range []string{"HTTP", "JSON", "TCP", "TLS"} {
						ui.Text(c, label).FontSize(10).Font("monospace").Padding(5, 10).Radius(7).Background(t.Surface).TextColor(t.TextMuted)
					}
				})
			})
			return
		}
		x := a.entries[idx]
		ui.Column(c).FillWidth().Padding(22, 24, 18, 24).Gap(13).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, x.Protocol).FontSize(10).TextColor(blue).Bold().Padding(4, 8).Radius(6).Background(blue.Alpha(0.08))
				ui.Text(c, "连接详情").FontSize(10).TextColor(t.TextMuted).Grow(1)
				ui.Text(c, x.Time.Format("15:04:05.000")).FontSize(10).Font("monospace").TextColor(t.TextMuted)
			})
			ui.Text(c, traffic.Description(x)).FontSize(19).Bold().SingleLine().FillWidth()
			ui.Text(c, x.Client+"  →  "+x.Server).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine().FillWidth().Selectable()
			ui.Row(c).Gap(22).Children(func() {
				status := "传输中"
				if x.Complete {
					status = "已完成"
				}
				if x.Status > 0 {
					status = fmt.Sprintf("%d", x.Status)
				}
				a.detailMetric(c, "状态", status)
				a.detailMetric(c, "耗时", fmt.Sprintf("%.1f ms", x.DurationMS))
				a.detailMetric(c, "已捕获", bytesLabel(x.Bytes))
			})
		})
		ui.Row(c).Padding(0, 22, 14, 22).FillWidth().Children(func() {
			ui.Segmented(c, &a.side, "↑  请求 / 输入", "↓  响应 / 输出").Label("流量方向").FillWidth()
		})
		ui.Row(c).FillWidth().Padding(10, 16).Gap(10).AlignItems(ui.Center).Background(t.Surface.Alpha(0.7)).BorderWidth(1, 0, 1, 0).BorderColor(t.Border.Alpha(0.5)).Children(func() {
			ui.Segmented(c, &a.format, "Body", "Headers", "Raw", "Hex").Label("内容格式")
			ui.Box(c).Grow(1)
			ui.Button(c, "复制").FontSize(11).OnClick(func() { c.WriteClipboard(a.currentText(x)) })
		})
		content := a.currentText(x)
		if content != a.codeText {
			a.codeText = content
			a.code.Buffer.Reset(content)
			lang := "text.txt"
			if a.format == 0 && strings.HasPrefix(strings.TrimSpace(content), "{") {
				lang = "body.json"
			}
			a.code.Buffer.SetLanguage(editor.LanguageFor(lang))
		}
		editor.View(c, a.code).Grow(1).FillWidth().MinWidth(0)
		m := x.Request
		if a.side == 1 {
			m = x.Response
		}
		if m.Truncated {
			ui.Text(c, "内容显示上限为 64 KB；此条内容已截断。").FontSize(10).TextColor(ui.Hex("#B27930")).Padding(8, 16)
		}
		if x.Note != "" {
			ui.Text(c, x.Note).FontSize(10).TextColor(t.TextMuted).Padding(8, 16).MaxLines(3)
		}
		ui.Row(c).FillWidth().Padding(10, 16).BorderWidth(1, 0, 0, 0).BorderColor(t.Border.Alpha(0.5)).Children(func() {
			label := "可选择文本 · ⌘C 复制 · ⌘F 查找"
			if a.demo {
				label = "示例数据 · 非实际捕获"
			}
			ui.Text(c, label).FontSize(10).TextColor(t.TextMuted)
		})
	})
}

func (a *App) detailMetric(c *ui.Context, label, value string) {
	ui.Column(c).Gap(5).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).FontSize(12).Font("monospace").Bold()
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
