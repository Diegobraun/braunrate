package metrics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BodyShape reduces a request body to the fields it carries and their types,
// dropping the values. Counting distinct bodies would count distinct ids, which
// says nothing: what the target branches on is the shape — a field that came
// empty, an optional block that never appeared, a list that was always of one
// item. That is the second half of what ADR 0007 left pending.
func BodyShape(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "vazio"
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		// Not JSON is a shape too, and the only thing that separates one text
		// body from another without reading it is whether it is there.
		return "texto"
	}
	var fields []string
	describeShape("", decoded, &fields)
	sort.Strings(fields)
	return strings.Join(fields, ", ")
}

func describeShape(path string, value any, into *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			// A body the scenario wrote as {} is a declaration, not an accident,
			// and it has no field name to blame. Naming it apart from an empty
			// field is what keeps the warning about the accident.
			if path == "" {
				*into = append(*into, "corpo sem campos")
				return
			}
			*into = append(*into, path+": objeto vazio")
			return
		}
		for name, field := range typed {
			describeShape(fieldPath(path, name), field, into)
		}
	case []any:
		// The count of items is bucketed on purpose: a list of 3 and a list of 4
		// are the same code path, a list of 0 is not.
		switch len(typed) {
		case 0:
			*into = append(*into, path+"[]: vazia")
		default:
			describeShape(path+"[]", typed[0], into)
		}
	case string:
		if typed == "" {
			*into = append(*into, path+": vazio")
			return
		}
		*into = append(*into, path+": texto")
	case float64:
		*into = append(*into, path+": numero")
	case bool:
		*into = append(*into, path+": booleano")
	case nil:
		*into = append(*into, path+": nulo")
	default:
		*into = append(*into, fmt.Sprintf("%s: %T", path, typed))
	}
}

func fieldPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// BodyShapeName is the variable name a body shape is counted under. The prefix
// is what tells the report it is looking at a shape and not at a value.
const BodyShapeName = "corpo."

func bodyShape(name string) bool { return strings.HasPrefix(name, BodyShapeName) }

// Kept few on purpose: the reader compares two or three shapes to see what
// changed between them, and a list of forty is a list nobody reads.
const shapesKept = 5

func shapesSeen(counter *varietyCounter) []string {
	shapes := make([]string, 0, len(counter.seen))
	for shape := range counter.seen {
		shapes = append(shapes, shape)
	}
	sort.Strings(shapes)
	if len(shapes) > shapesKept {
		shapes = shapes[:shapesKept]
	}
	return shapes
}

// A field that arrived empty is the shape difference that is almost never on
// purpose: it is what an interpolation that resolved to nothing looks like on
// the wire.
// Only a named field counts. A body declared as {} in the scenario has no field
// to blame, and warning about it taught the reader to skip the block where the
// real warnings live — the same noise ADR 0007 refuses for a source that has a
// single value.
func emptyField(shape string) bool {
	for _, entry := range strings.Split(shape, ", ") {
		name, kind, separated := strings.Cut(entry, ": ")
		if !separated || name == "" {
			continue
		}
		switch kind {
		case "vazio", "nulo", "objeto vazio", "vazia":
			return true
		}
	}
	return false
}

// A field that went out empty is a reading warning, never an invalid result:
// there are scenarios that mean to send a blank field, and the tool cannot tell
// those from an interpolation that resolved to nothing. What it can do is say
// it happened, which is what was missing when a body left with an empty value
// and every number in the report still looked healthy.
func emptyFieldWarnings(variety Variety) []Warning {
	var empty []string
	for _, shape := range variety.Shapes {
		if emptyField(shape) {
			empty = append(empty, shape)
		}
	}
	if len(empty) == 0 {
		return nil
	}
	step := strings.TrimPrefix(variety.Name, BodyShapeName)
	return []Warning{{
		Kind:     "corpo_com_campo_vazio",
		Severity: SeverityMedium,
		Message:  fmt.Sprintf("o corpo de %q saiu com campo vazio; se isso nao for proposital, o alvo exercitou um caminho que producao nao ve", step),
		Evidence: strings.Join(empty, " | "),
	}}
}

// Notable says whether the line is worth a sentence in the report. Every step
// with a body has exactly one shape in the normal case, and printing that for
// each of them would bury the lines that do say something.
func (variety Variety) Notable() bool {
	if !bodyShape(variety.Name) {
		return true
	}
	if variety.Distinct > 1 || variety.Capped {
		return true
	}
	for _, shape := range variety.Shapes {
		if emptyField(shape) {
			return true
		}
	}
	return false
}
