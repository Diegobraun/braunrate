package importador

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

type Requisicao struct {
	Metodo         string
	Alvo           string
	Caminho        string
	Cabecalhos     map[string]string
	Corpo          string
	SeguirRedirect bool
	Usuario        string
	Senha          string
}

type Importacao struct {
	YAML   string
	Avisos []string
}

func DeCurl(comando string) (Importacao, error) {
	campos, err := separar(comando)
	if err != nil {
		return Importacao{}, err
	}
	requisicao, err := interpretar(campos)
	if err != nil {
		return Importacao{}, err
	}
	return montar(requisicao), nil
}

// Aspas e barra invertida de continuacao de linha vem coladas no que a pessoa
// copiou do navegador; separar por espaco simples quebraria todo corpo JSON.
func separar(comando string) ([]string, error) {
	var campos []string
	var atual strings.Builder
	tem := false
	aspas := rune(0)

	runas := []rune(comando)
	for indice := 0; indice < len(runas); indice++ {
		caractere := runas[indice]
		switch {
		case caractere == '\\' && aspas != '\'':
			if indice+1 < len(runas) {
				seguinte := runas[indice+1]
				if seguinte == '\n' {
					indice++
					continue
				}
				if aspas == 0 {
					atual.WriteRune(seguinte)
					tem = true
					indice++
					continue
				}
				if seguinte == '"' || seguinte == '\\' {
					atual.WriteRune(seguinte)
					tem = true
					indice++
					continue
				}
			}
			atual.WriteRune(caractere)
			tem = true
		case aspas != 0:
			if caractere == aspas {
				aspas = 0
				continue
			}
			atual.WriteRune(caractere)
			tem = true
		case caractere == '\'' || caractere == '"':
			aspas = caractere
			tem = true
		case unicode.IsSpace(caractere):
			if tem {
				campos = append(campos, atual.String())
				atual.Reset()
				tem = false
			}
		default:
			atual.WriteRune(caractere)
			tem = true
		}
	}
	if aspas != 0 {
		return nil, fmt.Errorf("o comando tem aspas abertas que nunca fecham; cole o curl inteiro, inclusive a ultima linha")
	}
	if tem {
		campos = append(campos, atual.String())
	}
	if len(campos) == 0 {
		return nil, fmt.Errorf("nao recebi nenhum comando; use:\n  braunrate importar curl \"curl -X POST https://exemplo/pedidos -d '{}'\"\nou passe o comando pela entrada padrao")
	}
	return campos, nil
}

func interpretar(campos []string) (Requisicao, error) {
	requisicao := Requisicao{Cabecalhos: map[string]string{}}
	if campos[0] == "curl" {
		campos = campos[1:]
	}

	proximo := func(indice *int, bandeira string) (string, error) {
		if *indice+1 >= len(campos) {
			return "", fmt.Errorf("a opcao %s ficou sem valor no fim do comando", bandeira)
		}
		*indice++
		return campos[*indice], nil
	}

	var endereco string
	for indice := 0; indice < len(campos); indice++ {
		campo := campos[indice]
		nome, valorColado, colado := strings.Cut(campo, "=")

		switch {
		case campo == "-X" || campo == "--request":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			requisicao.Metodo = strings.ToUpper(valor)
		case campo == "-H" || campo == "--header":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			chave, conteudo, tem := strings.Cut(valor, ":")
			if !tem {
				return requisicao, fmt.Errorf("o cabecalho %q nao tem dois-pontos; a forma e -H \"Nome: valor\"", valor)
			}
			requisicao.Cabecalhos[strings.TrimSpace(chave)] = strings.TrimSpace(conteudo)
		case campo == "-d" || campo == "--data" || campo == "--data-raw" || campo == "--data-binary" || campo == "--data-ascii":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			requisicao.Corpo = valor
		case colado && (nome == "--data" || nome == "--data-raw" || nome == "--data-binary"):
			requisicao.Corpo = valorColado
		case campo == "-u" || campo == "--user":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			requisicao.Usuario, requisicao.Senha, _ = strings.Cut(valor, ":")
		case campo == "--url":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			endereco = valor
		case campo == "-L" || campo == "--location":
			requisicao.SeguirRedirect = true
		case campo == "-o" || campo == "--output" || campo == "-w" || campo == "--write-out" || campo == "-A" || campo == "--user-agent" || campo == "-b" || campo == "--cookie" || campo == "-e" || campo == "--referer":
			valor, err := proximo(&indice, campo)
			if err != nil {
				return requisicao, err
			}
			if campo == "-A" || campo == "--user-agent" {
				requisicao.Cabecalhos["User-Agent"] = valor
			}
			if campo == "-b" || campo == "--cookie" {
				requisicao.Cabecalhos["Cookie"] = valor
			}
		case strings.HasPrefix(campo, "-"):
			continue
		default:
			if endereco == "" {
				endereco = campo
			}
		}
	}

	if endereco == "" {
		return requisicao, fmt.Errorf("nao achei a URL no comando; o curl precisa ter o endereco, como em:\n  curl https://exemplo/pedidos")
	}
	if !strings.Contains(endereco, "://") {
		endereco = "https://" + endereco
	}
	partes, err := url.Parse(endereco)
	if err != nil {
		return requisicao, fmt.Errorf("nao consegui entender a URL %q: %v", endereco, err)
	}

	requisicao.Alvo = partes.Scheme + "://" + partes.Host
	requisicao.Caminho = partes.RequestURI()
	if requisicao.Metodo == "" {
		requisicao.Metodo = "GET"
		if requisicao.Corpo != "" {
			requisicao.Metodo = "POST"
		}
	}
	return requisicao, nil
}

// Cabecalho de credencial nunca sai no YAML: o arquivo gerado vai para o
// repositorio, e token commitado e o jeito mais comum de vazar um.
var cabecalhosDeSegredo = map[string]string{
	"authorization": "TOKEN",
	"x-api-key":     "API_KEY",
	"api-key":       "API_KEY",
	"cookie":        "COOKIE",
}

func montar(requisicao Requisicao) Importacao {
	importacao := Importacao{}
	var linhas []string

	escrever := func(formato string, argumentos ...any) {
		linhas = append(linhas, fmt.Sprintf(formato, argumentos...))
	}

	escrever("# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json")
	escrever("nome: %q", nomeDoCenario(requisicao))
	escrever("alvo: ${ALVO:-%s}", requisicao.Alvo)
	escrever("")
	semSegredo := map[string]string{}
	var variaveis []string
	for nome, valor := range requisicao.Cabecalhos {
		variavel, segredo := cabecalhosDeSegredo[strings.ToLower(nome)]
		if !segredo {
			semSegredo[nome] = valor
			continue
		}
		prefixo := ""
		if partes := strings.SplitN(valor, " ", 2); len(partes) == 2 && !strings.Contains(partes[0], "=") {
			prefixo = partes[0] + " "
		}
		local := strings.ToLower(variavel)
		semSegredo[nome] = prefixo + "${" + local + "}"
		variaveis = append(variaveis, fmt.Sprintf("  %s: ${%s}", local, variavel))
		importacao.Avisos = append(importacao.Avisos,
			fmt.Sprintf("o cabecalho %s virou ${%s}: rode com %s=... no ambiente, para nao versionar credencial", nome, local, variavel))
	}

	if len(variaveis) > 0 {
		escrever("variaveis:")
		sort.Strings(variaveis)
		linhas = append(linhas, variaveis...)
		escrever("")
	}
	escrever("carga:")
	escrever("  perfis:")
	escrever("    - rampa: { de: 1/s, ate: 20/s, durante: 30s }")
	escrever("    - patamar: { taxa: 20/s, durante: 1m }")
	escrever("")
	escrever("cenario:")

	corpoSimples := requisicao.Corpo == "" && len(semSegredo) == 0 && !requisicao.SeguirRedirect
	if corpoSimples {
		escrever("  - http: %s %s", requisicao.Metodo, requisicao.Caminho)
		escrever("    nome: %s", nomeDoPasso(requisicao))
	} else {
		escrever("  - nome: %s", nomeDoPasso(requisicao))
		escrever("    http:")
		escrever("      metodo: %s", requisicao.Metodo)
		escrever("      caminho: %s", requisicao.Caminho)
		if len(semSegredo) > 0 {
			escrever("      cabecalhos:")
			nomes := make([]string, 0, len(semSegredo))
			for nome := range semSegredo {
				nomes = append(nomes, nome)
			}
			sort.Strings(nomes)
			for _, nome := range nomes {
				escrever("        %s: %q", nome, semSegredo[nome])
			}
		}
		if requisicao.Corpo != "" {
			escrever("      corpo: %s", emLinha(requisicao.Corpo))
		}
		if requisicao.SeguirRedirect {
			escrever("      seguir_redirect: true")
		}
	}
	escrever("    verificar:")
	escrever("      status: 200")
	escrever("")
	escrever("slo:")
	escrever("  - %s: { p95: < 500ms }", nomeDoPasso(requisicao))
	escrever("  - global: { erros: < 1 }")

	if requisicao.Usuario != "" {
		importacao.Avisos = append(importacao.Avisos,
			fmt.Sprintf("o comando usava -u %s:...; declare isso no bloco 'autenticacao' com tipo: basica, e deixe a senha em variavel de ambiente", requisicao.Usuario))
	}
	if strings.Contains(requisicao.Caminho, "?") || temIdentificador(requisicao.Caminho) {
		importacao.Avisos = append(importacao.Avisos,
			"o caminho tem valor fixo: com um valor so, o alvo responde de cache e o numero fica otimista. Troque por ${dados.coluna} e declare um bloco 'dados'")
	}
	importacao.Avisos = append(importacao.Avisos,
		"os numeros de carga e de slo sao um chute de partida, nao uma medicao: ajuste antes de usar como gate")

	importacao.YAML = strings.Join(linhas, "\n") + "\n"
	return importacao
}

func emLinha(corpo string) string {
	limpo := strings.TrimSpace(strings.ReplaceAll(corpo, "\n", " "))
	return "'" + strings.ReplaceAll(limpo, "'", "''") + "'"
}

func nomeDoCenario(requisicao Requisicao) string {
	return "Importado de curl " + strings.ToUpper(requisicao.Metodo) + " " + recurso(requisicao.Caminho)
}

func nomeDoPasso(requisicao Requisicao) string {
	return strings.ToLower(requisicao.Metodo) + " " + recurso(requisicao.Caminho)
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
