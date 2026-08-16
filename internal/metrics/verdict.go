package metrics

// Verdict lives in the document because the HTML report and the run comparison
// are generated from the JSON, and without it the file would not say whether
// the run passed.
type Verdict struct {
	Passed      bool         `json:"passou"`
	Evaluations []Evaluation `json:"avaliacoes"`
	Undeclared  []string     `json:"criterios_nao_declarados"`
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
	// Untrustworthy marks a rule that was measured but cannot judge: the
	// comparison behind it has a caveat that explains the difference on its own.
	Untrustworthy bool `json:"comparacao_nao_confiavel"`
}
