package testsupport

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// RawServer e o alvo mais barato que ainda fala HTTP: ele nao interpreta a
// requisicao, nao aloca por chamada e nao formata resposta — conta os fins de
// cabecalho que chegaram e devolve uma resposta pronta para cada um.
//
// Por que existe: medir o teto do gerador com o alvo normal mede o par
// gerador+alvo. Na Fase 0, acima de ~30.000/s o processo alvo ja consumia 2,1
// dos 10 nucleos, e o proprio documento de medicao declara que dali para cima o
// numero nao e do gerador. Um alvo que custa quase nada devolve o eixo.
//
// O que ele nao e: um servidor HTTP. Ele nao roteia, nao le metodo, nao valida
// nada. Serve para medir o gerador, e dizer isso e parte de usa-lo.
type RawServer struct {
	listener net.Listener
	response []byte
	served   atomic.Int64

	closing atomic.Bool
	group   sync.WaitGroup

	mutex sync.Mutex
	open  map[net.Conn]struct{}
}

var rawResponse = []byte("HTTP/1.1 200 OK\r\n" +
	"Content-Type: application/json\r\n" +
	"Content-Length: 22\r\n" +
	"\r\n" +
	`{"id":1,"status":"OK"}`)

func NewRaw() *RawServer {
	return &RawServer{response: rawResponse, open: map[net.Conn]struct{}{}}
}

func (server *RawServer) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server.listener = listener
	server.group.Add(1)
	go server.accept()
	return nil
}

func (server *RawServer) Address() string {
	if server.listener == nil {
		return ""
	}
	return server.listener.Addr().String()
}

func (server *RawServer) Served() int64 { return server.served.Load() }

func (server *RawServer) Close() error {
	server.closing.Store(true)
	if server.listener == nil {
		return nil
	}
	err := server.listener.Close()
	// Uma conexao com keep-alive fica esperando a proxima requisicao, entao
	// esperar por ela e esperar para sempre: fechar e o que faz o encerramento
	// terminar.
	server.mutex.Lock()
	for connection := range server.open {
		_ = connection.Close()
	}
	server.mutex.Unlock()
	server.group.Wait()
	return err
}

func (server *RawServer) accept() {
	defer server.group.Done()
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			if server.closing.Load() || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if tcp, isTCP := connection.(*net.TCPConn); isTCP {
			_ = tcp.SetNoDelay(true)
		}
		server.mutex.Lock()
		server.open[connection] = struct{}{}
		server.mutex.Unlock()
		server.group.Add(1)
		go server.serve(connection)
	}
}

var headerEnd = []byte("\r\n\r\n")

// Uma leitura pode trazer varias requisicoes ou parte de uma. Contar os fins de
// cabecalho e o suficiente para nao responder a mais nem a menos, e nao exige
// interpretar nada.
func (server *RawServer) serve(connection net.Conn) {
	defer server.group.Done()
	defer func() {
		server.mutex.Lock()
		delete(server.open, connection)
		server.mutex.Unlock()
		_ = connection.Close()
	}()

	buffer := make([]byte, 8192)
	pending := 0
	for {
		read, err := connection.Read(buffer)
		if read > 0 {
			pending += bytes.Count(buffer[:read], headerEnd)
			for pending > 0 {
				if _, writeErr := connection.Write(server.response); writeErr != nil {
					return
				}
				server.served.Add(1)
				pending--
			}
		}
		if err != nil {
			if err != io.EOF && !server.closing.Load() {
				return
			}
			return
		}
	}
}
