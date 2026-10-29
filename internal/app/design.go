package app

import (
	"embed"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Icons are unmodified Lucide assets; see THIRD_PARTY.md.
//
//go:embed icons/*.svg
var iconFiles embed.FS

var icons = func() map[string]*ui.SVG {
	result := map[string]*ui.SVG{}
	files, _ := iconFiles.ReadDir("icons")
	for _, f := range files {
		data, _ := iconFiles.ReadFile("icons/" + f.Name())
		result[strings.TrimSuffix(f.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	return result
}()

type palette struct {
	canvas, panel, inset, line, muted, accent, success, glowA, glowB ui.Color
}

func (a *App) theme(c *ui.Context) palette {
	t := *c.Theme()
	if a.appearance == 1 {
		t.Dark = false
	} else if a.appearance == 2 {
		t.Dark = true
	}
	p := palette{
		canvas: ui.Hex("#E8EEF2"), panel: ui.Hex("#FAFCFD"), inset: ui.Hex("#F0F4F7"),
		line: ui.Hex("#D8E1E8"), muted: ui.Hex("#647586"), accent: ui.Hex("#2678BE"), success: ui.Hex("#16816D"),
		glowA: ui.Hex("#A5CBDC"), glowB: ui.Hex("#CCD7EF"),
	}
	if t.Dark {
		p = palette{
			canvas: ui.Hex("#111A24"), panel: ui.Hex("#16212C"), inset: ui.Hex("#111C26"),
			line: ui.Hex("#2A3A47"), muted: ui.Hex("#9AAEBD"), accent: ui.Hex("#84CEF1"), success: ui.Hex("#78D5B4"),
			glowA: ui.Hex("#254E60"), glowB: ui.Hex("#313D59"),
		}
		t.Text = ui.Hex("#E8F0F5")
	} else {
		t.Text = ui.Hex("#1E3346")
	}
	t.Background, t.Surface = p.panel, p.inset
	t.SurfaceHover, t.SurfacePressed = p.accent.Alpha(0.10), p.accent.Alpha(0.17)
	t.Border, t.TextMuted = p.line, p.muted
	t.Accent, t.AccentHover, t.AccentPressed = p.accent, p.accent, p.accent
	t.AccentText = ui.Hex("#FFFFFF")
	t.Selection, t.Focus = p.accent.Alpha(0.2), p.accent.Alpha(0.75)
	t.Radius, t.FontSize = 10, 13
	if c.Preferences().HighContrast {
		t.Border, p.line = t.TextMuted, t.TextMuted
		t.TextMuted, p.muted = t.Text, t.Text
	}
	c.SetTheme(&t)
	a.reducedTransparency = c.Preferences().HighContrast
	if a.win != nil {
		a.reducedTransparency = a.reducedTransparency || mygo.Theme.Preferences().ReduceTransparency
	}
	return p
}

// Only chrome uses the lens material. Content and code stay on quiet surfaces.
// MyGo evaluates this material with Metal on macOS; the CPU path also works for
// screenshots and systems where the GPU renderer is unavailable.
func (a *App) glass(c *ui.Context, e ui.Element, tint ui.Color, interactive bool) ui.Element {
	if a.reducedTransparency {
		return e.Background(c.Theme().Surface).Border(1, c.Theme().Border)
	}
	return e.Material(glass.Glass{Style: glass.Clear, Tint: tint.Alpha(0.10), Clarity: 0.55, Interactive: interactive})
}

func glassButton(tint ui.Color) glass.Glass {
	return glass.Glass{Tint: tint.Alpha(0.94), Interactive: true}
}

func glyph(c *ui.Context, name string, size float32, color ui.Color) {
	ui.Icon(c, icons[name]).FontSize(size).TextColor(color)
}

func quietButton(c *ui.Context, name, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Height(30).Padding(0, 10).Gap(6).Radius(8).AlignItems(ui.Center).TextColor(t.TextMuted)
	if b.Hovered() {
		b.Background(t.SurfaceHover).TextColor(t.Text)
	}
	b.Children(func() {
		glyph(c, name, 14, t.TextMuted)
		ui.Text(c, label).FontSize(11)
	})
	return b
}

func (a *App) setAppearance(mode int) {
	a.appearance = mode
	if a.win != nil {
		mygo.Theme.SetSource([]mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark}[mode])
		a.saveConfig()
	}
}

func codeTheme(t *ui.Theme, p palette) *editor.Theme {
	ct := editor.DefaultTheme(t)
	ct.Background, ct.LineNumber, ct.IndentGuide = p.panel, p.muted.Alpha(0.65), p.line.Alpha(0.65)
	if t.Dark {
		ct.Tokens[editor.TokenProperty].Color = ui.Hex("#9DBFD9")
		ct.Tokens[editor.TokenString].Color = ui.Hex("#9DE0C0")
		ct.Tokens[editor.TokenNumber].Color = ui.Hex("#DEBF89")
		ct.Tokens[editor.TokenKeyword].Color = ui.Hex("#CCAEE5")
		ct.Tokens[editor.TokenConstant].Color = ui.Hex("#CCAEE5")
	} else {
		ct.Tokens[editor.TokenProperty].Color = ui.Hex("#446A90")
		ct.Tokens[editor.TokenString].Color = ui.Hex("#207D65")
		ct.Tokens[editor.TokenNumber].Color = ui.Hex("#95652C")
	}
	return ct
}
