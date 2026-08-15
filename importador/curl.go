package importador

import (
	"fmt"
	"net/url"
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

func montar(requisicao Requisicao) Importacao {
	roteiro := Roteiro{
		Nome: nomeDoCenario(requisicao),
		Alvo: requisicao.Alvo,
		Passos: []PassoImportado{{
			Nome:           nomeDoPasso(requisicao),
			Metodo:         requisicao.Metodo,
			Caminho:        requisicao.Caminho,
			Cabecalhos:     requisicao.Cabecalhos,
			Corpo:          requisicao.Corpo,
			SeguirRedirect: requisicao.SeguirRedirect,
		}},
	}

	if requisicao.Usuario != "" {
		roteiro.Avisos = append(roteiro.Avisos,
			fmt.Sprintf("o comando usava -u %s:...; declare isso no bloco 'autenticacao' com tipo: basica, e deixe a senha em variavel de ambiente", requisicao.Usuario))
	}
	if strings.Contains(requisicao.Caminho, "?") || temIdentificador(requisicao.Caminho) {
		roteiro.Avisos = append(roteiro.Avisos,
			"o caminho tem valor fixo: com um valor so, o alvo responde de cache e o numero fica otimista. Troque por ${dados.coluna} e declare um bloco 'dados'")
	}
	return GerarYAML(roteiro)
}

func nomeDoCenario(requisicao Requisicao) string {
	return "Importado de curl " + strings.ToUpper(requisicao.Metodo) + " " + recurso(requisicao.Caminho)
}

func nomeDoPasso(requisicao Requisicao) string {
	return strings.ToLower(requisicao.Metodo) + " " + recurso(requisicao.Caminho)
}
