package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

const maiorLatenciaUs = int64(300_000_000)

type medidor struct {
	mutex                sync.Mutex
	latenciaCorrigida    *hdrhistogram.Histogram
	latenciaDeServico    *hdrhistogram.Histogram
	desvioDeAgendamento  *hdrhistogram.Histogram
}

func novoMedidor() *medidor {
	return &medidor{
		latenciaCorrigida:   hdrhistogram.New(1, maiorLatenciaUs, 3),
		latenciaDeServico:   hdrhistogram.New(1, maiorLatenciaUs, 3),
		desvioDeAgendamento: hdrhistogram.New(1, maiorLatenciaUs, 3),
	}
}

// hdrhistogram-go nao e seguro para escrita concorrente; o mutex e o que
// garante que nenhuma amostra se perca sob milhares de goroutines.
func (m *medidor) registrar(histograma *hdrhistogram.Histogram, valorUs int64) {
	if valorUs < 1 {
		valorUs = 1
	}
	m.mutex.Lock()
	histograma.RecordValue(valorUs)
	m.mutex.Unlock()
}

func percentis(nome string, histograma *hdrhistogram.Histogram) string {
	return fmt.Sprintf("  %q: {\"p50\": %d, \"p90\": %d, \"p99\": %d, \"p999\": %d, \"max\": %d, \"amostras\": %d}",
		nome,
		histograma.ValueAtQuantile(50),
		histograma.ValueAtQuantile(90),
		histograma.ValueAtQuantile(99),
		histograma.ValueAtQuantile(99.9),
		histograma.Max(),
		histograma.TotalCount())
}

// Sleep sozinho erra na casa de milissegundos; a espera ativa final e o que
// sustenta desvio de agendamento abaixo de 100 us em taxa alta.
func dormirAte(instanteAlvo time.Time) {
	restante := time.Until(instanteAlvo)
	if restante > 2*time.Millisecond {
		time.Sleep(restante - 1500*time.Microsecond)
	}
	for time.Now().Before(instanteAlvo) {
	}
}

func cpuGastoNs() int64 {
	var uso syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &uso)
	usuario := uso.Utime.Sec*1_000_000_000 + int64(uso.Utime.Usec)*1000
	sistema := uso.Stime.Sec*1_000_000_000 + int64(uso.Stime.Usec)*1000
	return usuario + sistema
}

func main() {
	alvo := flag.String("alvo", "http://127.0.0.1:8080/pedido", "url do alvo")
	taxa := flag.Int64("taxa", 1000, "taxa de chegada por segundo")
	duracao := flag.Duration("duracao", 10*time.Second, "duracao da janela medida")
	aquecimento := flag.Duration("aquecimento", 2*time.Second, "janela descartada antes de medir")
	esperaAntes := flag.Duration("espera-antes", 2*time.Second, "pausa apos PRONTO, para amostrar RSS em repouso")
	limiarAtraso := flag.Duration("limiar-atraso", 10*time.Millisecond, "atraso de despacho que conta como back-pressure")
	flag.Parse()

	transporte := &http.Transport{
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: 65536,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
	}
	cliente := &http.Client{Transport: transporte, Timeout: 30 * time.Second}

	if resposta, err := cliente.Get(*alvo); err == nil {
		io.Copy(io.Discard, resposta.Body)
		resposta.Body.Close()
	}

	fmt.Fprintln(os.Stderr, "PRONTO")
	time.Sleep(*esperaAntes)

	m := novoMedidor()
	var enviadas, concluidas, erros, despachosAtrasados atomic.Int64
	var emAndamento, picoEmAndamento atomic.Int64
	var grupo sync.WaitGroup

	intervalo := time.Duration(int64(time.Second) / *taxa)
	inicio := time.Now()
	fimDoAquecimento := inicio.Add(*aquecimento)
	fim := fimDoAquecimento.Add(*duracao)
	cpuNoInicio := cpuGastoNs()
	relogioNoInicio := inicio
	medindo := false

	for indice := int64(0); ; indice++ {
		agendado := inicio.Add(time.Duration(indice) * intervalo)
		if !agendado.Before(fim) {
			break
		}
		dormirAte(agendado)
		despacho := time.Now()
		valeMedir := !despacho.Before(fimDoAquecimento)
		if valeMedir && !medindo {
			medindo = true
			cpuNoInicio = cpuGastoNs()
			relogioNoInicio = despacho
		}
		atrasoUs := despacho.Sub(agendado).Microseconds()
		if valeMedir {
			m.registrar(m.desvioDeAgendamento, atrasoUs)
			if despacho.Sub(agendado) > *limiarAtraso {
				despachosAtrasados.Add(1)
			}
		}
		enviadas.Add(1)
		atuais := emAndamento.Add(1)
		for {
			pico := picoEmAndamento.Load()
			if atuais <= pico || picoEmAndamento.CompareAndSwap(pico, atuais) {
				break
			}
		}

		grupo.Add(1)
		go func() {
			defer grupo.Done()
			defer emAndamento.Add(-1)
			envio := time.Now()
			resposta, err := cliente.Get(*alvo)
			if err != nil {
				erros.Add(1)
				return
			}
			io.Copy(io.Discard, resposta.Body)
			resposta.Body.Close()
			termino := time.Now()
			if resposta.StatusCode != http.StatusOK {
				erros.Add(1)
			} else if valeMedir {
				m.registrar(m.latenciaCorrigida, termino.Sub(agendado).Microseconds())
				m.registrar(m.latenciaDeServico, termino.Sub(envio).Microseconds())
			}
			concluidas.Add(1)
		}()
	}

	fimDoDespacho := time.Now()
	grupo.Wait()
	cpuGasto := cpuGastoNs() - cpuNoInicio
	relogioGasto := fimDoDespacho.Sub(relogioNoInicio).Nanoseconds()

	medidas := m.latenciaCorrigida.TotalCount()
	taxaEfetiva := 0.0
	cpuPorRequisicao := int64(0)
	if medidas > 0 {
		taxaEfetiva = float64(medidas) / (float64(relogioGasto) / 1e9)
		cpuPorRequisicao = cpuGasto / medidas
	}

	fmt.Printf("{\n")
	fmt.Printf("  \"prototipo\": \"go-goroutines\",\n")
	fmt.Printf("  \"taxa_alvo\": %d,\n", *taxa)
	fmt.Printf("  \"taxa_efetiva\": %.1f,\n", taxaEfetiva)
	fmt.Printf("  \"enviadas\": %d,\n", enviadas.Load())
	fmt.Printf("  \"concluidas\": %d,\n", concluidas.Load())
	fmt.Printf("  \"erros\": %d,\n", erros.Load())
	fmt.Printf("  \"medidas\": %d,\n", medidas)
	fmt.Printf("  \"drenou\": true,\n")
	fmt.Printf("  \"pico_em_andamento\": %d,\n", picoEmAndamento.Load())
	fmt.Printf("  \"despachos_atrasados\": %d,\n", despachosAtrasados.Load())
	fmt.Printf("  \"cpu_ns_por_requisicao\": %d,\n", cpuPorRequisicao)
	fmt.Printf("  \"cpu_percentual_de_um_nucleo\": %.1f,\n", 100.0*float64(cpuGasto)/float64(relogioGasto))
	fmt.Printf("%s,\n", percentis("latencia_corrigida_us", m.latenciaCorrigida))
	fmt.Printf("%s,\n", percentis("latencia_de_servico_us", m.latenciaDeServico))
	fmt.Printf("%s\n", percentis("desvio_de_agendamento_us", m.desvioDeAgendamento))
	fmt.Printf("}\n")
}
