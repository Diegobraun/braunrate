package cenario

import (
	"fmt"
	"net/url"
	"strings"
)

func (c Cenario) Validar() error {
	var problemas []string

	if strings.TrimSpace(c.Nome) == "" {
		problemas = append(problemas, "o cenario precisa de um nome")
	}
	if strings.TrimSpace(c.Alvo) == "" {
		problemas = append(problemas, "o cenario precisa de um alvo")
	} else if endereco, err := url.Parse(c.Alvo); err != nil || endereco.Scheme == "" || endereco.Host == "" {
		problemas = append(problemas, fmt.Sprintf("alvo invalido: %q (use por exemplo https://api.exemplo.com)", c.Alvo))
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
