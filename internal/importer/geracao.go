package importer

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Todo importador termina aqui: curl, .jmx e o que vier depois montam um
// roteiro e a escrita do YAML acontece num lugar so. Duas escritas separadas
// significariam segredo mascarado num caminho e vazando no outro.
type PassoImportado struct {
	Nome           string
	Metodo         string
	Caminho        string
	Cabecalhos     map[string]string
	Corpo          string
	SeguirRedirect bool
}

type FonteImportada struct {
	Nome    string
	Arquivo string
	Consumo string
}

type Roteiro struct {
	Nome      string
	Alvo      string
	Dados     []FonteImportada
	Passos    []PassoImportado
	Perfis    []string
	Avisos    []string
	Descartes []string
}

// Cabecalho de credencial nunca sai no YAML: o arquivo gerado vai para o
// repositorio, e token commitado e o jeito mais comum de vazar um.
var cabecalhosDeSegredo = map[string]string{
	"authorization": "TOKEN",
	"x-api-key":     "API_KEY",
	"api-key":       "API_KEY",
	"cookie":        "COOKIE",
}

func GerarYAML(roteiro Roteiro) Importacao {
	importacao := Importacao{Avisos: append([]string{}, roteiro.Avisos...)}
	var linhas []string
	escrever := func(formato string, argumentos ...any) {
		linhas = append(linhas, fmt.Sprintf(formato, argumentos...))
	}

	passos := make([]PassoImportado, len(roteiro.Passos))
	copy(passos, roteiro.Passos)
	variaveis := map[string]string{}
	for indice := range passos {
		semSegredo := map[string]string{}
		for nome, valor := range passos[indice].Cabecalhos {
			variavel, segredo := cabecalhosDeSegredo[strings.ToLower(nome)]
			if !segredo {
				semSegredo[nome] = valor
				continue
			}
			local := strings.ToLower(variavel)
			semSegredo[nome] = prefixoDeCredencial(valor) + "${" + local + "}"
			if _, jaAvisado := variaveis[local]; !jaAvisado {
				variaveis[local] = variavel
				importacao.Avisos = append(importacao.Avisos,
					fmt.Sprintf("o cabecalho %s virou ${%s}: rode com %s=... no ambiente, para nao versionar credencial", nome, local, variavel))
			}
		}
		passos[indice].Cabecalhos = semSegredo
	}

	escrever("# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json")
	escrever("nome: %q", roteiro.Nome)
	escrever("alvo: ${ALVO:-%s}", roteiro.Alvo)
	escrever("")

	if len(variaveis) > 0 {
		escrever("variaveis:")
		var declaracoes []string
		for local, ambiente := range variaveis {
			declaracoes = append(declaracoes, fmt.Sprintf("  %s: ${%s}", local, ambiente))
		}
		sort.Strings(declaracoes)
		linhas = append(linhas, declaracoes...)
		escrever("")
	}

	if len(roteiro.Dados) > 0 {
		escrever("dados:")
		for _, fonte := range roteiro.Dados {
			consumo := fonte.Consumo
			if consumo == "" {
				consumo = "circular"
			}
			escrever("  %s: { arquivo: %s, consumo: %s }", fonte.Nome, fonte.Arquivo, consumo)
		}
		escrever("")
	}

	escrever("carga:")
	escrever("  perfis:")
	perfis := roteiro.Perfis
	if len(perfis) == 0 {
		perfis = []string{
			"    - rampa: { de: 1/s, ate: 20/s, durante: 30s }",
			"    - patamar: { taxa: 20/s, durante: 1m }",
		}
	}
	linhas = append(linhas, perfis...)
	escrever("")
	escrever("cenario:")

	for _, passo := range passos {
		simples := passo.Corpo == "" && len(passo.Cabecalhos) == 0 && !passo.SeguirRedirect
		if simples {
			escrever("  - http: %s %s", passo.Metodo, passo.Caminho)
			escrever("    nome: %s", passo.Nome)
		} else {
			escrever("  - nome: %s", passo.Nome)
			escrever("    http:")
			escrever("      metodo: %s", passo.Metodo)
			escrever("      caminho: %s", passo.Caminho)
			if len(passo.Cabecalhos) > 0 {
				escrever("      cabecalhos:")
				nomes := make([]string, 0, len(passo.Cabecalhos))
				for nome := range passo.Cabecalhos {
					nomes = append(nomes, nome)
				}
				sort.Strings(nomes)
				for _, nome := range nomes {
					escrever("        %s: %q", nome, passo.Cabecalhos[nome])
				}
			}
			if passo.Corpo != "" {
				escrever("      corpo: %s", emLinha(passo.Corpo))
			}
			if passo.SeguirRedirect {
				escrever("      seguir_redirect: true")
			}
		}
		escrever("    verificar:")
		escrever("      status: 200")
	}

	escrever("")
	escrever("slo:")
	for _, passo := range passos {
		escrever("  - %s: { p95: < 500ms }", passo.Nome)
	}
	escrever("  - global: { erros: < 1 }")

	importacao.Avisos = append(importacao.Avisos,
		"os numeros de carga e de slo sao um chute de partida, nao uma medicao: ajuste antes de usar como gate")
	importacao.YAML = strings.Join(linhas, "\n") + "\n"
	return importacao
}

func prefixoDeCredencial(valor string) string {
	if partes := strings.SplitN(valor, " ", 2); len(partes) == 2 && !strings.Contains(partes[0], "=") {
		return partes[0] + " "
	}
	return ""
}

func emLinha(corpo string) string {
	limpo := strings.TrimSpace(strings.ReplaceAll(corpo, "\n", " "))
	return "'" + strings.ReplaceAll(limpo, "'", "''") + "'"
}

// Identificador e prefixo de versao nao entram no nome do passo: o nome e a
// chave de agregacao do relatorio, e um nome por id daria uma linha por
// requisicao em vez de uma linha por operacao.
func recurso(caminho string) string {
	semConsulta, _, _ := strings.Cut(caminho, "?")
	var partes []string
	for _, parte := range strings.Split(semConsulta, "/") {
		if parte == "" || pareceIdentificador(parte) || pareceVersao(parte) {
			continue
		}
		partes = append(partes, parte)
	}
	if len(partes) == 0 {
		return "raiz"
	}
	return strings.Join(partes, " ")
}

func pareceVersao(parte string) bool {
	if len(parte) < 2 || (parte[0] != 'v' && parte[0] != 'V') {
		return false
	}
	for _, caractere := range parte[1:] {
		if !unicode.IsDigit(caractere) {
			return false
		}
	}
	return true
}

func pareceIdentificador(parte string) bool {
	digitos := 0
	for _, caractere := range parte {
		if unicode.IsDigit(caractere) {
			digitos++
		}
	}
	return digitos >= 3 || len(parte) >= 16
}

func temIdentificador(caminho string) bool {
	semConsulta, _, _ := strings.Cut(caminho, "?")
	for _, parte := range strings.Split(semConsulta, "/") {
		if parte != "" && pareceIdentificador(parte) {
			return true
		}
	}
	return false
}
