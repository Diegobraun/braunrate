package site

import (
	"encoding/json"
	"regexp"
	"strings"
)

// O indice e gerado na build e servido junto: buscar de graca com script de
// terceiro entregaria a quem lê a documentacao para outro servidor, e a regra
// de rede fechada vale para o site inteiro.
type entry struct {
	Page    string `json:"p"`
	Title   string `json:"t"`
	Section string `json:"s"`
	Anchor  string `json:"a"`
	Text    string `json:"x"`
}

func searchIndex(pages []Page) (string, error) {
	var entries []entry
	for position, page := range pages {
		file := fileOf(page, position)
		for _, piece := range split(page) {
			entries = append(entries, entry{
				Page: file, Title: page.Title, Section: piece.heading,
				Anchor: piece.anchor, Text: piece.text,
			})
		}
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return "window.SEARCH_INDEX=" + string(encoded) + "\n", nil
}

type piece struct {
	heading string
	anchor  string
	text    string
}

var (
	sectionBreak = regexp.MustCompile(`(?m)^(#{1,3}) +(.+?)\s*$`)
	tableRow     = regexp.MustCompile(`[|]+`)
	linkText     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	spaces       = regexp.MustCompile(`\s+`)
)

// Uma entrada por secao, e nao por pagina: o resultado precisa levar ao ponto
// da pagina, e um trecho de uma pagina de 900 linhas nao diz onde ele estava.
func split(page Page) []piece {
	positions := sectionBreak.FindAllStringSubmatchIndex(page.Markdown, -1)
	if len(positions) == 0 {
		return []piece{{heading: page.Title, text: clean(page.Markdown)}}
	}
	var pieces []piece
	for index, position := range positions {
		heading := strings.TrimSpace(page.Markdown[position[4]:position[5]])
		end := len(page.Markdown)
		if index+1 < len(positions) {
			end = positions[index+1][0]
		}
		text := clean(page.Markdown[position[1]:end])
		if text == "" {
			continue
		}
		anchor := ""
		if position[3]-position[2] > 1 {
			anchor = slugify(heading)
		}
		pieces = append(pieces, piece{heading: plain(heading), anchor: anchor, text: text})
	}
	return pieces
}

const maxCharactersPerPiece = 900

func clean(markdown string) string {
	text := strings.ReplaceAll(markdown, "```", " ")
	text = linkText.ReplaceAllString(text, "$1")
	text = tableRow.ReplaceAllString(text, " ")
	text = plain(text)
	text = spaces.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if len(text) > maxCharactersPerPiece {
		cut := strings.LastIndex(text[:maxCharactersPerPiece], " ")
		if cut < maxCharactersPerPiece/2 {
			cut = maxCharactersPerPiece
		}
		text = text[:cut]
	}
	return text
}
