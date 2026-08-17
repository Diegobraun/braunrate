package data

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Source interface {
	Name() string
	Next(virtualUser int64) (map[string]string, error)
	Exhausted() bool
	// How many distinct values each variable can take. It is what lets the
	// report tell a defect from a scenario that declared a single value. A
	// negative value means unknown.
	Available() map[string]int64
}

func Open(source scenario.DataSource, root string) (Source, error) {
	if source.Synthetic() {
		return newSyntheticSource(source)
	}
	return openCSV(source, root)
}

type csvSource struct {
	name      string
	columns   []string
	records   [][]string
	consume   scenario.ConsumePolicy
	position  atomic.Int64
	exhausted atomic.Bool
	random    *rand.Rand
	mu        sync.Mutex
}

func openCSV(source scenario.DataSource, root string) (Source, error) {
	path := source.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open the data file %q: %w", source.File, err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	lines, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid data file %q: %w", source.File, err)
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("the data file %q needs a header row and at least one line", source.File)
	}

	seed := source.Seed
	if seed == 0 {
		seed = 1
	}
	return &csvSource{
		name:    source.Name,
		columns: lines[0],
		records: lines[1:],
		consume: source.Consume,
		random:  rand.New(rand.NewSource(seed)),
	}, nil
}

func (csvSource *csvSource) Name() string { return csvSource.name }

func (csvSource *csvSource) Available() map[string]int64 {
	available := map[string]int64{}
	for position, column := range csvSource.columns {
		distinct := map[string]struct{}{}
		for _, record := range csvSource.records {
			if position < len(record) {
				distinct[record[position]] = struct{}{}
			}
		}
		available[csvSource.name+"."+column] = int64(len(distinct))
	}
	return available
}

func (csvSource *csvSource) Exhausted() bool { return csvSource.exhausted.Load() }

func (csvSource *csvSource) Next(virtualUser int64) (map[string]string, error) {
	total := int64(len(csvSource.records))
	var index int64

	switch csvSource.consume {
	case scenario.ConsumeRandom:
		csvSource.mu.Lock()
		index = csvSource.random.Int63n(total)
		csvSource.mu.Unlock()
	case scenario.ConsumeUniquePerUser:
		index = virtualUser % total
	case scenario.ConsumeSequential:
		index = csvSource.position.Add(1) - 1
		if index >= total {
			csvSource.exhausted.Store(true)
			return nil, fmt.Errorf("the data of %q ran out at line %d; use circular consume to start over", csvSource.name, total)
		}
	default:
		index = (csvSource.position.Add(1) - 1) % total
	}

	record := csvSource.records[index]
	values := make(map[string]string, len(csvSource.columns))
	for position, column := range csvSource.columns {
		if position < len(record) {
			values[csvSource.name+"."+column] = record[position]
		}
	}
	return values, nil
}

type syntheticSource struct {
	name        string
	fields      map[string]scenario.Generator
	sortedNames []string
	seed        int64
	sequence    atomic.Int64
}

func newSyntheticSource(source scenario.DataSource) (Source, error) {
	if len(source.Fields) == 0 {
		return nil, fmt.Errorf("the data source %q has neither a file nor fields to generate", source.Name)
	}
	seed := source.Seed
	if seed == 0 {
		seed = 1
	}
	sortedNames := make([]string, 0, len(source.Fields))
	for field := range source.Fields {
		sortedNames = append(sortedNames, field)
	}
	sort.Strings(sortedNames)
	return &syntheticSource{name: source.Name, fields: source.Fields,
		sortedNames: sortedNames, seed: seed}, nil
}

func (syntheticSource *syntheticSource) Name() string { return syntheticSource.name }

func (syntheticSource *syntheticSource) Exhausted() bool { return false }

// Synthetic data has no closed list of values: all that is known is that
// always generating the same value would be a defect.
func (syntheticSource *syntheticSource) Available() map[string]int64 {
	available := map[string]int64{}
	for _, field := range syntheticSource.sortedNames {
		available[syntheticSource.name+"."+field] = -1
	}
	return available
}

// The seed goes into the environment block and fields are generated in a fixed
// order: without both the run is not reproducible, and a non-reproducible
// result is useless for comparing two runs.
// Semente e sequencia entram misturadas, e nao somadas. Somar faz a semente 1 na
// sequencia 2 e a semente 2 na sequencia 1 caírem na mesma fonte: duas execucoes
// com sementes diferentes percorriam os mesmos valores deslocados de um, e a
// promessa de variar a semente para explorar dados diferentes valia muito menos
// do que o relatorio dizia. O misturador separa entradas vizinhas.
func streamSeed(seed, sequence int64) int64 {
	mixed := uint64(seed)*0x9E3779B97F4A7C15 + uint64(sequence)
	mixed ^= mixed >> 30
	mixed *= 0xBF58476D1CE4E5B9
	mixed ^= mixed >> 27
	mixed *= 0x94D049BB133111EB
	mixed ^= mixed >> 31
	return int64(mixed)
}

func (syntheticSource *syntheticSource) Next(virtualUser int64) (map[string]string, error) {
	sequence := syntheticSource.sequence.Add(1)
	random := rand.New(rand.NewSource(streamSeed(syntheticSource.seed, sequence)))
	values := make(map[string]string, len(syntheticSource.fields))
	for _, field := range syntheticSource.sortedNames {
		value, err := generate(syntheticSource.fields[field], random, sequence)
		if err != nil {
			return nil, fmt.Errorf("field %q of source %q: %w", field, syntheticSource.name, err)
		}
		values[syntheticSource.name+"."+field] = value
	}
	return values, nil
}

// PerUse fields are handed over as a function instead of a value: substitution
// calls it at every occurrence, so the report counts each one as a distinct use.
func (syntheticSource *syntheticSource) PerUse() map[string]func() (string, error) {
	perUse := map[string]func() (string, error){}
	for _, field := range syntheticSource.sortedNames {
		generator := syntheticSource.fields[field]
		if !generator.PerUse {
			continue
		}
		perUse[syntheticSource.name+"."+field] = func() (string, error) {
			sequence := syntheticSource.sequence.Add(1)
			return generate(generator, rand.New(rand.NewSource(streamSeed(syntheticSource.seed, sequence))), sequence)
		}
	}
	return perUse
}

var names = []string{"ana", "bruno", "carla", "diego", "elisa", "fabio", "gabriela", "heitor", "isabel", "joao"}
var lastNames = []string{"souza", "lima", "braun", "costa", "martins", "azevedo", "ferreira", "rocha"}

func generate(generator scenario.Generator, random *rand.Rand, sequence int64) (string, error) {
	name, args := splitGenerator(generator.Recipe)
	switch name {
	case "uuid":
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", random.Uint32(), random.Intn(0xffff),
			random.Intn(0xffff), random.Intn(0xffff), random.Int63n(0xffffffffffff)), nil
	case "sequence":
		return strconv.FormatInt(sequence, 10), nil
	case "number":
		minimum, maximum := 0.0, 100.0
		if len(args) == 2 {
			var err error
			if minimum, err = strconv.ParseFloat(args[0], 64); err != nil {
				return "", fmt.Errorf("invalid first argument of number(): %q", args[0])
			}
			if maximum, err = strconv.ParseFloat(args[1], 64); err != nil {
				return "", fmt.Errorf("invalid second argument of number(): %q", args[1])
			}
		}
		if maximum <= minimum {
			return "", fmt.Errorf("number(%v,%v) needs a maximum greater than the minimum", minimum, maximum)
		}
		return strconv.FormatFloat(minimum+random.Float64()*(maximum-minimum), 'f', 2, 64), nil
	case "integer":
		minimum, maximum := int64(0), int64(100)
		if len(args) == 2 {
			minimum, _ = strconv.ParseInt(args[0], 10, 64)
			maximum, _ = strconv.ParseInt(args[1], 10, 64)
		}
		if maximum <= minimum {
			return "", fmt.Errorf("integer(%d,%d) needs a maximum greater than the minimum", minimum, maximum)
		}
		return strconv.FormatInt(minimum+random.Int63n(maximum-minimum), 10), nil
	case "name":
		return names[random.Intn(len(names))] + " " + lastNames[random.Intn(len(lastNames))], nil
	case "email":
		return fmt.Sprintf("%s.%d@exemplo.com", names[random.Intn(len(names))], sequence), nil
	case "text":
		size := 12
		if len(args) == 1 {
			size, _ = strconv.Atoi(args[0])
		}
		letters := "abcdefghijklmnopqrstuvwxyz"
		builder := strings.Builder{}
		for i := 0; i < size; i++ {
			builder.WriteByte(letters[random.Intn(len(letters))])
		}
		return builder.String(), nil
	case "pattern":
		// The pattern arrives in two shapes, and only one of them was read:
		// `{ type: pattern, format: "BR-######" }` fills Format, and
		// `pattern(BR-######)` fills the argument. Reading only Format made the
		// second shape produce an empty string with no complaint — the request
		// went out with a blank field, which is the failure this tool exists to
		// catch.
		format := generator.Format
		if len(args) > 0 {
			format = strings.Join(args, ",")
		}
		if strings.TrimSpace(format) == "" {
			return "", fmt.Errorf(`pattern with no format: say the shape of the value, for example pattern(BR-######) or { type: pattern, format: "BR-######" }
    # becomes a digit, @ becomes an uppercase letter; everything else comes out as it is`)
		}
		return fromPattern(format, random), nil
	case "cpf":
		return brazilianDocument(random, cpfLength, cpfWeights), nil
	case "cnpj":
		return brazilianDocument(random, cnpjLength, cnpjWeights), nil
	default:
		return "", fmt.Errorf("unknown generator: %q\n"+
			"    available: uuid, sequence, number, integer, name, email, text, pattern, cpf, cnpj", name)
	}
}

// fromPattern keeps two placeholders only. More would need a grammar, and the
// point here is filling a format the target validates, not a template engine.
func fromPattern(format string, random *rand.Rand) string {
	builder := strings.Builder{}
	for _, character := range format {
		switch character {
		case '#':
			builder.WriteByte(byte('0' + random.Intn(10)))
		case '@':
			builder.WriteByte(byte('A' + random.Intn(26)))
		default:
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

// An invalid CPF or CNPJ makes the target refuse every request with a
// validation error, and the run measures the rejection path instead of the
// work. The check digits are the same modulo 11 rule for both, changing only
// the length and the weights.
const (
	cpfLength  = 9
	cnpjLength = 12
)

var (
	cpfWeights  = [][]int{{10, 9, 8, 7, 6, 5, 4, 3, 2}, {11, 10, 9, 8, 7, 6, 5, 4, 3, 2}}
	cnpjWeights = [][]int{{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}, {6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}}
)

func brazilianDocument(random *rand.Rand, length int, weights [][]int) string {
	digits := make([]int, 0, length+2)
	for index := 0; index < length; index++ {
		digits = append(digits, random.Intn(10))
	}
	for _, weight := range weights {
		digits = append(digits, checkDigit(digits, weight))
	}
	builder := strings.Builder{}
	for _, digit := range digits {
		builder.WriteByte(byte('0' + digit))
	}
	return builder.String()
}

func checkDigit(digits []int, weights []int) int {
	total := 0
	for index, weight := range weights {
		total += digits[index] * weight
	}
	if remainder := total % 11; remainder >= 2 {
		return 11 - remainder
	}
	return 0
}

func splitGenerator(expression string) (string, []string) {
	expression = strings.TrimSpace(expression)
	opening := strings.Index(expression, "(")
	if opening < 0 || !strings.HasSuffix(expression, ")") {
		return expression, nil
	}
	name := expression[:opening]
	content := expression[opening+1 : len(expression)-1]
	if strings.TrimSpace(content) == "" {
		return name, nil
	}
	args := strings.Split(content, ",")
	for index := range args {
		args[index] = strings.TrimSpace(args[index])
	}
	return name, args
}
