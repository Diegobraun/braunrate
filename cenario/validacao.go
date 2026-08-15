package cenario

import (
	"fmt"
	"net/url"
	"strings"
)

// Broker de mensageria nao tem esquema obrigatorio: "127.0.0.1:9092" e a forma
// que todo mundo cola de um docker-compose, e recusar isso seria pedir que a
// pessoa aprenda uma sintaxe que nem o Kafka usa.
func alvoValido(alvo string) bool {
	if endereco, err := url.Parse(alvo); err == nil && endereco.Scheme != "" && endereco.Host != "" {
		return true
	}
	maquina, porta, encontrou := strings.Cut(alvo, ":")
	if !encontrou || maquina == "" || porta == "" {
		return false
	}
	for _, caractere := range porta {
		if caractere < '0' || caractere > '9' {
			return false
		}
	}
	return true
}

func (c Cenario) Validar() error {
	var problemas []string

	if strings.TrimSpace(c.Nome) == "" {
		problemas = append(problemas, "o cenario precisa de um nome")
	}
	if strings.TrimSpace(c.Alvo) == "" {
		problemas = append(problemas, "o cenario precisa de um alvo")
	} else if !alvoValido(c.Alvo) {
		problemas = append(problemas, fmt.Sprintf("alvo invalido: %q (use https://api.exemplo.com, kafka://127.0.0.1:9092 ou amqp://usuario:senha@127.0.0.1:5672/)", c.Alvo))
	}
	if len(c.Passos) == 0 {
		problemas = append(problemas, "o cenario precisa de pelo menos um passo")
	}
	if len(c.Carga.Fases) == 0 {
		problemas = append(problemas, "o cenario precisa de pelo menos um perfil de carga")
	}

	vistos := map[string]int{}
	for _, passo := range c.Passos {
		vistos[passo.Nome]++
		if vistos[passo.Nome] == 2 {
			problemas = append(problemas, fmt.Sprintf("passo com nome repetido: %q (o relatorio agrega por nome)", passo.Nome))
		}
	}

	for _, fase := range c.Carga.Fases {
		if fase.Durante <= 0 {
			problemas = append(problemas, fmt.Sprintf("linha %d: perfil %s sem duracao", fase.Linha, fase.Tipo))
		}
		if fase.Tipo != FaseRampa && fase.Ate <= 0 {
			problemas = append(problemas, fmt.Sprintf("linha %d: perfil %s sem taxa", fase.Linha, fase.Tipo))
		}
	}

	if len(problemas) == 0 {
		return nil
	}
	return fmt.Errorf("cenario invalido:\n  - %s", strings.Join(problemas, "\n  - "))
}
