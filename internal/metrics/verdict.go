package metrics

// O veredito mora no documento porque o relatorio HTML e a comparacao entre
// execucoes sao gerados a partir do JSON, e sem ele o arquivo nao diria se a
// execucao passou.
type Verdict struct {
	Passed      bool         `json:"passou"`
	Evaluations []Evaluation `json:"avaliacoes"`
	Sentence    string       `json:"frase"`
}

type Evaluation struct {
	Step     string  `json:"passo"`
	Metrica  string  `json:"metrica"`
	Rule     string  `json:"regra"`
	Obtained float64 `json:"obtido"`
	Limit    float64 `json:"limite"`
	Unit     string  `json:"unidade"`
	Passed   bool    `json:"passou"`
	Sentence string  `json:"frase"`
	NoData   bool    `json:"sem_dados"`
}
