package relatorio

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/Diegobraun/braunrate/metrica"
)

// O CSV existe para planilha e para juntar execucoes; o campo tipo_de_latencia
// vai junto porque uma coluna de p95 sem ele mistura latencia corrigida com
// tempo de servico na mesma media.
func CSV(saida io.Writer, documento metrica.Documento) error {
	escritor := csv.NewWriter(saida)
	defer escritor.Flush()

	cabecalho := []string{
		"cenario", "alvo", "inicio", "passo", "tipo_de_latencia", "contagem", "erros",
		"p50_ms", "p95_ms", "p99_ms", "p99_9_ms", "max_ms", "bytes",
	}
	if err := escritor.Write(cabecalho); err != nil {
		return err
	}

	inicio := documento.Execucao.Inicio.Format("2006-01-02T15:04:05Z07:00")
	linha := func(nome, tipo string, contagem, erros, bytes int64, distribuicao metrica.Distribuicao) []string {
		return []string{
			documento.Execucao.Cenario, documento.Execucao.Alvo, inicio, nome, tipo,
			fmt.Sprintf("%d", contagem), fmt.Sprintf("%d", erros),
			numero(distribuicao.P50), numero(distribuicao.P95), numero(distribuicao.P99),
			numero(distribuicao.P999), numero(distribuicao.Maximo), fmt.Sprintf("%d", bytes),
		}
	}

	if documento.Jornada.Iniciadas > 0 {
		perdidas := documento.Jornada.Iniciadas - documento.Jornada.Completas
		if err := escritor.Write(linha("jornada inteira", "corrigida", documento.Jornada.Iniciadas, perdidas, 0, documento.Jornada.Latencia)); err != nil {
			return err
		}
	}
	for _, passo := range documento.Passos {
		if err := escritor.Write(linha(passo.Nome, passo.TipoDeLatencia, passo.Contagem, passo.Erros, passo.Bytes, passo.Latencia)); err != nil {
			return err
		}
	}
	return escritor.Write(linha("global", "corrigida", documento.Global.Contagem, documento.Global.Erros, 0, documento.Global.Latencia))
}

func numero(valor float64) string {
	return fmt.Sprintf("%.3f", valor)
}
