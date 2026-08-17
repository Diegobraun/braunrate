package site

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// A dobra e declarada no proprio markdown da pagina inicial, e nao montada no
// gerador: o texto que a pessoa le primeiro tem que viver onde o resto do texto
// vive, e chegar por pull request como os guias chegam.
type Hero struct {
	Motto   string
	Summary string
	Command string
	Facts   []string
	Actions []Action
	Proof   string
	Sides   []Side
	Balance string
}

type Action struct {
	Label string
	Href  string
}

type Side struct {
	Name   string
	Number string
	Phrase string
}

var heroBlock = regexp.MustCompile("(?s)```hero\n(.*?)```\n?")

func extractHero(markdown string) (*Hero, string) {
	match := heroBlock.FindStringSubmatch(markdown)
	if match == nil {
		return nil, markdown
	}
	hero := &Hero{}
	for _, line := range strings.Split(match[1], "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "motto":
			hero.Motto = value
		case "summary":
			hero.Summary = value
		case "command":
			hero.Command = value
		case "facts":
			hero.Facts = fields(value)
		case "action":
			if parts := fields(value); len(parts) == 2 {
				hero.Actions = append(hero.Actions, Action{Label: parts[0], Href: parts[1]})
			}
		case "proof":
			hero.Proof = value
		case "side":
			if parts := fields(value); len(parts) == 3 {
				hero.Sides = append(hero.Sides, Side{Name: parts[0], Number: parts[1], Phrase: parts[2]})
			}
		case "balance":
			hero.Balance = value
		}
	}
	return hero, heroBlock.ReplaceAllString(markdown, "")
}

func fields(value string) []string {
	parts := strings.Split(value, "|")
	for index, part := range parts {
		parts[index] = strings.TrimSpace(part)
	}
	return parts
}

func (hero *Hero) render(text chrome) string {
	var written strings.Builder
	written.WriteString("    <section class=\"hero\">\n")
	written.WriteString("      <h1>braunrate</h1>\n")
	fmt.Fprintf(&written, "      <p class=\"motto\">%s</p>\n", html.EscapeString(hero.Motto))
	fmt.Fprintf(&written, "      <p class=\"summary\">%s</p>\n", html.EscapeString(hero.Summary))

	written.WriteString("      <div class=\"call\">\n")
	fmt.Fprintf(&written, "        <code class=\"command\" id=\"hero-command\">%s</code>"+
		"<button type=\"button\" class=\"copy-command\" data-target=\"hero-command\">%s</button>\n",
		html.EscapeString(hero.Command), text.Copy)
	for index, action := range hero.Actions {
		class := "secondary"
		if index == 0 {
			class = "secondary first"
		}
		fmt.Fprintf(&written, "        <a class=%q href=%q>%s</a>\n",
			class, html.EscapeString(action.Href), html.EscapeString(action.Label))
	}
	written.WriteString("      </div>\n")

	if len(hero.Facts) > 0 {
		written.WriteString("      <p class=\"facts\">")
		for index, item := range hero.Facts {
			if index > 0 {
				written.WriteString("<span aria-hidden=\"true\"> · </span>")
			}
			written.WriteString(html.EscapeString(item))
		}
		written.WriteString("</p>\n")
	}

	if len(hero.Sides) == 2 {
		written.WriteString("      <figure class=\"proof\">\n")
		fmt.Fprintf(&written, "        <figcaption>%s</figcaption>\n", html.EscapeString(hero.Proof))
		written.WriteString("        <div class=\"sides\">\n")
		for index, side := range hero.Sides {
			class := "side"
			if index == 1 {
				class = "side ours"
			}
			fmt.Fprintf(&written, "          <div class=%q><p class=\"name\">%s</p>"+
				"<p class=\"number\">%s</p><p class=\"phrase\">%s</p></div>\n",
				class, html.EscapeString(side.Name), html.EscapeString(side.Number), html.EscapeString(side.Phrase))
		}
		written.WriteString("        </div>\n")
		fmt.Fprintf(&written, "        <p class=\"balance\">%s</p>\n", html.EscapeString(hero.Balance))
		written.WriteString("      </figure>\n")
	}
	written.WriteString("    </section>\n")
	return written.String()
}
