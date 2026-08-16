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
}

func New(virtualUser, iteration int64, base map[string]string) *Values {
	values := make(map[string]string, len(base)+2)
	for name, value := range base {
		values[name] = value
	}
	return &Values{VirtualUser: virtualUser, Iteration: iteration, values: values, uses: map[string]string{}}
}

func (c *Values) Set(name, value string) {
	c.mu.Lock()
	c.values[name] = value
	c.mu.Unlock()
}

func (c *Values) SetAll(values map[string]string) {
	c.mu.Lock()
	for name, value := range values {
		c.values[name] = value
	}
	c.mu.Unlock()
}

func (c *Values) Value(name string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists := c.values[name]
	return value, exists
}

func (c *Values) Values() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clone := make(map[string]string, len(c.values))
	for name, value := range c.values {
		clone[name] = value
	}
	return clone
}

// Resolve happens at run time, not at load time: without that, a value
// captured in one step never reaches the next one.
func (c *Values) Resolve(text string) string {
	if text == "" {
		return text
	}
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		name, fallback := parts[1], parts[2]
		if value, exists := c.Value(name); exists {
			c.noteUse(name, value)
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
func (c *Values) noteUse(name, value string) {
	c.mu.Lock()
	c.uses[name] = value
	c.mu.Unlock()
}

func (c *Values) Uses() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.uses) == 0 {
		return nil
	}
	clone := make(map[string]string, len(c.uses))
	for name, value := range c.uses {
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
