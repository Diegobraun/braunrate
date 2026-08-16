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
		return nil, fmt.Errorf("nao consegui abrir o arquivo de dados %q: %w", source.File, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	lines, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("arquivo de dados %q invalido: %w", source.File, err)
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("arquivo de dados %q precisa de cabecalho e pelo menos uma linha", source.File)
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

func (f *csvSource) Name() string { return f.name }

func (f *csvSource) Available() map[string]int64 {
	available := map[string]int64{}
	for position, column := range f.columns {
		distinct := map[string]struct{}{}
		for _, record := range f.records {
			if position < len(record) {
				distinct[record[position]] = struct{}{}
			}
		}
		available[f.name+"."+column] = int64(len(distinct))
	}
	return available
}

func (f *csvSource) Exhausted() bool { return f.exhausted.Load() }

func (f *csvSource) Next(virtualUser int64) (map[string]string, error) {
	total := int64(len(f.records))
	var index int64

	switch f.consume {
	case scenario.ConsumeRandom:
		f.mu.Lock()
		index = f.random.Int63n(total)
		f.mu.Unlock()
	case scenario.ConsumeUniquePerUser:
		index = virtualUser % total
	case scenario.ConsumeSequential:
		index = f.position.Add(1) - 1
		if index >= total {
			f.exhausted.Store(true)
			return nil, fmt.Errorf("os dados de %q acabaram na linha %d; use consumo circular para repetir do inicio", f.name, total)
		}
	default:
		index = (f.position.Add(1) - 1) % total
	}

	record := f.records[index]
	values := make(map[string]string, len(f.columns))
	for position, column := range f.columns {
		if position < len(record) {
			values[f.name+"."+column] = record[position]
		}
	}
	return values, nil
}

type syntheticSource struct {
	name        string
	fields      map[string]string
	sortedNames []string
	seed        int64
	sequence    atomic.Int64
}

func newSyntheticSource(source scenario.DataSource) (Source, error) {
	if len(source.Fields) == 0 {
		return nil, fmt.Errorf("a fonte de dados %q nao tem arquivo nem campos para gerar", source.Name)
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

func (f *syntheticSource) Name() string { return f.name }

func (f *syntheticSource) Exhausted() bool { return false }

// Synthetic data has no closed list of values: all that is known is that
// always generating the same value would be a defect.
func (f *syntheticSource) Available() map[string]int64 {
	available := map[string]int64{}
	for _, field := range f.sortedNames {
		available[f.name+"."+field] = -1
	}
	return available
}

// The seed goes into the environment block and fields are generated in a fixed
// order: without both the run is not reproducible, and a non-reproducible
// result is useless for comparing two runs.
func (f *syntheticSource) Next(virtualUser int64) (map[string]string, error) {
	sequence := f.sequence.Add(1)
	random := rand.New(rand.NewSource(f.seed + sequence))
	values := make(map[string]string, len(f.fields))
	for _, field := range f.sortedNames {
		value, err := generate(f.fields[field], random, sequence)
		if err != nil {
			return nil, fmt.Errorf("campo %q da fonte %q: %w", field, f.name, err)
		}
		values[f.name+"."+field] = value
	}
	return values, nil
}

var names = []string{"ana", "bruno", "carla", "diego", "elisa", "fabio", "gabriela", "heitor", "isabel", "joao"}
var lastNames = []string{"souza", "lima", "braun", "costa", "martins", "azevedo", "ferreira", "rocha"}

func generate(expression string, random *rand.Rand, sequence int64) (string, error) {
	name, args := splitGenerator(expression)
	switch name {
	case "uuid":
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", random.Uint32(), random.Intn(0xffff),
			random.Intn(0xffff), random.Intn(0xffff), random.Int63n(0xffffffffffff)), nil
	case "sequencia":
		return strconv.FormatInt(sequence, 10), nil
	case "numero":
		minimum, maximum := 0.0, 100.0
		if len(args) == 2 {
			var err error
			if minimum, err = strconv.ParseFloat(args[0], 64); err != nil {
				return "", fmt.Errorf("primeiro argumento de numero() invalido: %q", args[0])
			}
			if maximum, err = strconv.ParseFloat(args[1], 64); err != nil {
				return "", fmt.Errorf("segundo argumento de numero() invalido: %q", args[1])
			}
		}
		if maximum <= minimum {
			return "", fmt.Errorf("numero(%v,%v) precisa de maximo maior que minimo", minimum, maximum)
		}
		return strconv.FormatFloat(minimum+random.Float64()*(maximum-minimum), 'f', 2, 64), nil
	case "inteiro":
		minimum, maximum := int64(0), int64(100)
		if len(args) == 2 {
			minimum, _ = strconv.ParseInt(args[0], 10, 64)
			maximum, _ = strconv.ParseInt(args[1], 10, 64)
		}
		if maximum <= minimum {
			return "", fmt.Errorf("inteiro(%d,%d) precisa de maximo maior que minimo", minimum, maximum)
		}
		return strconv.FormatInt(minimum+random.Int63n(maximum-minimum), 10), nil
	case "nome":
		return names[random.Intn(len(names))] + " " + lastNames[random.Intn(len(lastNames))], nil
	case "email":
		return fmt.Sprintf("%s.%d@exemplo.com", names[random.Intn(len(names))], sequence), nil
	case "texto":
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
	default:
		return "", fmt.Errorf("gerador desconhecido: %q (use uuid, sequencia, numero, inteiro, nome, email ou texto)", name)
	}
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
