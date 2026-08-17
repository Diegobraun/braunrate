package importer

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type element struct {
	XMLName    xml.Name
	Attributes []xml.Attr `xml:",any,attr"`
	Children   []*element `xml:",any"`
	Text       string     `xml:",chardata"`
}

func (node *element) attribute(name string) string {
	for _, attribute := range node.Attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

// A JMeter property is always "<stringProp name=...>value</stringProp>",
// including inside nested elementProp, so the lookup walks down the tree.
func (node *element) property(name string) string {
	for _, child := range node.Children {
		if child.attribute("name") == name && len(child.Children) == 0 {
			return strings.TrimSpace(child.Text)
		}
		if value := child.property(name); value != "" {
			return value
		}
	}
	return ""
}

func (node *element) findAll(name string) []*element {
	var findings []*element
	if node.XMLName.Local == name {
		findings = append(findings, node)
	}
	for _, child := range node.Children {
		findings = append(findings, child.findAll(name)...)
	}
	return findings
}

// The translation is partial on purpose and whatever was left out is
// declared: an importer that swallows the whole file silently hands back a
// scenario that measures something else and nobody notices.
var translatedElements = map[string]bool{
	"jmeterTestPlan": true, "hashTree": true, "TestPlan": true, "ThreadGroup": true,
	"HTTPSamplerProxy": true, "HeaderManager": true, "CSVDataSet": true,
	"JSONPostProcessor": true, "RegexExtractor": true, "ResponseAssertion": true,
	"elementProp": true, "collectionProp": true, "stringProp": true, "boolProp": true,
	"intProp": true, "longProp": true, "doubleProp": true, "objProp": true,
}

func FromJMX(content []byte) (Import, error) {
	var root element
	if err := xml.Unmarshal(content, &root); err != nil {
		return Import{}, fmt.Errorf("I could not read the file as a .jmx: %v", err)
	}

	samplers := root.findAll("HTTPSamplerProxy")
	if len(samplers) == 0 {
		return Import{}, fmt.Errorf(`não achei nenhuma requisição HTTP no .jmx.
Hoje o importador traduz HTTPSamplerProxy (requisição HTTP), HeaderManager, CSVDataSet,
extratores JSON e regex, e assercao de resposta. Sampler de JDBC, JMS ou script não entra`)
	}

	script := Script{Name: planName(&root), Steps: nil}
	cabecalhosGlobais := map[string]string{}
	for _, manager := range root.findAll("HeaderManager") {
		for name, value := range headersOf(manager) {
			cabecalhosGlobais[name] = value
		}
	}

	targets := map[string]int{}
	used := map[string]int{}
	for _, sampler := range samplers {
		step, target := stepFromSampler(sampler, cabecalhosGlobais)
		if target != "" {
			targets[target]++
		}
		name := step.Name
		used[name]++
		if used[name] > 1 {
			step.Name = fmt.Sprintf("%s %d", name, used[name])
		}
		script.Steps = append(script.Steps, step)
	}

	script.Target = mostCommonTarget(targets)
	if script.Target == "" {
		script.Target = "http://127.0.0.1:8080"
		script.Warnings = append(script.Warnings,
			"the .jmx declares no domain on the requests (it probably uses a plan variable): change the target before running")
	}
	if len(targets) > 1 {
		script.Warnings = append(script.Warnings,
			fmt.Sprintf("the .jmx points at %d different domains; the most frequent one stayed and the others became fixed paths", len(targets)))
	}

	for _, set := range root.findAll("CSVDataSet") {
		script.Data = append(script.Data, sourceFromCSV(set))
	}

	for _, step := range script.Steps {
		if hasIdentifier(step.Path) {
			script.Warnings = append(script.Warnings, fmt.Sprintf(
				"the step %q has a fixed value in the path (%s): with a single value the target answers from cache and the number comes out optimistic. "+
					"Swap it for ${data.column} and point it at a CSV", step.Name, step.Path))
		}
	}
	script.Warnings = append(script.Warnings, loadWarnings(&root)...)
	script.Warnings = append(script.Warnings, correlationWarnings(&root)...)
	script.Warnings = append(script.Warnings, untranslatedWarnings(&root)...)

	importResult := RenderYAML(script)
	return importResult, nil
}

func planName(root *element) string {
	for _, plan := range root.findAll("TestPlan") {
		if name := strings.TrimSpace(plan.attribute("testname")); name != "" {
			return "Importado de JMeter: " + name
		}
	}
	return "Importado de JMeter"
}

func stepFromSampler(sampler *element, global map[string]string) (ImportedStep, string) {
	method := strings.ToUpper(sampler.property("HTTPSampler.method"))
	if method == "" {
		method = "GET"
	}
	path := sampler.property("HTTPSampler.path")
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "http") {
		path = "/" + path
	}

	step := ImportedStep{
		Method:  method,
		Path:    path,
		Headers: map[string]string{},
		Body:    samplerBody(sampler),
	}
	for name, value := range global {
		step.Headers[name] = value
	}

	name := strings.TrimSpace(sampler.attribute("testname"))
	if name == "" || name == "HTTP Request" {
		name = strings.ToLower(method) + " " + Resource(path)
	}
	step.Name = name

	domain := sampler.property("HTTPSampler.domain")
	if domain == "" {
		return step, ""
	}
	schema := sampler.property("HTTPSampler.protocol")
	if schema == "" {
		schema = "http"
	}
	target := schema + "://" + domain
	if port := sampler.property("HTTPSampler.port"); port != "" && port != "80" && port != "443" {
		target += ":" + port
	}
	return step, target
}

func samplerBody(sampler *element) string {
	for _, arg := range sampler.findAll("elementProp") {
		if arg.attribute("elementType") != "HTTPArgument" {
			continue
		}
		if value := arg.property("Argument.value"); value != "" {
			return value
		}
	}
	return ""
}

func headersOf(manager *element) map[string]string {
	headers := map[string]string{}
	for _, header := range manager.findAll("elementProp") {
		name := header.property("Header.name")
		value := header.property("Header.value")
		if name != "" {
			headers[name] = value
		}
	}
	return headers
}

func sourceFromCSV(set *element) ImportedSource {
	file := set.property("filename")
	name := strings.TrimSpace(set.attribute("testname"))
	if name == "" || strings.Contains(name, " ") {
		name = "dados"
	}
	consume := "circular"
	if set.property("recycle") == "false" {
		consume = "sequencial"
	}
	return ImportedSource{Name: name, File: file, Consume: consume}
}

// Threads are not a rate: in JMeter each thread only sends after the previous
// response arrived, so 50 threads are 50/s if the target answers in 1 s and 5/s
// if it answers in 10 s. Converting silently would import coordinated omission
// along with the scenario.
func loadWarnings(root *element) []string {
	var warnings []string
	for _, group := range root.findAll("ThreadGroup") {
		threads := group.property("ThreadGroup.num_threads")
		if threads == "" {
			continue
		}
		duration := group.property("ThreadGroup.duration")
		description := threads + " threads"
		if ramp := group.property("ThreadGroup.ramp_time"); ramp != "" && ramp != "0" {
			description += ", rampa de " + ramp + "s"
		}
		if duration != "" && duration != "0" {
			description += ", " + duration + "s of duration"
		}
		warnings = append(warnings, fmt.Sprintf(
			"the group %q declares %s: a thread count does not turn into an arrival rate, because a thread only sends after the previous response. "+
				"The 'load' block came out as a guess; swap it for the rate you want to sustain (requests per second)",
			group.attribute("testname"), description))
	}
	return warnings
}

func correlationWarnings(root *element) []string {
	var warnings []string
	for _, extractor := range root.findAll("JSONPostProcessor") {
		variable := extractor.property("JSONPostProcessor.referenceNames")
		path := extractor.property("JSONPostProcessor.jsonPathExprs")
		if variable == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"the .jmx captures %q from %q: declare it on the step that produces the value, as capture: { %s: %s }",
			variable, path, variable, path))
	}
	for _, extractor := range root.findAll("RegexExtractor") {
		variable := extractor.property("RegexExtractor.refname")
		expression := extractor.property("RegexExtractor.regex")
		if variable == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"the .jmx captures %q by regular expression: declare it on the step that produces the value, as capture: { %s: /%s/ }",
			variable, variable, expression))
	}
	for _, assertion := range root.findAll("ResponseAssertion") {
		name := assertion.attribute("testname")
		warnings = append(warnings, fmt.Sprintf(
			"the assertion %q was not translated: every step came out with 'expect: { status: 200 }', adjust whatever was different from that", name))
	}
	return warnings
}

func untranslatedWarnings(root *element) []string {
	ignored := map[string]int{}
	var count func(*element)
	count = func(current *element) {
		name := current.XMLName.Local
		if name != "" && !translatedElements[name] {
			ignored[name]++
			return
		}
		for _, child := range current.Children {
			count(child)
		}
	}
	count(root)

	if len(ignored) == 0 {
		return nil
	}
	names := make([]string, 0, len(ignored))
	total := 0
	for name, count := range ignored {
		names = append(names, name+" ("+strconv.Itoa(count)+")")
		total += count
	}
	sort.Strings(names)
	return []string{fmt.Sprintf(
		"%d element(s) of the .jmx were not translated and stayed out of the scenario: %s. "+
			"Check whether any of them changed what was being measured", total, strings.Join(names, ", "))}
}

func mostCommonTarget(targets map[string]int) string {
	best, greater := "", 0
	for target, count := range targets {
		if count > greater || (count == greater && target < best) {
			best, greater = target, count
		}
	}
	return best
}
