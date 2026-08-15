package metrica

import (
	"fmt"
	"sort"
	"strings"
)

// Teto de valores distintos guardados por variavel. O que importa e distinguir
// "um valor so" de "muitos"; contar exatamente um milhao de valores custaria
// memoria proporcional a carga sem mudar nenhuma conclusao.
const tetoDeValoresDistintos = 1024

type Variedade struct {
	Nome       string `json:"nome"`
	Distintos  int64  `json:"valores_distintos"`
	Usos       int64  `json:"usos"`
	Disponivel int64  `json:"valores_disponiveis"`
	Limitado   bool   `json:"limitado_pelo_teto"`
	Frase      string `json:"frase"`
}

type contadorDeVariedade struct {
	vistos   map[string]struct{}
	usos     int64
	limitado bool
}

func (c *contadorDeVariedade) registrar(valor string) {
	c.usos++
	if c.limitado {
		return
	}
	if len(c.vistos) >= tetoDeValoresDistintos {
		c.limitado = true
		return
	}
	c.vistos[valor] = struct{}{}
}

// Disponivel por variavel: quantos valores a fonte que alimenta aquela variavel
// tem para oferecer. E o que permite dizer que usar um valor so foi defeito, e
// nao um cenario que declarou um valor so.
type Disponibilidade map[string]int64

const DisponibilidadeIndefinida = int64(-1)

func montarVariedades(contadores map[string]*contadorDeVariedade, disponivel Disponibilidade) []Variedade {
	nomes := make([]string, 0, len(contadores))
	for nome := range contadores {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)

	variedades := make([]Variedade, 0, len(nomes))
	for _, nome := range nomes {
		contador := contadores[nome]
		variedade := Variedade{
			Nome:      nome,
			Distintos: int64(len(contador.vistos)),
			Usos:      contador.usos,
			Limitado:  contador.limitado,
		}
		if contador.limitado {
			variedade.Distintos = tetoDeValoresDistintos
		}
		if quantos, sabe := disponivel[nome]; sabe {
			variedade.Disponivel = quantos
		}
		variedade.Frase = frasearVariedade(variedade)
		variedades = append(variedades, variedade)
	}
	return variedades
}

func frasearVariedade(v Variedade) string {
	if v.Limitado {
		return fmt.Sprintf("mais de %d valores distintos de %s em %s usos", tetoDeValoresDistintos-1, v.Nome, milhar(v.Usos))
	}
	if v.Distintos == 1 {
		return fmt.Sprintf("1 unico valor de %s em %s usos", v.Nome, milhar(v.Usos))
	}
	return fmt.Sprintf("%d valores distintos de %s em %s usos", v.Distintos, v.Nome, milhar(v.Usos))
}

// O bug que motivou esta metrica: a autenticacao congelava os dados da primeira
// iteracao e a execucao inteira rodava sobre um assinante so, com o relatorio
// declarando variedade que nao existiu.
//
// A gravidade separa dois casos diferentes: fonte com varios valores e execucao
// com um so e defeito e invalida o resultado; valor fixo declarado no cenario e
// escolha de quem escreveu, e vira aviso de leitura.
func AvisosDeVariedade(variedades []Variedade) []Aviso {
	var avisos []Aviso
	for _, variedade := range variedades {
		if variedade.Distintos != 1 || variedade.Usos < 2 {
			continue
		}
		if variedade.Disponivel == 1 {
			continue
		}

		if variedade.Disponivel == 0 {
			avisos = append(avisos, Aviso{
				Tipo:      "valor_fixo",
				Gravidade: GravidadeMedia,
				Mensagem: fmt.Sprintf("a carga inteira usou o mesmo valor de %s; se o alvo guardar resposta por esse valor, o numero fica otimista",
					variedade.Nome),
				Evidencia: fmt.Sprintf("%s: 1 valor em %s usos", variedade.Nome, milhar(variedade.Usos)),
			})
			continue
		}

		mensagem := fmt.Sprintf("a execucao inteira rodou com um unico valor de %s, embora a fonte tenha mais; o alvo pode ter respondido de cache, e o resultado nao representa a carga declarada",
			variedade.Nome)
		if strings.HasPrefix(variedade.Nome, "kafka.particao.") {
			mensagem = fmt.Sprintf("toda a carga caiu numa particao so de %s; o resto do cluster ficou parado e o numero nao representa producao. Faca a chave da mensagem variar por iteracao",
				strings.TrimPrefix(strings.TrimPrefix(variedade.Nome, "kafka.particao.consumida."), "kafka.particao."))
		}

		evidencia := fmt.Sprintf("%s tinha %d valores disponiveis e a execucao usou 1, em %s usos",
			variedade.Nome, variedade.Disponivel, milhar(variedade.Usos))
		if variedade.Disponivel < 0 {
			evidencia = fmt.Sprintf("%s e gerada por iteracao e mesmo assim repetiu o mesmo valor em %s usos",
				variedade.Nome, milhar(variedade.Usos))
		}
		avisos = append(avisos, Aviso{
			Tipo:      "variedade_ausente",
			Gravidade: GravidadeAlta,
			Mensagem:  mensagem,
			Evidencia: evidencia,
		})
	}
	return avisos
}

func milhar(valor int64) string {
	texto := fmt.Sprintf("%d", valor)
	if len(texto) <= 3 {
		return texto
	}
	var partes []string
	for len(texto) > 3 {
		partes = append([]string{texto[len(texto)-3:]}, partes...)
		texto = texto[:len(texto)-3]
	}
	partes = append([]string{texto}, partes...)
	return join(partes, ".")
}

func join(partes []string, separador string) string {
	saida := ""
	for indice, parte := range partes {
		if indice > 0 {
			saida += separador
		}
		saida += parte
	}
	return saida
}
