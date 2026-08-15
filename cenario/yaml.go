package cenario

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"gopkg.in/yaml.v3"
)

type ErroDeCenario struct {
	Arquivo  string
	Linha    int
	Coluna   int
	Mensagem string
}

func (e ErroDeCenario) Error() string {
	if e.Arquivo == "" {
		return fmt.Sprintf("linha %d: %s", e.Linha, e.Mensagem)
	}
	return fmt.Sprintf("%s:%d:%d: %s", e.Arquivo, e.Linha, e.Coluna, e.Mensagem)
}

func erroNo(no *yaml.Node, formato string, argumentos ...any) error {
	linha, coluna := 0, 0
	if no != nil {
		linha, coluna = no.Line, no.Column
	}
	return ErroDeCenario{Linha: linha, Coluna: coluna, Mensagem: fmt.Sprintf(formato, argumentos...)}
}

// Listadas aqui porque o schema publicado e testado contra elas: chave que
// existe so num dos dois lados vira autocompletar que o parser recusa.
var (
	ChavesDeTopo  = []string{"nome", "alvo", "variaveis", "autenticacao", "dados", "carga", "cenario", "slo"}
	ChavesDePasso = []string{"nome", "captura", "verificar", "espera"}
)

func CarregarArquivo(caminho string) (Cenario, error) {
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		return Cenario{}, err
	}
	c, err := Carregar(conteudo)
	if erro, ok := err.(ErroDeCenario); ok {
		erro.Arquivo = caminho
		return c, erro
	}
	return c, err
}

func Carregar(conteudo []byte) (Cenario, error) {
	var raiz yaml.Node
	if err := yaml.Unmarshal(conteudo, &raiz); err != nil {
		return Cenario{}, err
	}
	if len(raiz.Content) == 0 {
		return Cenario{}, ErroDeCenario{Linha: 1, Mensagem: "cenario vazio"}
	}
	documento := raiz.Content[0]
	if documento.Kind != yaml.MappingNode {
		return Cenario{}, erroNo(documento, "o cenario precisa ser um mapa de chaves, comecando por:\n"+
			"  nome: Consulta de pedidos\n"+
			"  alvo: http://127.0.0.1:8080")
	}

	c := Cenario{
		VersaoDoFormato: VersaoDoFormato,
		Variaveis:       map[string]string{},
		Carga:           PlanoDeCarga{Modelo: ChegadaAberta},
	}

	for indice := 0; indice+1 < len(documento.Content); indice += 2 {
		chave := documento.Content[indice]
		valor := documento.Content[indice+1]
		switch chave.Value {
		case "nome":
			c.Nome = valor.Value
		case "alvo":
			c.Alvo = valor.Value
		case "variaveis":
			variaveis, err := lerVariaveis(valor)
			if err != nil {
				return c, err
			}
			c.Variaveis = variaveis
		case "carga":
			carga, err := lerCarga(valor)
			if err != nil {
				return c, err
			}
			c.Carga = carga
		case "cenario":
			passos, err := lerPassos(valor)
			if err != nil {
				return c, err
			}
			c.Passos = passos
		case "autenticacao":
			autenticacao, err := lerAutenticacao(valor)
			if err != nil {
				return c, err
			}
			c.Autenticacao = autenticacao
		case "dados":
			fontes, err := lerDados(valor)
			if err != nil {
				return c, err
			}
			c.Dados = fontes
		case "slo":
			regras, err := lerSLO(valor)
			if err != nil {
				return c, err
			}
			c.SLO = regras
		default:
			return c, erroNo(chave, "chave desconhecida no topo do cenario: %q\n%s",
				chave.Value, sugerir(chave.Value, ChavesDeTopo))
		}
	}

	c.Alvo = interpolar(c.Alvo, c.Variaveis)
	return c, nil
}

func lerVariaveis(no *yaml.Node) (map[string]string, error) {
	variaveis := map[string]string{}
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "variaveis precisa ser um mapa, por exemplo:\n"+
			"  variaveis:\n"+
			"    usuario: ${USUARIO:-ana}")
	}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		nome := no.Content[indice].Value
		variaveis[nome] = expandirDoAmbiente(no.Content[indice+1].Value)
	}
	return variaveis, nil
}

var padraoDeVariavel = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.]*)(?::-([^}]*))?\}`)

func expandirDoAmbiente(texto string) string {
	return padraoDeVariavel.ReplaceAllStringFunc(texto, func(ocorrencia string) string {
		partes := padraoDeVariavel.FindStringSubmatch(ocorrencia)
		if valor, definida := os.LookupEnv(partes[1]); definida {
			return valor
		}
		return partes[2]
	})
}

func interpolar(texto string, variaveis map[string]string) string {
	if texto == "" {
		return texto
	}
	return padraoDeVariavel.ReplaceAllStringFunc(texto, func(ocorrencia string) string {
		partes := padraoDeVariavel.FindStringSubmatch(ocorrencia)
		if valor, existe := variaveis[partes[1]]; existe {
			return valor
		}
		if valor, definida := os.LookupEnv(partes[1]); definida {
			return valor
		}
		return partes[2]
	})
}

func lerCarga(no *yaml.Node) (PlanoDeCarga, error) {
	plano := PlanoDeCarga{Modelo: ChegadaAberta}
	if no.Kind != yaml.MappingNode {
		return plano, erroNo(no, "carga precisa ser um mapa, por exemplo:\n"+
			"  carga:\n"+
			"    perfis:\n"+
			"      - patamar: { taxa: 300/s, durante: 1m }")
	}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "modelo":
			switch valor.Value {
			case string(ChegadaAberta):
				plano.Modelo = ChegadaAberta
			case string(ChegadaFechada):
				return plano, erroNo(valor, "modelo fechado ainda nao e suportado; o padrao e aberto")
			default:
				return plano, erroNo(valor, "modelo de carga desconhecido: %q (o unico modelo e 'aberto', e ele e o padrao: pode omitir a linha)", valor.Value)
			}
		case "perfis":
			if valor.Kind != yaml.SequenceNode {
				return plano, erroNo(valor, "perfis precisa ser uma lista, um trecho por linha:\n"+
					"  perfis:\n"+
					"    - rampa: { de: 50/s, ate: 300/s, durante: 30s }\n"+
					"    - patamar: { taxa: 300/s, durante: 5m }")
			}
			for _, itemNo := range valor.Content {
				fase, err := lerFase(itemNo)
				if err != nil {
					return plano, err
				}
				plano.Fases = append(plano.Fases, fase)
			}
		default:
			return plano, erroNo(chave, "chave desconhecida em carga: %q\n%s", chave.Value, sugerir(chave.Value, []string{"modelo", "perfis"}))
		}
	}
	return plano, nil
}

func lerFase(no *yaml.Node) (Fase, error) {
	if no.Kind != yaml.MappingNode || len(no.Content) < 2 {
		return Fase{}, erroNo(no, "cada perfil precisa ser um mapa com um tipo (rampa, patamar, pico, constante), por exemplo:\n"+
			"  - patamar: { taxa: 300/s, durante: 5m }")
	}
	tipoNo := no.Content[0]
	corpo := no.Content[1]
	fase := Fase{Linha: no.Line}

	switch tipoNo.Value {
	case "rampa":
		fase.Tipo = FaseRampa
	case "patamar":
		fase.Tipo = FasePatamar
	case "pico":
		fase.Tipo = FasePico
	case "constante":
		fase.Tipo = FaseConstante
	default:
		return fase, erroNo(tipoNo, "tipo de perfil desconhecido: %q\n%s\nexemplo: - patamar: { taxa: 300/s, durante: 5m }",
			tipoNo.Value, sugerir(tipoNo.Value, []string{"rampa", "patamar", "pico", "constante"}))
	}

	if corpo.Kind != yaml.MappingNode {
		return fase, erroNo(corpo, "o perfil %q precisa de um mapa de parametros, por exemplo: %s: { taxa: 300/s, durante: 5m }", tipoNo.Value, tipoNo.Value)
	}
	for indice := 0; indice+1 < len(corpo.Content); indice += 2 {
		chave := corpo.Content[indice]
		valor := corpo.Content[indice+1]
		switch chave.Value {
		case "de":
			taxa, err := lerTaxa(valor)
			if err != nil {
				return fase, err
			}
			fase.De = taxa
		case "ate", "taxa":
			taxa, err := lerTaxa(valor)
			if err != nil {
				return fase, err
			}
			fase.Ate = taxa
		case "durante":
			duracao, err := time.ParseDuration(valor.Value)
			if err != nil {
				return fase, erroNo(valor, "duracao invalida: %q (use 30s, 5m, 1h30m)", valor.Value)
			}
			fase.Durante = duracao
		default:
			return fase, erroNo(chave, "chave desconhecida no perfil %q: %q\n%s", tipoNo.Value, chave.Value,
				sugerir(chave.Value, []string{"de", "ate", "taxa", "durante"}))
		}
	}
	if fase.Tipo == FaseRampa && fase.De == 0 && fase.Ate == 0 {
		return fase, erroNo(corpo, "rampa precisa de 'de' e 'ate', por exemplo: - rampa: { de: 50/s, ate: 300/s, durante: 30s }")
	}
	return fase, nil
}

func lerTaxa(no *yaml.Node) (float64, error) {
	texto := strings.TrimSpace(no.Value)
	divisor := 1.0
	switch {
	case strings.HasSuffix(texto, "/s"):
		texto = strings.TrimSuffix(texto, "/s")
	case strings.HasSuffix(texto, "/m"):
		texto = strings.TrimSuffix(texto, "/m")
		divisor = 60
	case strings.HasSuffix(texto, "/h"):
		texto = strings.TrimSuffix(texto, "/h")
		divisor = 3600
	}
	valor, err := strconv.ParseFloat(strings.TrimSpace(texto), 64)
	if err != nil {
		return 0, erroNo(no, "taxa invalida: %q (use por exemplo 50/s)", no.Value)
	}
	if valor <= 0 {
		return 0, erroNo(no, "taxa precisa ser maior que zero")
	}
	return valor / divisor, nil
}

func lerPassos(no *yaml.Node) ([]Passo, error) {
	if no.Kind != yaml.SequenceNode {
		return nil, erroNo(no, "cenario precisa ser uma lista de passos, um por linha:\n"+
			"  cenario:\n"+
			"    - http: GET /pedidos/1")
	}
	passos := make([]Passo, 0, len(no.Content))
	for _, itemNo := range no.Content {
		passo, err := lerPasso(itemNo)
		if err != nil {
			return nil, err
		}
		passos = append(passos, passo)
	}
	return passos, nil
}

func lerPasso(no *yaml.Node) (Passo, error) {
	passo := Passo{Linha: no.Line}
	if no.Kind != yaml.MappingNode {
		return passo, erroNo(no, "cada passo precisa ser um mapa, por exemplo:\n"+
			"  - http: GET /pedidos/1\n"+
			"    nome: consultar pedido")
	}
	var configuracaoNo *yaml.Node
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "nome":
			passo.Nome = valor.Value
		case "verificar", "espera":
			verificacoes, assercoes, err := lerAssercoes(valor)
			if err != nil {
				return passo, err
			}
			passo.Verificacoes = verificacoes
			passo.Assercoes = assercoes
		case "captura":
			capturas, err := lerCapturas(valor)
			if err != nil {
				return passo, err
			}
			passo.Capturas = capturas
		case "peso":
			return passo, erroNo(chave, "a chave %q ainda nao existe: mix ponderado de operacoes entra junto com o GraphQL", chave.Value)
		default:
			if _, existe := protocolo.Buscar(chave.Value); !existe {
				return passo, erroNo(chave, "nao reconheco %q como tipo de passo\n%s",
					chave.Value, sugerir(chave.Value, append(protocolo.Registrados(), ChavesDePasso...)))
			}
			if passo.Protocolo != "" {
				return passo, erroNo(chave, "o passo declara mais de um protocolo: %q e %q", passo.Protocolo, chave.Value)
			}
			passo.Protocolo = chave.Value
			configuracaoNo = valor
		}
	}
	if passo.Protocolo == "" {
		return passo, erroNo(no, "passo sem protocolo (compilados: %s), por exemplo:\n"+
			"  - http: GET /pedidos/1", strings.Join(protocolo.Registrados(), ", "))
	}
	implementacao, _ := protocolo.Buscar(passo.Protocolo)
	configuracao, err := implementacao.Decodificar(configuracaoNo)
	if err != nil {
		if _, jaEhErroDeCenario := err.(ErroDeCenario); !jaEhErroDeCenario {
			return passo, erroNo(configuracaoNo, "%v", err)
		}
		return passo, err
	}
	passo.Configuracao = configuracao
	if passo.Nome == "" {
		passo.Nome = configuracao.ChaveDeAgregacao()
	}
	return passo, nil
}

func sugerir(recebido string, validas []string) string {
	melhor, menorDistancia := "", 1<<30
	for _, valida := range validas {
		distancia := distanciaDeEdicao(strings.ToLower(recebido), strings.ToLower(valida))
		if distancia < menorDistancia {
			melhor, menorDistancia = valida, distancia
		}
	}
	linhas := ""
	if melhor != "" && menorDistancia <= 3 {
		linhas += fmt.Sprintf("    voce quis dizer %q?\n", melhor)
	}
	return linhas + "    disponiveis: " + strings.Join(validas, ", ")
}

func distanciaDeEdicao(primeira, segunda string) int {
	anterior := make([]int, len(segunda)+1)
	atual := make([]int, len(segunda)+1)
	for j := range anterior {
		anterior[j] = j
	}
	for i := 1; i <= len(primeira); i++ {
		atual[0] = i
		for j := 1; j <= len(segunda); j++ {
			custo := 1
			if primeira[i-1] == segunda[j-1] {
				custo = 0
			}
			atual[j] = min(min(atual[j-1]+1, anterior[j]+1), anterior[j-1]+custo)
		}
		copy(anterior, atual)
	}
	return anterior[len(segunda)]
}
