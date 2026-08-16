package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Diegobraun/braunrate/internal/data"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

// A column that is not in the CSV used to interpolate to nothing: the request
// left with a blank in the middle of the path and only the target's 404 said
// anything. The columns are only knowable with the file open, which is here.
func checkDataFields(spec scenario.Spec, sources []data.Source) error {
	available := map[string]bool{}
	for _, source := range sources {
		for name := range source.Available() {
			available[name] = true
		}
	}

	for sourceName, fields := range scenario.ReferencedFields(spec) {
		known := columnsOf(available, sourceName)
		if len(known) == 0 {
			continue
		}
		for _, field := range fields {
			if available[sourceName+"."+field] {
				continue
			}
			return fmt.Errorf("a fonte de dados %q não tem o campo %q.\n    campos disponíveis: %s",
				sourceName, field, strings.Join(known, ", "))
		}
	}
	return nil
}

func columnsOf(available map[string]bool, source string) []string {
	var columns []string
	for name := range available {
		if declared, column, dotted := strings.Cut(name, "."); dotted && declared == source {
			columns = append(columns, column)
		}
	}
	sort.Strings(columns)
	return columns
}
