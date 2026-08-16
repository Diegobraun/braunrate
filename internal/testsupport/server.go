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
	options    Options
	served     atomic.Int64
	created    atomic.Int64
	mutexPausa sync.RWMutex
	fimDaPausa time.Time
	armarPausa sync.Once
	random     *rand.Rand
	randomMu   sync.Mutex
	server     *http.Server
	listener   net.Listener
}

func New(options Options) *Server {
	return &Server{options: options, random: rand.New(rand.NewSource(1))}
}

func (server *Server) Address() string {
	if server.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", server.listener.Addr().String())
}

func (server *Server) Served() int64 { return server.served.Load() }

func (server *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handle)
	mux.HandleFunc("/auth/token", server.wrap(server.handleToken))
	// /pedidos e o passo que o 'braunrate new' escreve: exigir token aqui fazia
	// new, target e execute devolverem 401 no primeiro contato de quem chega.
	mux.HandleFunc("/pedidos", server.wrap(server.handleOrder))
	mux.HandleFunc("/pedidos/", server.wrap(server.handleOrder))
	mux.HandleFunc("/faturas/", server.wrap(server.requireToken(server.handlePayment)))
	mux.HandleFunc("/graphql", server.wrap(server.requireToken(server.handleGraphQL)))
	mux.HandleFunc("/saude", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/congelar", server.handleFreeze)
	server.server = &http.Server{Handler: mux}

	go func() { _ = server.server.Serve(listener) }()
	return nil
}

func (server *Server) Close() error {
	if server.server == nil {
		return nil
	}
	return server.server.Close()
}

// The freeze holds the whole handler to reproduce a stop-the-world on the
// target, which is the situation where coordinated omission shows up.
func (server *Server) Freeze(duration time.Duration) {
	server.mutexPausa.Lock()
	server.fimDaPausa = time.Now().Add(duration)
	server.mutexPausa.Unlock()
}

// The freeze is counted from the first request, not from Start. Counting from
// Start put it on the wall clock of the whole test — engine setup, data loading
// and a busy machine all shifted the run relative to the freeze, and the window
// could fall outside the measurement. Anchored on the first request it lands in
// the same place every time.
func (server *Server) armFreeze() {
	if server.options.FreezeFor <= 0 {
		return
	}
	server.armarPausa.Do(func() {
		go func() {
			time.Sleep(server.options.FreezeAfter)
			server.Freeze(server.options.FreezeFor)
		}()
	})
}

func (server *Server) waitForResume() {
	server.armFreeze()
	server.mutexPausa.RLock()
	end := server.fimDaPausa
	server.mutexPausa.RUnlock()
	if remaining := time.Until(end); remaining > 0 {
		time.Sleep(remaining)
	}
}

func (server *Server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	duration, err := time.ParseDuration(r.URL.Query().Get("por"))
	if err != nil {
		http.Error(w, "parâmetro 'por' inválido", http.StatusBadRequest)
		return
	}
	server.Freeze(duration)
	w.WriteHeader(http.StatusAccepted)
}

func (server *Server) handle(w http.ResponseWriter, r *http.Request) {
	server.waitForResume()

	wait := server.options.Latency
	if server.options.Jitter > 0 {
		server.randomMu.Lock()
		wait += time.Duration(server.random.Int63n(int64(server.options.Jitter)))
		server.randomMu.Unlock()
	}
	if wait > 0 {
		time.Sleep(wait)
	}

	number := server.served.Add(1)

	if server.options.ErrorProportion > 0 && server.options.ErrorStatus > 0 {
		server.randomMu.Lock()
		draw := server.random.Float64()
		server.randomMu.Unlock()
		if draw < server.options.ErrorProportion {
			w.WriteHeader(server.options.ErrorStatus)
			_, _ = fmt.Fprintf(w, `{"id":%d,"status":"ERRO"}`, number)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":%d,"status":"OK","caminho":%q}`, number, r.URL.Path)
}

// The authenticated journey exists so the README example works without anyone
// needing a real service at hand.
func (server *Server) wrap(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server.waitForResume()
		if server.options.Latency > 0 {
			time.Sleep(server.options.Latency)
		}
		server.served.Add(1)
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}
}

func (server *Server) requireToken(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+server.token() {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"erro":"token ausente ou inválido"}`)
			return
		}
		handler(w, r)
	}
}

func (server *Server) token() string { return "token-de-teste" }

func (server *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_, _ = fmt.Fprintf(w, `{"access_token":%q,"expira_em":1800}`, server.token())
}

func (server *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	order := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/pedidos"), "/")
	if order == "" {
		if r.Method == http.MethodPost {
			server.created.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"id":"p-%d","status":"ABERTO"}`, server.created.Load())
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"erro":"informe o pedido"}`)
		return
	}
	_, _ = fmt.Fprintf(w, `{"id":%q,"status":"ABERTO","ultimaFatura":{"id":"f-%s","valor":199.90,"status":"ABERTA"}}`,
		order, order)
}

func (server *Server) handlePayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	invoice := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/faturas/"), "/pagar")
	_, _ = fmt.Fprintf(w, `{"id":%q,"status":"PAGA","pagoEm":"2026-08-15T00:00:00Z"}`, invoice)
}
