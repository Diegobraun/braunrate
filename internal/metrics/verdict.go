package metrics

// Verdict lives in the document because the HTML report and the run comparison
// are generated from the JSON, and without it the file would not say whether
// the run passed.
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
