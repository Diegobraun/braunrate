package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Diegobraun/braunrate/alvo"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/motor"
	"github.com/Diegobraun/braunrate/protocolo"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
	"github.com/Diegobraun/braunrate/relatorio"
)

const versao = "0.1.0"

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
  -limite-de-voo <n>          maximo de requisicoes simultaneas em voo (padrao 20000)
  -limiar-de-atraso <dur>     atraso de despacho que conta como back-pressure (padrao 10ms)
  -silencioso                 nao imprime progresso durante a execucao
`, versao)
}

func executar(argumentos []string) int {
	conjunto := flag.NewFlagSet("executar", flag.ExitOnError)
	arquivoDeResultado := conjunto.String("resultado", "", "arquivo JSON de resultado")
	limiteDeVoo := conjunto.Int64("limite-de-voo", 20000, "maximo de requisicoes em voo")
	limiarDeAtraso := conjunto.Duration("limiar-de-atraso", 10*time.Millisecond, "atraso que conta como back-pressure")
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
	opcoes.LimiteDeVoo = *limiteDeVoo
	opcoes.LimiarDeAtraso = *limiarDeAtraso
	if !*silencioso {
		opcoes.AoProgredir = func(instantaneo metrica.Instantaneo, taxaAlvo float64, restante time.Duration) {
			fmt.Fprintf(os.Stderr, "\r%s", relatorio.LinhaDeProgresso(instantaneo, taxaAlvo, restante))
		}
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	m := motor.Novo(c, opcoes)
	fmt.Fprintf(os.Stderr, "executando %q contra %s: %d requisicoes agendadas em %s\n",
		c.Nome, c.Alvo, m.Plano().TotalDeRequisicoes(), m.Plano().Duracao())

	documento := m.Executar(ctx)
	protocolo.EncerrarTodos()
	if !*silencioso {
		fmt.Fprintln(os.Stderr)
	}
	relatorio.Resumo(os.Stdout, documento)

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
	return 0
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
	fmt.Printf("cenario valido: %q, %d passos, %d requisicoes agendadas em %s\n",
		c.Nome, len(c.Passos), plano.TotalDeRequisicoes(), plano.Duracao())
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
