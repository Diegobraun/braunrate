package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type alvo struct {
	latencia    time.Duration
	jitter      time.Duration
	atendidas   atomic.Int64
	mutexPausa  sync.RWMutex
	fimDaPausa  time.Time
}

func (a *alvo) pausar(inicio time.Time, duracao time.Duration) {
	a.mutexPausa.Lock()
	a.fimDaPausa = inicio.Add(duracao)
	a.mutexPausa.Unlock()
}

// A pausa segura o handler inteiro para reproduzir stop-the-world do alvo,
// que e o cenario onde a omissao coordenada aparece.
func (a *alvo) esperarFimDaPausa() {
	a.mutexPausa.RLock()
	fim := a.fimDaPausa
	a.mutexPausa.RUnlock()
	if restante := time.Until(fim); restante > 0 {
		time.Sleep(restante)
	}
}

func (a *alvo) tratar(w http.ResponseWriter, r *http.Request) {
	a.esperarFimDaPausa()
	espera := a.latencia
	if a.jitter > 0 {
		espera += time.Duration(rand.Int63n(int64(a.jitter)))
	}
	if espera > 0 {
		time.Sleep(espera)
	}
	n := a.atendidas.Add(1)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"status":"OK"}`, n)
}

func main() {
	porta := flag.Int("porta", 8080, "porta de escuta")
	latencia := flag.Duration("latencia", 5*time.Millisecond, "latencia fixa por requisicao")
	jitter := flag.Duration("jitter", 0, "variacao aleatoria somada a latencia")
	congelarApos := flag.Duration("congelar-apos", 0, "instante, contado do start, em que o alvo congela")
	congelarPor := flag.Duration("congelar-por", 0, "duracao do congelamento")
	flag.Parse()

	a := &alvo{latencia: *latencia, jitter: *jitter}

	if *congelarPor > 0 {
		go func() {
			time.Sleep(*congelarApos)
			inicio := time.Now()
			a.pausar(inicio, *congelarPor)
			fmt.Fprintf(os.Stderr, "alvo congelado por %s\n", *congelarPor)
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/pedido", a.tratar)
	mux.HandleFunc("/saude", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/atendidas", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%d\n", a.atendidas.Load())
	})

	servidor := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", *porta),
		Handler: mux,
	}
	fmt.Fprintf(os.Stderr, "alvo em 127.0.0.1:%d latencia=%s jitter=%s\n", *porta, *latencia, *jitter)
	log.Fatal(servidor.ListenAndServe())
}
