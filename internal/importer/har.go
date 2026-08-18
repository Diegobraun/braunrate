package importer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type harFile struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harEntry struct {
	ResourceType string      `json:"_resourceType"`
	Request      harRequest  `json:"request"`
	Response     harResponse `json:"response"`
}

type harRequest struct {
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Headers  []harHeader  `json:"headers"`
	PostData *harPostData `json:"postData"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harResponse struct {
	Status  int `json:"status"`
	Content struct {
		MimeType string `json:"mimeType"`
	} `json:"content"`
}

// FromHAR turns a browser's exported .har into a scenario. A HAR is every
// request the page made, assets included; the importer keeps the ones that read
// like API calls and declares how many it left out, so nobody mistakes the
// scenario for the whole capture.
func FromHAR(content []byte) (Import, error) {
	var file harFile
	if err := json.Unmarshal(content, &file); err != nil {
		return Import{}, fmt.Errorf("could not read the file as a .har: %v", err)
	}
	if len(file.Log.Entries) == 0 {
		return Import{}, fmt.Errorf("no request found in the .har (the log has no entries)")
	}

	origins := map[string]int{}
	type kept struct {
		entry  harEntry
		parsed *url.URL
		origin string
	}
	var candidates []kept
	assets := 0
	for _, entry := range file.Log.Entries {
		parsed, err := url.Parse(entry.Request.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		if !worthKeeping(entry) {
			assets++
			continue
		}
		origin := parsed.Scheme + "://" + parsed.Host
		origins[origin]++
		candidates = append(candidates, kept{entry, parsed, origin})
	}
	if len(candidates) == 0 {
		return Import{}, fmt.Errorf(`no API-like request found in the .har.
The importer keeps non-GET requests and GET requests that returned JSON; images, styles,
fonts and navigation were left out. If the calls you want are plain GETs of HTML, they were
treated as navigation`)
	}

	target := commonest(origins)
	script := Script{Name: "Imported from HAR", Target: target}
	otherHosts := 0
	used := map[string]int{}
	for _, candidate := range candidates {
		if candidate.origin != target {
			otherHosts++
			continue
		}
		script.Steps = append(script.Steps, harStep(candidate.entry, candidate.parsed, used))
	}
	if len(script.Steps) == 0 {
		return Import{}, fmt.Errorf("every kept request pointed at a host other than %s; the importer writes one target per scenario", target)
	}

	if assets > 0 {
		script.Warnings = append(script.Warnings,
			fmt.Sprintf("%d request(s) left out as assets or navigation (images, styles, fonts, GET without JSON)", assets))
	}
	if otherHosts > 0 {
		script.Warnings = append(script.Warnings,
			fmt.Sprintf("%d request(s) left out for pointing at a host other than %s", otherHosts, target))
	}
	return RenderYAML(script), nil
}

func worthKeeping(entry harEntry) bool {
	if strings.ToUpper(entry.Request.Method) != "GET" {
		return true
	}
	switch strings.ToLower(entry.ResourceType) {
	case "xhr", "fetch":
		return true
	}
	return strings.Contains(strings.ToLower(entry.Response.Content.MimeType), "json")
}

func harStep(entry harEntry, parsed *url.URL, used map[string]int) ImportedStep {
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	headers := map[string]string{}
	for _, header := range entry.Request.Headers {
		name := strings.TrimSpace(header.Name)
		if name == "" || strings.HasPrefix(name, ":") {
			continue
		}
		switch strings.ToLower(name) {
		case "host", "content-length", "connection", "accept-encoding":
			continue
		}
		headers[name] = header.Value
	}
	step := ImportedStep{
		Method:  strings.ToUpper(entry.Request.Method),
		Path:    path,
		Headers: headers,
	}
	if entry.Request.PostData != nil {
		step.Body = entry.Request.PostData.Text
	}
	if entry.Response.Status >= 100 && entry.Response.Status < 600 {
		step.ExpectedStatus = entry.Response.Status
	}
	step.Name = harStepName(step.Method, parsed.Path, used)
	return step
}

func harStepName(method, path string, used map[string]int) string {
	segment := "root"
	if parts := strings.Split(strings.Trim(path, "/"), "/"); len(parts) > 0 && parts[len(parts)-1] != "" {
		segment = parts[len(parts)-1]
	}
	name := strings.ToLower(method) + " " + segment
	used[name]++
	if used[name] > 1 {
		name = fmt.Sprintf("%s %d", name, used[name])
	}
	return name
}

func commonest(counts map[string]int) string {
	best, most := "", -1
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if counts[key] > most {
			best, most = key, counts[key]
		}
	}
	return best
}
