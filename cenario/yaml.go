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
		return Cenario{}, erroNo(documento, "o cenario precisa ser um mapa de chaves")
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
		case "autenticacao", "dados", "slo":
			return c, erroNo(chave, "a chave %q ainda nao e suportada nesta versao", chave.Value)
		default:
			return c, erroNo(chave, "chave desconhecida no topo do cenario: %q", chave.Value)
		}
	}

	c.Alvo = interpolar(c.Alvo, c.Variaveis)
	for indice := range c.Passos {
		if configuravel, ok := c.Passos[indice].Configuracao.(interface {
			Interpolar(func(string) string)
		}); ok {
			configuravel.Interpolar(func(texto string) string { return interpolar(texto, c.Variaveis) })
		}
	}
	return c, nil
}

func lerVariaveis(no *yaml.Node) (map[string]string, error) {
	variaveis := map[string]string{}
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "variaveis precisa ser um mapa")
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
		return plano, erroNo(no, "carga precisa ser um mapa")
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
				return plano, erroNo(valor, "modelo de carga desconhecido: %q", valor.Value)
			}
		case "perfis":
			if valor.Kind != yaml.SequenceNode {
				return plano, erroNo(valor, "perfis precisa ser uma lista")
			}
			for _, itemNo := range valor.Content {
				fase, err := lerFase(itemNo)
				if err != nil {
					return plano, err
				}
				plano.Fases = append(plano.Fases, fase)
			}
		default:
			return plano, erroNo(chave, "chave desconhecida em carga: %q", chave.Value)
		}
	}
	return plano, nil
}

func lerFase(no *yaml.Node) (Fase, error) {
	if no.Kind != yaml.MappingNode || len(no.Content) < 2 {
		return Fase{}, erroNo(no, "cada perfil precisa ser um mapa com um tipo (rampa, patamar, pico, constante)")
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
		return fase, erroNo(tipoNo, "tipo de perfil desconhecido: %q (use rampa, patamar, pico ou constante)", tipoNo.Value)
	}

	if corpo.Kind != yaml.MappingNode {
		return fase, erroNo(corpo, "o perfil %q precisa de um mapa de parametros", tipoNo.Value)
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
				return fase, erroNo(valor, "duracao invalida em %q: %v", valor.Value, err)
			}
			fase.Durante = duracao
		default:
			return fase, erroNo(chave, "chave desconhecida no perfil %q: %q", tipoNo.Value, chave.Value)
		}
	}
	if fase.Tipo == FaseRampa && fase.De == 0 && fase.Ate == 0 {
		return fase, erroNo(corpo, "rampa precisa de 'de' e 'ate'")
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
		return nil, erroNo(no, "cenario precisa ser uma lista de passos")
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
		return passo, erroNo(no, "cada passo precisa ser um mapa")
	}
	var configuracaoNo *yaml.Node
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "nome":
			passo.Nome = valor.Value
		case "verificar", "espera":
			verificacoes, err := lerVerificacoes(valor)
			if err != nil {
				return passo, err
			}
			passo.Verificacoes = verificacoes
		case "captura", "peso":
			return passo, erroNo(chave, "a chave %q ainda nao e suportada nesta versao", chave.Value)
		default:
			if _, existe := protocolo.Buscar(chave.Value); !existe {
				return passo, erroNo(chave, "protocolo desconhecido: %q (compilados: %s)",
					chave.Value, strings.Join(protocolo.Registrados(), ", "))
			}
			if passo.Protocolo != "" {
				return passo, erroNo(chave, "o passo declara mais de um protocolo: %q e %q", passo.Protocolo, chave.Value)
			}
			passo.Protocolo = chave.Value
			configuracaoNo = valor
		}
	}
	if passo.Protocolo == "" {
		return passo, erroNo(no, "passo sem protocolo (compilados: %s)", strings.Join(protocolo.Registrados(), ", "))
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

func lerVerificacoes(no *yaml.Node) ([]Verificacao, error) {
	if no.Kind != yaml.MappingNode {
		return nil, erroNo(no, "verificar precisa ser um mapa")
	}
	verificacoes := []Verificacao{}
	for indice := 0; indice+1 < len(no.Content); indice += 2 {
		chave := no.Content[indice]
		valor := no.Content[indice+1]
		switch chave.Value {
		case "status":
			status, err := strconv.Atoi(valor.Value)
			if err != nil {
				return nil, erroNo(valor, "status invalido: %q", valor.Value)
			}
			verificacoes = append(verificacoes, Verificacao{Tipo: VerificarStatus, Status: status})
		case "corpo_contem":
			verificacoes = append(verificacoes, Verificacao{Tipo: VerificarCorpo, Texto: valor.Value})
		default:
			return nil, erroNo(chave, "verificacao desconhecida: %q (use status ou corpo_contem)", chave.Value)
		}
	}
	return verificacoes, nil
}
