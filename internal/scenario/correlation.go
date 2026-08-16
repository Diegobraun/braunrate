package scenario

import (
	"time"
)

type CaptureOrigin string

const (
	CaptureJSON   CaptureOrigin = "json"
	CaptureHeader CaptureOrigin = "cabecalho"
	CaptureRegex  CaptureOrigin = "regex"
	CaptureBody   CaptureOrigin = "corpo"
	CaptureStatus CaptureOrigin = "status"
)

type Capture struct {
	Variable   string
	Origin     CaptureOrigin
	Expression string
	Required   bool
	Default    string
	Line       int
}

type AssertionKind string

const (
	AssertStatus       AssertionKind = "status"
	AssertBodyContains AssertionKind = "corpo_contem"
	AssertJSON         AssertionKind = "json"
	AssertRegex        AssertionKind = "regex"
	AssertHeader       AssertionKind = "cabecalho"
)

type Assertion struct {
	Kind        AssertionKind
	Target      string
	Operator    Operator
	Value       string
	Description string
	Line        int
}

type Operator string

const (
	OpEqual          Operator = "=="
	OpNotEqual       Operator = "!="
	OpLess           Operator = "<"
	OpLessOrEqual    Operator = "<="
	OpGreater        Operator = ">"
	OpGreaterOrEqual Operator = ">="
	OpContains       Operator = "contem"
	OpExists         Operator = "existe"
)

type AuthKind string

const (
	AuthToken  AuthKind = "token"
	AuthBasic  AuthKind = "basica"
	AuthHeader AuthKind = "cabecalho"
)

type Auth struct {
	Kind         AuthKind
	Obtain       *Step
	RefreshAfter time.Duration
	Header       string
	User         string
	Password     string
	Line         int
}

type ConsumePolicy string

const (
	ConsumeSequential    ConsumePolicy = "sequencial"
	ConsumeRandom        ConsumePolicy = "aleatorio"
	ConsumeCircular      ConsumePolicy = "circular"
	ConsumeUniquePerUser ConsumePolicy = "unico_por_usuario"
)

type DataSource struct {
	Name      string
	File      string
	Consume   ConsumePolicy
	Seed      int64
	Fields    map[string]string
	Registros int
	Line      int
}

func (f DataSource) Synthetic() bool {
	return f.File == ""
}

type SLORule struct {
	Step     string
	Overall  bool
	Metrica  string
	Operator Operator
	Limit    float64
	Unit     string
	Text     string
	Line     int
}
