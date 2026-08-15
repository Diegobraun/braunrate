package cenario

import (
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// A forma da expressao decide a origem: "$.campo" e JSON, "cabecalho:X-Id" e
// cabecalho, "/padrao/" e expressao regular. O QA escreve uma linha e pronto.
func lerCapturas(no *yaml.Node) ([]Captura, error) {
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "captura precisa ser um mapa, por exemplo: captura: { faturaId: $.fatura.id }")
	}
	capturas := make([]Captura, 0, len(no.Content)/2)
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		nome := no.Content[indice]
		valor := no.Content[indice+1]

		if valor.Kind == yaml.MappingNode {
			captura, err := lerCapturaCompleta(nome.Value, valor)
			if err != nil {
				return nil, err
			}
			capturas = append(capturas, captura)
			continue
		}

		captura, err := interpretarExpressaoDeCaptura(nome.Value, valor)
		if err != nil {
			return nil, err
		}
		capturas = append(capturas, captura)
	}
	return capturas, nil
}

func interpretarExpressaoDeCaptura(nome string, no *yaml.Node) (Captura, error) {
	expressao := strings.TrimSpace(no.Value)
	captura := Captura{Variavel: nome, Expressao: expressao, Obrigatoria: true, Linha: no.Line}

	switch {
	case expressao == "status":
		captura.Origem = CapturaDeStatus
	case expressao == "corpo":
		captura.Origem = CapturaDeCorpo
	case strings.HasPrefix(expressao, "$"):
		captura.Origem = CapturaDeJSON
	case strings.HasPrefix(expressao, "cabecalho:"):
		captura.Origem = CapturaDeCabecalho
		captura.Expressao = strings.TrimSpace(strings.TrimPrefix(expressao, "cabecalho:"))
	case strings.HasPrefix(expressao, "/") && strings.HasSuffix(expressao, "/") && len(expressao) > 2:
		captura.Origem = CapturaDeRegex
		captura.Expressao = expressao[1 : len(expressao)-1]
	default:
		return captura, erroNo(no, "nao entendi de onde capturar %q.\n"+
			"    use uma destas formas:\n"+
			"      %s: $.caminho.no.json      captura de um campo do corpo JSON\n"+
			"      %s: cabecalho:X-Request-Id captura de um cabecalho da resposta\n"+
			"      %s: /token=([a-z0-9]+)/    captura pelo primeiro grupo da expressao regular",
			nome, nome, nome, nome)
	}
	return captura, nil
}

func lerCapturaCompleta(nome string, no *yaml.Node) (Captura, error) {
	captura := Captura{Variavel: nome, Obrigatoria: true, Linha: no.Line}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "de":
			interpretada, err := interpretarExpressaoDeCaptura(nome, valor)
			if err != nil {
				return captura, err
			}
			captura.Origem = interpretada.Origem
			captura.Expressao = interpretada.Expressao
		case "padrao":
			captura.Padrao = valor.Value
			captura.Obrigatoria = false
		case "obrigatoria":
			captura.Obrigatoria = valor.Value != "false"
		default:
			return captura, erroNo(chave, "chave desconhecida na captura %q: %q (use de, padrao ou obrigatoria)", nome, chave.Value)
		}
	}
	if captura.Origem == "" {
		return captura, erroNo(no, "a captura %q precisa de 'de', por exemplo: de: $.fatura.id", nome)
	}
	return captura, nil
}

func lerAssercoes(no *yaml.Node) ([]Verificacao, []Assercao, error) {
	if no.Kind != yaml.MappingNode {
		return nil, nil, erroNo(no, "verificar precisa ser um mapa, por exemplo: verificar: { status: 200 }")
	}
	var verificacoes []Verificacao
	var assercoes []Assercao

	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "status":
			status, err := strconv.Atoi(strings.TrimSpace(valor.Value))
			if err != nil {
				return nil, nil, erroNo(valor, "status invalido: %q (use um numero, por exemplo 200)", valor.Value)
			}
			verificacoes = append(verificacoes, Verificacao{Tipo: VerificarStatus, Status: status})
		case "corpo_contem":
			assercoes = append(assercoes, Assercao{Tipo: AsserirCorpoContem, Valor: valor.Value, Linha: valor.Line})
		case "corpo_casa":
			assercoes = append(assercoes, Assercao{Tipo: AsserirRegex, Valor: valor.Value, Linha: valor.Line})
		case "json":
			if valor.Kind != yaml.MappingNode {
				return nil, nil, erroNo(valor, "json precisa ser um mapa, por exemplo: json: { $.status: PAGA }")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				assercao, err := interpretarComparacao(valor.Content[i].Value, valor.Content[i+1])
				if err != nil {
					return nil, nil, err
				}
				assercao.Tipo = AsserirJSON
				assercoes = append(assercoes, assercao)
			}
		case "cabecalho":
			if valor.Kind != yaml.MappingNode {
				return nil, nil, erroNo(valor, "cabecalho precisa ser um mapa, por exemplo: cabecalho: { Content-Type: application/json }")
			}
			for i := 0; i+1 < len(valor.Content); i += 2 {
				assercoes = append(assercoes, Assercao{
					Tipo: AsserirCabecalho, Alvo: valor.Content[i].Value,
					Operador: OperadorIgual, Valor: valor.Content[i+1].Value, Linha: valor.Content[i].Line,
				})
			}
		default:
			return nil, nil, erroNo(chave, "verificacao desconhecida: %q\n"+
				"    disponiveis: status, corpo_contem, corpo_casa, json, cabecalho", chave.Value)
		}
	}
	return verificacoes, assercoes, nil
}

func interpretarComparacao(alvo string, no *yaml.Node) (Assercao, error) {
	texto := strings.TrimSpace(no.Value)
	assercao := Assercao{Alvo: alvo, Operador: OperadorIgual, Valor: texto, Linha: no.Line}

	for _, operador := range []Operador{OperadorMenorOuIgual, OperadorMaiorOuIgual, OperadorDiferente,
		OperadorMenor, OperadorMaior} {
		if strings.HasPrefix(texto, string(operador)) {
			assercao.Operador = operador
			assercao.Valor = strings.TrimSpace(strings.TrimPrefix(texto, string(operador)))
			return assercao, nil
		}
	}
	switch texto {
	case "existe":
		assercao.Operador = OperadorExiste
		assercao.Valor = ""
	case "contem":
		assercao.Operador = OperadorContem
	}
	if strings.HasPrefix(texto, "contem ") {
		assercao.Operador = OperadorContem
		assercao.Valor = strings.TrimSpace(strings.TrimPrefix(texto, "contem "))
	}
	return assercao, nil
}

func lerAutenticacao(no *yaml.Node) (*Autenticacao, error) {
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "autenticacao precisa ser um mapa, por exemplo:\n"+
			"  autenticacao:\n"+
			"    tipo: token\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	autenticacao := &Autenticacao{Tipo: AutenticacaoPorToken, Linha: no.Line}

	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "tipo":
			switch valor.Value {
			case "token":
				autenticacao.Tipo = AutenticacaoPorToken
			case "basica":
				autenticacao.Tipo = AutenticacaoBasica
			case "cabecalho":
				autenticacao.Tipo = AutenticacaoCabecalho
			default:
				return nil, erroNo(valor, "tipo de autenticacao desconhecido: %q (use token, basica ou cabecalho)", valor.Value)
			}
		case "obter":
			passo, err := lerPasso(valor)
			if err != nil {
				return nil, err
			}
			passo.Nome = "obter autenticacao"
			autenticacao.Obter = &passo
		case "renovar_apos":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return nil, erroNo(valor, "renovar_apos invalido: %q (use por exemplo 25m)", valor.Value)
			}
			autenticacao.RenovarApos = duracao
		case "cabecalho":
			autenticacao.Cabecalho = valor.Value
		case "usuario":
			autenticacao.Usuario = valor.Value
		case "senha":
			autenticacao.Senha = valor.Value
		default:
			return nil, erroNo(chave, "chave desconhecida em autenticacao: %q\n"+
				"    disponiveis: tipo, obter, renovar_apos, cabecalho, usuario, senha", chave.Value)
		}
	}

	if autenticacao.Tipo == AutenticacaoPorToken && autenticacao.Obter == nil {
		return nil, erroNo(no, "autenticacao por token precisa do bloco 'obter' com a requisicao que devolve o token:\n"+
			"    obter:\n"+
			"      http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: \"${SENHA}\" } }\n"+
			"      captura: { token: $.access_token }")
	}
	if autenticacao.Tipo == AutenticacaoBasica && (autenticacao.Usuario == "" || autenticacao.Senha == "") {
		return nil, erroNo(no, "autenticacao basica precisa de usuario e senha, por exemplo:\n"+
			"  autenticacao: { tipo: basica, usuario: ana, senha: \"${SENHA}\" }")
	}
	if autenticacao.Cabecalho == "" && autenticacao.Tipo != AutenticacaoBasica {
		autenticacao.Cabecalho = "Authorization: Bearer ${token}"
	}
	return autenticacao, nil
}

func lerDados(no *yaml.Node) ([]FonteDeDados, error) {
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "dados precisa ser um mapa de fontes, por exemplo:\n"+
			"    dados:\n      assinantes:\n        arquivo: dados/assinantes.csv")
	}
	fontes := make([]FonteDeDados, 0, len(no.Content)/2)

	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		nome := no.Content[indice]
		corpo := no.Content[indice+1]
		if corpo.Kind != yaml.MappingNode {
			return nil, erroNo(corpo, "a fonte de dados %q precisa de um mapa com 'arquivo' ou 'gerar', por exemplo:\n"+
				"  %s: { arquivo: dados/assinantes.csv, consumo: circular }", nome.Value, nome.Value)
		}
		fonte := FonteDeDados{Nome: nome.Value, Consumo: ConsumoCircular, Linha: corpo.Line}

		for i := 0; i+1 < len(corpo.Content); i += 2 {
			chave := corpo.Content[i]
			valor := corpo.Content[i+1]
			switch chave.Value {
			case "arquivo":
				fonte.Arquivo = valor.Value
			case "consumo":
				switch valor.Value {
				case "sequencial":
					fonte.Consumo = ConsumoSequencial
				case "aleatorio":
					fonte.Consumo = ConsumoAleatorio
				case "circular":
					fonte.Consumo = ConsumoCircular
				case "unico_por_usuario":
					fonte.Consumo = ConsumoUnicoPorUsuario
				default:
					return nil, erroNo(valor, "consumo desconhecido: %q\n"+
						"    disponiveis: circular (padrao), sequencial, aleatorio, unico_por_usuario", valor.Value)
				}
			case "semente":
				semente, err := strconv.ParseInt(valor.Value, 10, 64)
				if err != nil {
					return nil, erroNo(valor, "semente invalida: %q (use um numero inteiro)", valor.Value)
				}
				fonte.Semente = semente
			case "gerar":
				if valor.Kind != yaml.MappingNode {
					return nil, erroNo(valor, "gerar precisa ser um mapa, por exemplo: gerar: { id: uuid, valor: numero(10,500) }")
				}
				fonte.Campos = map[string]string{}
				for j := 0; j+1 < len(valor.Content); j += 2 {
					fonte.Campos[valor.Content[j].Value] = valor.Content[j+1].Value
				}
			default:
				return nil, erroNo(chave, "chave desconhecida na fonte de dados %q: %q\n"+
					"    disponiveis: arquivo, gerar, consumo, semente", nome.Value, chave.Value)
			}
		}
		if fonte.Arquivo == "" && len(fonte.Campos) == 0 {
			return nil, erroNo(corpo, "a fonte de dados %q precisa de 'arquivo' (CSV) ou 'gerar' (dado sintetico)", nome.Value)
		}
		fontes = append(fontes, fonte)
	}
	return fontes, nil
}

func lerSLO(no *yaml.Node) ([]RegraDeSLO, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, erroNo(no, "slo precisa ser uma lista, por exemplo:\n"+
			"    slo:\n      - consultar pedido: { p95: < 150ms }\n      - global: { erros: < 0.1 }")
	}
	var regras []RegraDeSLO

	for _, item := range no.Content {
		if item.Kind != yaml.MappingNode || len(item.Content) < 2 {
			return nil, erroNo(item, "cada regra de slo e um mapa com o nome do passo (ou 'global') e os limites, por exemplo:\n"+
				"  - consultar pedido: { p95: < 150ms }")
		}
		alvo := item.Content[0]
		limites := item.Content[1]
		if limites.Kind != yaml.MappingNode {
			return nil, erroNo(limites, "os limites de %q precisam ser um mapa, por exemplo: { p95: < 150ms, erros: 0 %% }", alvo.Value)
		}

		for i := 0; i+1 < len(limites.Content); i += 2 {
			regra, err := lerRegraDeSLO(alvo.Value, limites.Content[i], limites.Content[i+1])
			if err != nil {
				return nil, err
			}
			regras = append(regras, regra)
		}
	}
	return regras, nil
}

func lerRegraDeSLO(alvo string, metricaNo, limiteNo *yaml.Node) (RegraDeSLO, error) {
	regra := RegraDeSLO{
		Passo:    alvo,
		Global:   alvo == "global",
		Metrica:  metricaNo.Value,
		Operador: OperadorMenorOuIgual,
		Linha:    metricaNo.Line,
	}

	switch regra.Metrica {
	case "p50", "p75", "p90", "p95", "p99", "p99.9", "max":
		regra.Unidade = "ms"
	case "erros":
		regra.Unidade = "%"
	case "vazao":
		regra.Unidade = "/s"
		regra.Operador = OperadorMaiorOuIgual
	default:
		return regra, erroNo(metricaNo, "metrica de slo desconhecida: %q\n"+
			"    disponiveis: p50, p75, p90, p95, p99, p99.9, max, erros, vazao", regra.Metrica)
	}

	texto := strings.TrimSpace(limiteNo.Value)
	regra.Texto = regra.Metrica + ": " + texto

	for _, operador := range []Operador{OperadorMenorOuIgual, OperadorMaiorOuIgual, OperadorMenor, OperadorMaior} {
		if strings.HasPrefix(texto, string(operador)) {
			regra.Operador = operador
			texto = strings.TrimSpace(strings.TrimPrefix(texto, string(operador)))
			break
		}
	}

	limite, err := interpretarLimite(texto, regra.Unidade)
	if err != nil {
		return regra, erroNo(limiteNo, "limite invalido em %q: %v\n"+
			"    exemplos: p95: < 150ms | erros: < 0.1 | vazao: > 500/s", regra.Metrica, err)
	}
	regra.Limite = limite
	return regra, nil
}

func interpretarLimite(texto, unidade string) (float64, error) {
	texto = strings.TrimSpace(texto)
	switch unidade {
	case "ms":
		duracao, err := time.ParseDuration(texto)
		if err != nil {
			valor, erroNumerico := strconv.ParseFloat(texto, 64)
			if erroNumerico != nil {
				return 0, err
			}
			return valor, nil
		}
		return float64(duracao.Microseconds()) / 1000, nil
	case "%":
		return strconv.ParseFloat(strings.TrimSuffix(texto, "%"), 64)
	default:
		return strconv.ParseFloat(strings.TrimSuffix(texto, "/s"), 64)
	}
}
