package theme

import (
	"image/color"
	"io/fs"
	"os"
	"path/filepath"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type cnTheme struct {
	base fyne.Theme
	font fyne.Resource
}

func (t *cnTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, v)
}

func (t *cnTheme) Font(s fyne.TextStyle) fyne.Resource {
	if t.font != nil {
		return t.font
	}
	return theme.DefaultTheme().Font(s)
}

func (t *cnTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (t *cnTheme) Size(n fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(n)
}

// UseCNFontIfAvailable tries to set a macOS CJK font so Chinese labels render correctly
func UseCNFontIfAvailable(a fyne.App) {
	candidates := []string{
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/Hiragino Sans GB W3.otf",
		"/System/Library/Fonts/Hiragino Sans GB W6.otf",
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			res := fyne.NewStaticResource(filepath.Base(p), b)
			th := &cnTheme{base: theme.DefaultTheme(), font: res}
			a.Settings().SetTheme(th)
			return
		}
		if err != nil {
			if _, ok := err.(*fs.PathError); !ok {
				// ignore other errors
			}
		}
	}
	// no suitable font found: keep default theme
}
