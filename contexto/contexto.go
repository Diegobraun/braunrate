package contexto

import (
	"os"
	"regexp"
	"sync"
)

var padraoDeVariavel = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.-]*)(?::-([^}]*))?\}`)

type Contexto struct {
	UsuarioVirtual int64
	Iteracao       int64
	mutex          sync.RWMutex
	valores        map[string]string
	usos           map[string]string
}

func Novo(usuarioVirtual, iteracao int64, base map[string]string) *Contexto {
	valores := make(map[string]string, len(base)+2)
	for nome, valor := range base {
		valores[nome] = valor
	}
	return &Contexto{UsuarioVirtual: usuarioVirtual, Iteracao: iteracao, valores: valores, usos: map[string]string{}}
}

func (c *Contexto) Definir(nome, valor string) {
	c.mutex.Lock()
	c.valores[nome] = valor
	c.mutex.Unlock()
}

func (c *Contexto) DefinirVarios(valores map[string]string) {
	c.mutex.Lock()
	for nome, valor := range valores {
		c.valores[nome] = valor
	}
	c.mutex.Unlock()
}

func (c *Contexto) Valor(nome string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	valor, existe := c.valores[nome]
	return valor, existe
}

func (c *Contexto) Valores() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	copia := make(map[string]string, len(c.valores))
	for nome, valor := range c.valores {
		copia[nome] = valor
	}
	return copia
}

// A resolucao acontece na execucao, e nao no carregamento: sem isso valor
// capturado de um passo nao chega ao passo seguinte.
func (c *Contexto) Resolver(texto string) string {
	if texto == "" {
		return texto
	}
	return padraoDeVariavel.ReplaceAllStringFunc(texto, func(ocorrencia string) string {
		partes := padraoDeVariavel.FindStringSubmatch(ocorrencia)
		nome, padrao := partes[1], partes[2]
		if valor, existe := c.Valor(nome); existe {
			c.anotarUso(nome, valor)
			return valor
		}
		if valor, definida := os.LookupEnv(nome); definida {
			return valor
		}
		return padrao
	})
}

// Cada substituicao feita fica anotada porque e daqui que sai a variedade
// observada: caminho, corpo, cabecalho, variavel de GraphQL e chave de
// mensagem passam todos por aqui, entao um so ponto cobre o cenario inteiro.
func (c *Contexto) anotarUso(nome, valor string) {
	c.mutex.Lock()
	c.usos[nome] = valor
	c.mutex.Unlock()
}

func (c *Contexto) Usos() map[string]string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if len(c.usos) == 0 {
		return nil
	}
	copia := make(map[string]string, len(c.usos))
	for nome, valor := range c.usos {
		copia[nome] = valor
	}
	return copia
}

func NaoResolvidas(texto string) []string {
	var pendentes []string
	for _, ocorrencia := range padraoDeVariavel.FindAllStringSubmatch(texto, -1) {
		pendentes = append(pendentes, ocorrencia[1])
	}
	return pendentes
}
