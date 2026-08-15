package transporte

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
)

// Um so lugar monta o cliente: HTTP e GraphQL precisam do mesmo pool de
// conexoes, e pool diferente entre protocolos daria numero diferente para a
// mesma carga sem nada no cenario explicando a diferenca.
func NovoCliente(opcoes protocolo.Opcoes) *http.Client {
	transporte := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   65536,
		MaxConnsPerHost:       opcoes.ConexoesPorDestino,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	cliente := &http.Client{Transport: transporte, Timeout: opcoes.Timeout}
	if opcoes.ManterCookies {
		if jarra, err := cookiejar.New(nil); err == nil {
			cliente.Jar = jarra
		}
	}
	if !opcoes.SeguirRedirect {
		cliente.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return cliente
	}
	maximo := opcoes.MaximoDeRedirects
	cliente.CheckRedirect = func(_ *http.Request, anteriores []*http.Request) error {
		if len(anteriores) >= maximo {
			return fmt.Errorf("mais de %d redirects", maximo)
		}
		return nil
	}
	return cliente
}

func MontarURL(base, caminho string) (string, error) {
	if strings.HasPrefix(caminho, "http://") || strings.HasPrefix(caminho, "https://") {
		return caminho, nil
	}
	if base == "" {
		return "", fmt.Errorf("passo com caminho relativo %q e cenario sem alvo", caminho)
	}
	enderecoBase, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("alvo invalido: %q", base)
	}
	relativo, err := url.Parse(caminho)
	if err != nil {
		return "", fmt.Errorf("caminho invalido: %q", caminho)
	}
	return enderecoBase.ResolveReference(relativo).String(), nil
}

func Classificar(err error) protocolo.ClasseDeErro {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocolo.ErroDeTimeout
	}
	var erroDeRede net.Error
	if errors.As(err, &erroDeRede) && erroDeRede.Timeout() {
		return protocolo.ErroDeTimeout
	}
	return protocolo.ErroDeRede
}

func ResumirErro(err error) string {
	texto := err.Error()
	for _, padrao := range []string{"connection refused", "connection reset", "no such host",
		"too many open files", "cannot assign requested address", "context deadline exceeded",
		"EOF", "broken pipe"} {
		if strings.Contains(texto, padrao) {
			return padrao
		}
	}
	if len(texto) > 120 {
		return texto[:120]
	}
	return texto
}

// Token e senha aparecem cortados: a saida de depuracao costuma ir parar em
// ticket e em captura de tela.
func EsconderSegredo(nome, valor string) string {
	nomeMinusculo := strings.ToLower(nome)
	if nomeMinusculo != "authorization" && !strings.Contains(nomeMinusculo, "token") &&
		!strings.Contains(nomeMinusculo, "senha") && !strings.Contains(nomeMinusculo, "secret") {
		return valor
	}
	prefixo, resto, encontrou := strings.Cut(valor, " ")
	if !encontrou {
		prefixo, resto = "", valor
	}
	if len(resto) <= 6 {
		return strings.TrimSpace(prefixo + " ***")
	}
	return strings.TrimSpace(prefixo + " " + resto[:6] + "… (" + fmt.Sprint(len(resto)) + " caracteres)")
}
