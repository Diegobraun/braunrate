package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Diegobraun/braunrate/alvo"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/motor"
	"github.com/Diegobraun/braunrate/protocolo"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
	"github.com/Diegobraun/braunrate/relatorio"
	"github.com/Diegobraun/braunrate/slo"
)

const versao = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		uso()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "executar":
		os.Exit(executar(os.Args[2:]))
	case "validar":
		os.Exit(validar(os.Args[2:]))
	case "alvo":
		os.Exit(servirAlvo(os.Args[2:]))
	case "versao":
		fmt.Printf("braunrate %s\nprotocolos compilados: %v\n", versao, protocolo.Registrados())
		os.Exit(0)
	default:
		uso()
		os.Exit(2)
	}
}

func uso() {
	fmt.Fprintf(os.Stderr, `braunrate %s

uso:
  braunrate executar <cenario.yaml> [opcoes]
  braunrate validar <cenario.yaml>
  braunrate alvo [opcoes]
  braunrate versao

opcoes de executar:
  -resultado <arquivo.json>   grava o documento de resultado
  -maximo-simultaneas <n>     maximo de requisicoes simultaneas (padrao 20000)
  -atraso-tolerado <dur>      a partir daqui o gerador conta como saturado (padrao 10ms)
  -silencioso                 nao imprime progresso durante a execucao
`, versao)
}

func executar(argumentos []string) int {
	conjunto := flag.NewFlagSet("executar", flag.ExitOnError)
	arquivoDeResultado := conjunto.String("resultado", "", "arquivo JSON de resultado")
	maximoSimultaneas := conjunto.Int64("maximo-simultaneas", 20000, "maximo de requisicoes simultaneas antes de desistir de disparar")
	limiarDeAtraso := conjunto.Duration("atraso-tolerado", 10*time.Millisecond, "atraso de disparo a partir do qual o gerador e considerado saturado")
	silencioso := conjunto.Bool("silencioso", false, "nao imprime progresso")
	_ = conjunto.Parse(argumentos)

	if conjunto.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenario")
		return 2
	}

	c, err := cenario.CarregarArquivo(conjunto.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no cenario: %v\n", err)
		return 2
	}
	if err := c.Validar(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	opcoes := motor.OpcoesPadrao()
	opcoes.Versao = versao
	opcoes.MaximoSimultaneas = *maximoSimultaneas
	opcoes.RaizDeDados = filepath.Dir(conjunto.Arg(0))
	opcoes.LimiarDeAtraso = *limiarDeAtraso
	if !*silencioso {
		opcoes.AoProgredir = func(instantaneo metrica.Instantaneo, taxaAlvo float64, restante time.Duration) {
			fmt.Fprintf(os.Stderr, "\r%s", relatorio.LinhaDeProgresso(instantaneo, taxaAlvo, restante))
		}
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	m, err := motor.Novo(c, opcoes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "executando %q contra %s: %s iteracoes em %s\n",
		c.Nome, c.Alvo, humanizar(m.Plano().TotalDeRequisicoes()), m.Plano().Duracao())

	documento := m.Executar(ctx)
	protocolo.EncerrarTodos()
	if !*silencioso {
		fmt.Fprintln(os.Stderr)
	}
	veredito := slo.Avaliar(c.SLO, documento)
	relatorio.Resumo(os.Stdout, documento, veredito)

	if *arquivoDeResultado != "" {
		conteudo, err := json.MarshalIndent(documento, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro ao serializar resultado: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*arquivoDeResultado, conteudo, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "erro ao gravar resultado: %v\n", err)
			return 1
		}
	}

	if !documento.ResultadoValido() {
		return 3
	}
	if !veredito.Passou {
		return 1
	}
	return 0
}

func humanizar(valor int64) string {
	return fmt.Sprintf("%d", valor)
}

func validar(argumentos []string) int {
	if len(argumentos) < 1 {
		fmt.Fprintln(os.Stderr, "informe o arquivo de cenario")
		return 2
	}
	c, err := cenario.CarregarArquivo(argumentos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no cenario: %v\n", err)
		return 2
	}
	if err := c.Validar(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	plano := motor.CompilarPlano(c.Carga)
	fmt.Printf("Cenario valido: %q, %d passo(s), %d iteracoes em %s.\n",
		c.Nome, len(c.Passos), plano.TotalDeRequisicoes(), plano.Duracao())
	if len(c.SLO) == 0 {
		fmt.Println("Sem slo declarado: a execucao nunca vai falhar por lentidao. Adicione um bloco 'slo' para virar gate de CI.")
	}
	return 0
}

func servirAlvo(argumentos []string) int {
	conjunto := flag.NewFlagSet("alvo", flag.ExitOnError)
	endereco := conjunto.String("endereco", "127.0.0.1:8080", "endereco de escuta")
	latencia := conjunto.Duration("latencia", 5*time.Millisecond, "latencia fixa por requisicao")
	jitter := conjunto.Duration("jitter", 0, "variacao aleatoria somada a latencia")
	congelarApos := conjunto.Duration("congelar-apos", 0, "instante em que o alvo congela")
	congelarPor := conjunto.Duration("congelar-por", 0, "duracao do congelamento")
	_ = conjunto.Parse(argumentos)

	servidor := alvo.Novo(alvo.Opcoes{
		Latencia:     *latencia,
		Jitter:       *jitter,
		CongelarApos: *congelarApos,
		CongelarPor:  *congelarPor,
	})
	if err := servidor.Iniciar(*endereco); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao subir alvo: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "alvo de teste em %s (latencia %s)\n", servidor.Endereco(), *latencia)

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()
	<-ctx.Done()
	_ = servidor.Encerrar()
	fmt.Fprintf(os.Stderr, "\natendidas: %d\n", servidor.Atendidas())
	return 0
}
