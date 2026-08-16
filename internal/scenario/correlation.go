package scenario

import (
	"time"
)

type CaptureOrigin string

const (
	CaptureJSON   CaptureOrigin = "json"
	CaptureHeader CaptureOrigin = "header"
	CaptureCookie CaptureOrigin = "cookie"
	CaptureRegex  CaptureOrigin = "regex"
	CaptureBody   CaptureOrigin = "body"
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
	AssertBodyContains AssertionKind = "bodyContains"
	AssertJSON         AssertionKind = "json"
	AssertRegex        AssertionKind = "regex"
	AssertHeader       AssertionKind = "header"
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
	OpContains       Operator = "contains"
	OpExists         Operator = "exists"
)

type AuthKind string

const (
	AuthToken  AuthKind = "token"
	AuthBasic  AuthKind = "basic"
	AuthHeader AuthKind = "header"
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
	ConsumeSequential    ConsumePolicy = "sequential"
	ConsumeRandom        ConsumePolicy = "random"
	ConsumeCircular      ConsumePolicy = "circular"
	ConsumeUniquePerUser ConsumePolicy = "uniquePerUser"
)

type DataSource struct {
	Name    string
	File    string
	Consume ConsumePolicy
	Seed    int64
	// Name of the environment variable the seed came from, empty when it was
	// written in the file. Reproducing a run means knowing which seed ran and
	// where it came from, and there is no way to know that later.
	SeedFrom string
	Fields   map[string]Generator
	Records  int
	Line     int
}

// PerUse is off by default: the same generated value has to hold for the whole
// iteration, which is what an idempotency key needs — the same transactionId in
// two requests, a new one on the next iteration. Wanting a new value at every
// substitution is the rare case, so it is declared.
type Generator struct {
	Recipe string
	Format string
	PerUse bool
}

func ParseGenerator(recipe string) Generator { return Generator{Recipe: recipe} }

func (dataSource DataSource) Synthetic() bool {
	return dataSource.File == ""
}

type SLOScope string

const (
	ScopeStep       SLOScope = "step"
	ScopeOverall    SLOScope = "global"
	ScopeJourney    SLOScope = "journey"
	ScopeRegression SLOScope = "regression"
)

type SLORule struct {
	Scope    SLOScope
	Step     string
	Metric  string
	Operator Operator
	Limit    float64
	Unit     string
	Text     string
	Line     int
}

func (sloRule SLORule) Overall() bool { return sloRule.Scope == ScopeOverall }
