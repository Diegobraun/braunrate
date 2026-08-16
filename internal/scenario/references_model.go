package scenario

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// CheckReferences applies the undeclared-variable rule to the built scenario
// instead of to the YAML text, which is what makes it reach the scenario
// written in Go. The rule and the message are the same ones the YAML path uses;
// what the YAML path adds is the line and the column (ADR 0002).
//
// Without this a Go scenario accepted a ${nome} that resolves from nowhere,
// while the same scenario in YAML was refused — one rule with two answers
// depending on which public wrote it.
func CheckReferences(spec *Spec) error {
	known := knownVariables(*spec)
	missing := map[string]bool{}

	for name, value := range spec.Vars {
		if err := checkText(known, value, fmt.Sprintf("variavel %q", name), missing); err != nil {
			return err
		}
	}
	for _, step := range spec.Steps {
		where := fmt.Sprintf("passo %q", step.Name)
		for _, text := range textsOf(step.Config) {
			if err := checkText(known, text, where, missing); err != nil {
				return err
			}
		}
		for _, assertion := range step.Assertions {
			if err := checkText(known, assertion.Value, where, missing); err != nil {
				return err
			}
		}
	}

	for name := range missing {
		spec.MissingEnvironment = append(spec.MissingEnvironment, name)
	}
	sort.Strings(spec.MissingEnvironment)
	spec.MissingEnvironment = unique(spec.MissingEnvironment)
	return nil
}

func checkText(known variableScope, text, where string, missing map[string]bool) error {
	for _, used := range referencesIn(text) {
		if err := known.resolve(used, nil); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if !used.hasDefault && environmentName.MatchString(used.name) && !defined(used.name) {
			missing[used.name] = true
		}
	}
	return nil
}

// The step configuration belongs to the protocol, and the engine never reads
// its fields by name. Walking it by reflection is what keeps a protocol added
// later covered without writing anything here — the same reason the observed
// variety is instrumented in one place only (ADR 0007).
func textsOf(value any) []string {
	var found []string
	collectTexts(reflect.ValueOf(value), &found)
	return found
}

func collectTexts(value reflect.Value, into *[]string) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			collectTexts(value.Elem(), into)
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if value.Type().Field(index).IsExported() {
				collectTexts(value.Field(index), into)
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			*into = append(*into, string(value.Bytes()))
			return
		}
		for index := 0; index < value.Len(); index++ {
			collectTexts(value.Index(index), into)
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			collectTexts(key, into)
			collectTexts(value.MapIndex(key), into)
		}
	case reflect.String:
		*into = append(*into, value.String())
	}
}

func unique(names []string) []string {
	if len(names) < 2 {
		return names
	}
	kept := names[:1]
	for _, name := range names[1:] {
		if name != kept[len(kept)-1] {
			kept = append(kept, name)
		}
	}
	return kept
}

// ReferencedFields lists, per data source, the fields the scenario reads from
// it. A CSV declares its columns in the file, so the check that the column
// exists can only happen once the file is open — and until it did, a column
// that was not there interpolated to nothing and the request went out with a
// blank in the middle of the path.
func ReferencedFields(spec Spec) map[string][]string {
	bySource := map[string]map[string]bool{}
	collect := func(text string) {
		for _, used := range referencesIn(text) {
			source, field, dotted := strings.Cut(used.name, ".")
			if !dotted || field == "" {
				continue
			}
			if bySource[source] == nil {
				bySource[source] = map[string]bool{}
			}
			bySource[source][field] = true
		}
	}

	for _, value := range spec.Vars {
		collect(value)
	}
	collect(spec.Target)
	for _, step := range spec.Steps {
		for _, text := range textsOf(step.Config) {
			collect(text)
		}
		for _, assertion := range step.Assertions {
			collect(assertion.Target)
			collect(assertion.Value)
		}
	}

	fields := map[string][]string{}
	for source, names := range bySource {
		listed := make([]string, 0, len(names))
		for name := range names {
			listed = append(listed, name)
		}
		sort.Strings(listed)
		fields[source] = listed
	}
	return fields
}
