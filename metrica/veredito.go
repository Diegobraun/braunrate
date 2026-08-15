package metrica

// O veredito mora no documento porque o relatorio HTML e a comparacao entre
// execucoes sao gerados a partir do JSON, e sem ele o arquivo nao diria se a
// execucao passou.
type Veredito struct {
	Passou     bool        `json:"passou"`
	Avaliacoes []Avaliacao `json:"avaliacoes"`
	Frase      string      `json:"frase"`
}

type Avaliacao struct {
	Passo    string  `json:"passo"`
	Metrica  string  `json:"metrica"`
	Regra    string  `json:"regra"`
	Obtido   float64 `json:"obtido"`
	Limite   float64 `json:"limite"`
	Unidade  string  `json:"unidade"`
	Passou   bool    `json:"passou"`
	Frase    string  `json:"frase"`
	SemDados bool    `json:"sem_dados"`
}
