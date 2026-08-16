package aguardar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
	"github.com/Diegobraun/braunrate/protocolo/transporte"
	"github.com/tidwall/gjson"
)

const intervaloPadrao = 500 * time.Millisecond

// Boa parte dos sistemas assincronos so mostra o efeito por API: nao ha topico
// para escutar, e sem isto a cadeia ponta a ponta nao se mede neles. A medicao
// por sondagem e honesta desde que a granularidade seja declarada — o valor
// medido e sempre maior ou igual ao real, ate um intervalo de sondagem.
type Condicao struct {
	Status      int
	Caminho     string
	Valor       string
	CorpoContem string
}

func (c Condicao) vazia() bool {
	return c.Status == 0 && c.Caminho == "" && c.CorpoContem == ""
}

func (c Condicao) descrever() string {
	switch {
	case c.Caminho != "":
		return fmt.Sprintf("%s = %q", c.Caminho, c.Valor)
	case c.CorpoContem != "":
		return fmt.Sprintf("o corpo conter %q", c.CorpoContem)
	default:
		return fmt.Sprintf("status %d", c.Status)
	}
}

func (c Condicao) satisfeita(status int, corpo []byte) bool {
	switch {
	case c.Caminho != "":
		return gjson.GetBytes(corpo, strings.TrimPrefix(strings.TrimPrefix(c.Caminho, "$."), "$")).String() == c.Valor
	case c.CorpoContem != "":
		return strings.Contains(string(corpo), c.CorpoContem)
	default:
		return status == c.Status
	}
}

func (p *Protocolo) esperarPorHTTP(ctx context.Context, requisicao protocolo.Requisicao, configuracao *Configuracao) protocolo.Resposta {
	endereco, err := transporte.MontarURL(requisicao.URLBase, configuracao.Caminho)
	if err != nil {
		return protocolo.Resposta{Classe: protocolo.ErroDeConfigacao, Detalhe: err.Error()}
	}

	timeout := configuracao.Timeout
	if timeout <= 0 {
		timeout = timeoutPadrao
	}
	intervalo := configuracao.Intervalo
	if intervalo <= 0 {
		intervalo = intervaloPadrao
	}

	limite, cancelar := context.WithTimeout(ctx, timeout)
	defer cancelar()

	var ultimoStatus int
	var ultimoCorpo []byte
	var ultimoErro string
	sondagens := 0

	for {
		status, corpo, err := p.sondar(limite, endereco)
		sondagens++
		if err != nil {
			ultimoErro = err.Error()
		} else {
			ultimoStatus, ultimoCorpo, ultimoErro = status, corpo, ""
			if configuracao.Ate.satisfeita(status, corpo) {
				return protocolo.Resposta{
					Status: status,
					Corpo:  corpo,
					Bytes:  int64(len(corpo)),
					Classe: protocolo.Sucesso,
				}
			}
		}

		select {
		case <-limite.Done():
			return protocolo.Resposta{
				Status:  ultimoStatus,
				Corpo:   ultimoCorpo,
				Classe:  protocolo.ErroDeTimeout,
				Detalhe: detalheDeEspera(configuracao, endereco, timeout, intervalo, sondagens, ultimoStatus, ultimoCorpo, ultimoErro),
			}
		case <-time.After(intervalo):
		}
	}
}

func detalheDeEspera(configuracao *Configuracao, endereco string, timeout, intervalo time.Duration,
	sondagens int, status int, corpo []byte, erro string) string {

	if erro != "" {
		return fmt.Sprintf("esperei %s por %s em %s e a ultima tentativa falhou: %s (%d sondagens a cada %s)",
			timeout, configuracao.Ate.descrever(), endereco, erro, sondagens, intervalo)
	}
	amostra := string(corpo)
	if len(amostra) > 120 {
		amostra = amostra[:120] + "…"
	}
	return fmt.Sprintf("esperei %s por %s em %s e o efeito nao apareceu; ultima resposta: status %d, corpo %q (%d sondagens a cada %s)",
		timeout, configuracao.Ate.descrever(), endereco, status, amostra, sondagens, intervalo)
}

func (p *Protocolo) sondar(ctx context.Context, endereco string) (int, []byte, error) {
	requisicao, err := http.NewRequestWithContext(ctx, http.MethodGet, endereco, nil)
	if err != nil {
		return 0, nil, err
	}
	resposta, err := p.cliente().Do(requisicao)
	if err != nil {
		return 0, nil, err
	}
	defer resposta.Body.Close()

	corpo, err := io.ReadAll(resposta.Body)
	if err != nil {
		return resposta.StatusCode, nil, err
	}
	return resposta.StatusCode, corpo, nil
}

func (p *Protocolo) cliente() *http.Client {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.http == nil {
		p.http = transporte.NovoCliente(protocolo.Opcoes{})
	}
	return p.http
}

func lerCondicao(chave string, bruto map[string]string) (Condicao, error) {
	condicao := Condicao{}
	for nome, valor := range bruto {
		switch nome {
		case "status":
			numero, err := strconv.Atoi(strings.TrimSpace(valor))
			if err != nil {
				return condicao, fmt.Errorf("status invalido em %s: %q (use um numero, por exemplo 200)", chave, valor)
			}
			condicao.Status = numero
		case "corpo_contem":
			condicao.CorpoContem = valor
		default:
			condicao.Caminho = nome
			condicao.Valor = valor
		}
	}
	return condicao, nil
}

// O motor usa isto para declarar no relatorio que aquele passo foi medido por
// sondagem — sem a declaracao, o degrau do intervalo viraria latencia do alvo.
func (c *Configuracao) IntervaloDeSondagem() time.Duration {
	if c.Fonte != "http" {
		return 0
	}
	if c.Intervalo > 0 {
		return c.Intervalo
	}
	return intervaloPadrao
}
