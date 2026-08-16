package testsupport

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	Latency         time.Duration
	Jitter          time.Duration
	FreezeAfter     time.Duration
	FreezeFor       time.Duration
	ErrorStatus     int
	ErrorProportion float64
}

type Server struct {
	opts       Options
	served     atomic.Int64
	mutexPausa sync.RWMutex
	fimDaPausa time.Time
	random     *rand.Rand
	randomMu   sync.Mutex
	server     *http.Server
	listener   net.Listener
}

func New(opts Options) *Server {
	return &Server{opts: opts, random: rand.New(rand.NewSource(1))}
}

func (s *Server) Address() string {
	if s.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", s.listener.Addr().String())
}

func (s *Server) Atendidas() int64 { return s.served.Load() }

func (s *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	mux.HandleFunc("/auth/token", s.wrap(s.handleToken))
	mux.HandleFunc("/pedidos/", s.wrap(s.requireToken(s.handleOrder)))
	mux.HandleFunc("/faturas/", s.wrap(s.requireToken(s.handlePayment)))
	mux.HandleFunc("/graphql", s.wrap(s.requireToken(s.handleGraphQL)))
	mux.HandleFunc("/saude", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/congelar", s.handleFreeze)
	s.server = &http.Server{Handler: mux}

	if s.opts.FreezeFor > 0 {
		go func() {
			time.Sleep(s.opts.FreezeAfter)
			s.Freeze(s.opts.FreezeFor)
		}()
	}

	go func() { _ = s.server.Serve(listener) }()
	return nil
}

func (s *Server) Close() error {
	if s.server == nil {
		return nil
	}
	return s.server.Close()
}

// O congelamento segura o handler inteiro para reproduzir stop-the-world do
// alvo, que e a situacao em que a omissao coordenada aparece.
func (s *Server) Freeze(duration time.Duration) {
	s.mutexPausa.Lock()
	s.fimDaPausa = time.Now().Add(duration)
	s.mutexPausa.Unlock()
}

func (s *Server) waitForResume() {
	s.mutexPausa.RLock()
	end := s.fimDaPausa
	s.mutexPausa.RUnlock()
	if remaining := time.Until(end); remaining > 0 {
		time.Sleep(remaining)
	}
}

func (s *Server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.URL.Query().Get("por"))
	if err != nil {
		http.Error(w, "parametro 'por' invalido", http.StatusBadRequest)
		return
	}
	s.Freeze(duration)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.waitForResume()

	wait := s.opts.Latency
	if s.opts.Jitter > 0 {
		s.randomMu.Lock()
		wait += time.Duration(s.random.Int63n(int64(s.opts.Jitter)))
		s.randomMu.Unlock()
	}
	if wait > 0 {
		time.Sleep(wait)
	}

	number := s.served.Add(1)

	if s.opts.ErrorProportion > 0 && s.opts.ErrorStatus > 0 {
		s.randomMu.Lock()
		draw := s.random.Float64()
		s.randomMu.Unlock()
		if draw < s.opts.ErrorProportion {
			w.WriteHeader(s.opts.ErrorStatus)
			fmt.Fprintf(w, `{"id":%d,"status":"ERRO"}`, number)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"status":"OK","caminho":%q}`, number, r.URL.Path)
}

// A jornada com autenticacao existe para que o exemplo do README funcione sem
// que ninguem precise de um servico de verdade a mao.
func (s *Server) wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.waitForResume()
		if s.opts.Latency > 0 {
			time.Sleep(s.opts.Latency)
		}
		s.served.Add(1)
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}
}

func (s *Server) requireToken(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.token() {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"erro":"token ausente ou invalido"}`)
			return
		}
		handler(w, r)
	}
}

func (s *Server) token() string { return "token-de-teste" }

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	fmt.Fprintf(w, `{"access_token":%q,"expira_em":1800}`, s.token())
}

func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	order := strings.TrimPrefix(r.URL.Path, "/pedidos/")
	if order == "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"erro":"informe o pedido"}`)
		return
	}
	fmt.Fprintf(w, `{"id":%q,"status":"ABERTO","ultimaFatura":{"id":"f-%s","valor":199.90,"status":"ABERTA"}}`,
		order, order)
}

func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	invoice := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/faturas/"), "/pagar")
	fmt.Fprintf(w, `{"id":%q,"status":"PAGA","pagoEm":"2026-08-15T00:00:00Z"}`, invoice)
}
