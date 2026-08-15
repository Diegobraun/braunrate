package alvo

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Opcoes struct {
	Latencia        time.Duration
	Jitter          time.Duration
	CongelarApos    time.Duration
	CongelarPor     time.Duration
	StatusDeErro    int
	ProporcaoDeErro float64
}

type Servidor struct {
	opcoes         Opcoes
	atendidas      atomic.Int64
	mutexPausa     sync.RWMutex
	fimDaPausa     time.Time
	aleatorio      *rand.Rand
	mutexAleatorio sync.Mutex
	servidor       *http.Server
	ouvinte        net.Listener
}

func Novo(opcoes Opcoes) *Servidor {
	return &Servidor{opcoes: opcoes, aleatorio: rand.New(rand.NewSource(1))}
}

func (s *Servidor) Endereco() string {
	if s.ouvinte == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", s.ouvinte.Addr().String())
}

func (s *Servidor) Atendidas() int64 { return s.atendidas.Load() }

func (s *Servidor) Iniciar(endereco string) error {
	ouvinte, err := net.Listen("tcp", endereco)
	if err != nil {
		return err
	}
	s.ouvinte = ouvinte

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.tratar)
	mux.HandleFunc("/saude", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/congelar", s.tratarCongelamento)
	s.servidor = &http.Server{Handler: mux}

	if s.opcoes.CongelarPor > 0 {
		go func() {
			time.Sleep(s.opcoes.CongelarApos)
			s.Congelar(s.opcoes.CongelarPor)
		}()
	}

	go func() { _ = s.servidor.Serve(ouvinte) }()
	return nil
}

func (s *Servidor) Encerrar() error {
	if s.servidor == nil {
		return nil
	}
	return s.servidor.Close()
}

// O congelamento segura o handler inteiro para reproduzir stop-the-world do
// alvo, que e a situacao em que a omissao coordenada aparece.
func (s *Servidor) Congelar(duracao time.Duration) {
	s.mutexPausa.Lock()
	s.fimDaPausa = time.Now().Add(duracao)
	s.mutexPausa.Unlock()
}

func (s *Servidor) esperarFimDaPausa() {
	s.mutexPausa.RLock()
	fim := s.fimDaPausa
	s.mutexPausa.RUnlock()
	if restante := time.Until(fim); restante > 0 {
		time.Sleep(restante)
	}
}

func (s *Servidor) tratarCongelamento(w http.ResponseWriter, r *http.Request) {
	duracao, err := time.ParseDuration(r.URL.Query().Get("por"))
	if err != nil {
		http.Error(w, "parametro 'por' invalido", http.StatusBadRequest)
		return
	}
	s.Congelar(duracao)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Servidor) tratar(w http.ResponseWriter, r *http.Request) {
	s.esperarFimDaPausa()

	espera := s.opcoes.Latencia
	if s.opcoes.Jitter > 0 {
		s.mutexAleatorio.Lock()
		espera += time.Duration(s.aleatorio.Int63n(int64(s.opcoes.Jitter)))
		s.mutexAleatorio.Unlock()
	}
	if espera > 0 {
		time.Sleep(espera)
	}

	numero := s.atendidas.Add(1)

	if s.opcoes.ProporcaoDeErro > 0 && s.opcoes.StatusDeErro > 0 {
		s.mutexAleatorio.Lock()
		sorteio := s.aleatorio.Float64()
		s.mutexAleatorio.Unlock()
		if sorteio < s.opcoes.ProporcaoDeErro {
			w.WriteHeader(s.opcoes.StatusDeErro)
			fmt.Fprintf(w, `{"id":%d,"status":"ERRO"}`, numero)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"status":"OK","caminho":%q}`, numero, r.URL.Path)
}
