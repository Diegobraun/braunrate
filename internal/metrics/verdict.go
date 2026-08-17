package metrics

// Verdict lives in the document because the HTML report and the run comparison
// are generated from the JSON, and without it the file would not say whether
// the run passed.
type Verdict struct {
	Passed      bool         `json:"passed"`
	Evaluations []Evaluation `json:"evaluations"`
	Undeclared  []string     `json:"undeclaredCriteria"`
	Sentence    string       `json:"sentence"`
}

type Evaluation struct {
	Step     string  `json:"step"`
	Metric   string  `json:"metric"`
	Rule     string  `json:"rule"`
	Obtained float64 `json:"obtained"`
	Limit    float64 `json:"limit"`
	Unit     string  `json:"unit"`
	Passed   bool    `json:"passed"`
	Sentence string  `json:"sentence"`
	NoData   bool    `json:"noData"`
	// Untrustworthy marks a rule that was measured but cannot judge: the
	// comparison behind it has a caveat that explains the difference on its own.
	Untrustworthy bool `json:"comparisonNotTrustworthy"`
}
