package site

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Contraste minimo da WCAG para texto normal. Comentario de codigo e texto
// normal, e a paleta escura do chroma entrega o comentario em 3,8:1.
const minimumContrast = 4.5

type color struct{ red, green, blue float64 }

func parseColor(text string) (color, bool) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "#")
	if len(text) == 3 {
		text = string([]byte{text[0], text[0], text[1], text[1], text[2], text[2]})
	}
	if len(text) != 6 {
		return color{}, false
	}
	value, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return color{}, false
	}
	return color{
		red:   float64(value>>16&0xff) / 255,
		green: float64(value>>8&0xff) / 255,
		blue:  float64(value&0xff) / 255,
	}, true
}

func (c color) hex() string {
	round := func(channel float64) int { return int(math.Round(math.Max(0, math.Min(1, channel)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", round(c.red), round(c.green), round(c.blue))
}

func (c color) luminance() float64 {
	channel := func(value float64) float64 {
		if value <= 0.04045 {
			return value / 12.92
		}
		return math.Pow((value+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.red) + 0.7152*channel(c.green) + 0.0722*channel(c.blue)
}

func Contrast(first, second string) float64 {
	a, okFirst := parseColor(first)
	b, okSecond := parseColor(second)
	if !okFirst || !okSecond {
		return 0
	}
	lighter, darker := a.luminance(), b.luminance()
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

// Clarear ou escurecer misturando com o extremo, e nao mexendo na saturacao: a
// cor continua reconhecivel como a mesma do tema, so afasta do fundo.
func readable(text, background string) string {
	if Contrast(text, background) >= minimumContrast {
		return text
	}
	foreground, ok := parseColor(text)
	if !ok {
		return text
	}
	surface, ok := parseColor(background)
	if !ok {
		return text
	}
	toward := color{red: 1, green: 1, blue: 1}
	if surface.luminance() > 0.4 {
		toward = color{}
	}
	for step := 1; step <= 40; step++ {
		amount := float64(step) / 40
		mixed := color{
			red:   foreground.red + (toward.red-foreground.red)*amount,
			green: foreground.green + (toward.green-foreground.green)*amount,
			blue:  foreground.blue + (toward.blue-foreground.blue)*amount,
		}
		if Contrast(mixed.hex(), background) >= minimumContrast {
			return mixed.hex()
		}
	}
	return toward.hex()
}

var (
	colorDeclaration = regexp.MustCompile(`(?:^|[^-])color: (#[0-9a-fA-F]{3,6})`)
	ownBackground    = regexp.MustCompile(`background-color: #`)
	codeBackground   = regexp.MustCompile(`--fundo-codigo: (#[0-9a-fA-F]{3,6})`)
)

// A paleta do chroma nao foi feita para este fundo, e uma regra que nasce fora
// do AA passa despercebida: quem le codigo em comentario cinza claro nao
// reclama, so nao le.
func withReadableColors(css, background string) string {
	var written strings.Builder
	for _, line := range strings.Split(css, "\n") {
		if ownBackground.MatchString(line) {
			written.WriteString(line + "\n")
			continue
		}
		fixed := colorDeclaration.ReplaceAllStringFunc(line, func(match string) string {
			parts := colorDeclaration.FindStringSubmatch(match)
			return strings.Replace(match, parts[1], readable(parts[1], background), 1)
		})
		written.WriteString(fixed + "\n")
	}
	return strings.TrimSuffix(written.String(), "\n")
}

// Os dois fundos de bloco saem da propria folha: repetir o valor aqui criaria
// uma segunda definicao para alguem esquecer de mudar junto.
func codeBackgrounds() (string, string) {
	matches := codeBackground.FindAllStringSubmatch(stylesheet, -1)
	if len(matches) < 2 {
		return "#ffffff", "#000000"
	}
	return matches[0][1], matches[len(matches)-1][1]
}
