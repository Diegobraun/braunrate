package runtime

import (
	"os"
	"regexp"
	"sync"
)

var varPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.-]*)(?::-([^}]*))?\}`)

type Values struct {
	VirtualUser int64
	Iteration   int64
	mu          sync.RWMutex
	values      map[string]string
	uses        map[string]string
	perUse      map[string]func() (string, error)
}

func (values *Values) generatorFor(name string) (func() (string, error), bool) {
	values.mu.RLock()
	defer values.mu.RUnlock()
	generate, found := values.perUse[name]
	return generate, found
}

func New(virtualUser, iteration int64, base map[string]string) *Values {
	values := make(map[string]string, len(base)+2)
	for name, value := range base {
		values[name] = value
	}
	return &Values{VirtualUser: virtualUser, Iteration: iteration, values: values, uses: map[string]string{}}
}

func (values *Values) Set(name, value string) {
	values.mu.Lock()
	values.values[name] = value
	values.mu.Unlock()
}

func (values *Values) SetAll(incoming map[string]string) {
	values.mu.Lock()
	for name, value := range incoming {
		values.values[name] = value
	}
	values.mu.Unlock()
}

func (values *Values) Value(name string) (string, bool) {
	values.mu.RLock()
	defer values.mu.RUnlock()
	value, exists := values.values[name]
	return value, exists
}

func (values *Values) Values() map[string]string {
	values.mu.RLock()
	defer values.mu.RUnlock()
	clone := make(map[string]string, len(values.values))
	for name, value := range values.values {
		clone[name] = value
	}
	return clone
}

// Resolve happens at run time, not at load time: without that, a value
// captured in one step never reaches the next one.
func (values *Values) SetPerUse(name string, generate func() (string, error)) {
	values.mu.Lock()
	defer values.mu.Unlock()
	if values.perUse == nil {
		values.perUse = map[string]func() (string, error){}
	}
	values.perUse[name] = generate
}

func (values *Values) Resolve(text string) string {
	if text == "" {
		return text
	}
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		name, fallback := parts[1], parts[2]
		if generate, perUse := values.generatorFor(name); perUse {
			if value, err := generate(); err == nil {
				values.noteUse(name, value)
				return value
			}
		}
		if value, exists := values.Value(name); exists {
			values.noteUse(name, value)
			return value
		}
		if value, definida := os.LookupEnv(name); definida {
			return value
		}
		return fallback
	})
}

// Every substitution is noted because observed variety comes from here: path,
// body, header, GraphQL variable and message key all pass through, so a single
// point covers the whole scenario.
func (values *Values) noteUse(name, value string) {
	values.mu.Lock()
	values.uses[name] = value
	values.mu.Unlock()
}

func (values *Values) Uses() map[string]string {
	values.mu.RLock()
	defer values.mu.RUnlock()
	if len(values.uses) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values.uses))
	for name, value := range values.uses {
		clone[name] = value
	}
	return clone
}

func Unresolved(text string) []string {
	var pending []string
	for _, occurrence := range varPattern.FindAllStringSubmatch(text, -1) {
		pending = append(pending, occurrence[1])
	}
	return pending
}
