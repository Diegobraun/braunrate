package metrics

import (
	"fmt"
	"sort"
)

// Merge sums result documents. It is what ADR 0003 §5 promised from the start
// and the format did not deliver: N generators emit partial aggregates and a
// coordinator adds them. Percentiles do not add, so everything here goes
// through the serialized histograms, and a document without them cannot be
// summed at all.
//
// What it does not merge, and why: the SLO verdict and the sanity check are
// read from the sum, not added; observed variety counts distinct values, and
// two sets of distinct values have an unknown union; the time series keeps only
// the two quantiles of each closed bucket, and quantiles do not add either.
// Producing any of those by addition would be inventing a number.
func Merge(documents ...Document) (Document, error) {
	if len(documents) == 0 {
		return Document{}, fmt.Errorf("nada para somar")
	}
	if len(documents) == 1 {
		return documents[0], nil
	}
	for _, document := range documents {
		if err := summable(document); err != nil {
			return Document{}, err
		}
	}
	if err := comparable(documents); err != nil {
		return Document{}, err
	}

	merged := documents[0]
	merged.Series = nil
	merged.Variety = nil
	merged.Warnings = nil
	merged.SLO = Verdict{}
	merged.Sanity = Sanity{}

	steps := map[string]*StepResult{}
	order := []string{}
	for _, step := range merged.Steps {
		copied := step
		steps[step.Name] = &copied
		order = append(order, step.Name)
	}

	for _, document := range documents[1:] {
		for _, step := range document.Steps {
			existing, seen := steps[step.Name]
			if !seen {
				copied := step
				steps[step.Name] = &copied
				order = append(order, step.Name)
				continue
			}
			if err := addStep(existing, step); err != nil {
				return Document{}, err
			}
		}

		var err error
		if merged.Overall, err = addOverall(merged.Overall, document.Overall); err != nil {
			return Document{}, err
		}
		if merged.Journey, err = addJourney(merged.Journey, document.Journey); err != nil {
			return Document{}, err
		}
		if merged.Scheduling, err = addScheduling(merged.Scheduling, document.Scheduling); err != nil {
			return Document{}, err
		}
		merged.Run = addRun(merged.Run, document.Run)
	}

	sort.Strings(order)
	merged.Steps = make([]StepResult, 0, len(order))
	for _, name := range order {
		merged.Steps = append(merged.Steps, *steps[name])
	}

	if merged.Overall.Count > 0 {
		merged.Overall.ErrorRate = float64(merged.Overall.Errors) / float64(merged.Overall.Count)
	}
	if seconds := float64(merged.Run.DurationMs) / 1000; seconds > 0 {
		merged.Overall.EffectiveRate = float64(merged.Overall.Count) / seconds
	}
	return merged, nil
}

// A version 1 document has the percentiles and not the histogram they came
// from, which is exactly the difference between reading it and adding it.
func summable(document Document) error {
	if document.FormatVersion == ResultFormatVersion {
		return nil
	}
	return fmt.Errorf("o resultado está no formato %q e somar exige o formato %q, que guarda o histograma.\n"+
		"    o arquivo antigo continua sendo lido pelo relatório e pela comparação, com os percentis que ele já tem —\n"+
		"    o que ele não tem é de onde esses percentis vieram, e percentil não soma com percentil.\n"+
		"    para somar, gere os dois resultados com esta versão",
		document.FormatVersion, ResultFormatVersion)
}

func comparable(documents []Document) error {
	first := documents[0].Run
	for _, document := range documents[1:] {
		if document.Run.Spec != first.Spec {
			return fmt.Errorf("não somo execuções de cenários diferentes: %q e %q", first.Spec, document.Run.Spec)
		}
		if document.Run.Model != first.Model {
			return fmt.Errorf("não somo execuções de modelos de chegada diferentes: %q e %q", first.Model, document.Run.Model)
		}
	}
	return nil
}

func addStep(into *StepResult, other StepResult) error {
	latency, err := into.Latency.Merged(other.Latency)
	if err != nil {
		return err
	}
	service, err := into.ServiceLatency.Merged(other.ServiceLatency)
	if err != nil {
		return err
	}
	into.Latency, into.ServiceLatency = latency, service
	into.Count += other.Count
	into.Successes += other.Successes
	into.Errors += other.Errors
	into.Bytes += other.Bytes
	into.ErrorsByClass = addCounts(into.ErrorsByClass, other.ErrorsByClass)
	into.StatusByCode = addCounts(into.StatusByCode, other.StatusByCode)
	into.Details = addCounts(into.Details, other.Details)
	return nil
}

func addOverall(into, other OverallResult) (OverallResult, error) {
	latency, err := into.Latency.Merged(other.Latency)
	if err != nil {
		return into, err
	}
	service, err := into.ServiceLatency.Merged(other.ServiceLatency)
	if err != nil {
		return into, err
	}
	into.Latency, into.ServiceLatency = latency, service
	into.Count += other.Count
	into.Successes += other.Successes
	into.Errors += other.Errors
	return into, nil
}

func addJourney(into, other Journey) (Journey, error) {
	latency, err := into.Latency.Merged(other.Latency)
	if err != nil {
		return into, err
	}
	service, err := into.ServiceLatency.Merged(other.ServiceLatency)
	if err != nil {
		return into, err
	}
	into.Latency, into.ServiceLatency = latency, service
	into.Started += other.Started
	into.Completed += other.Completed
	into.Sentence = phraseJourney(into, false)
	return into, nil
}

func addScheduling(into, other Scheduling) (Scheduling, error) {
	skew, err := into.Skew.Merged(other.Skew)
	if err != nil {
		return into, err
	}
	into.Skew = skew
	into.Sent += other.Sent
	into.Completed += other.Completed
	into.LateDispatches += other.LateDispatches
	into.DroppedByInflightLimit += other.DroppedByInflightLimit
	into.LostSamples += other.LostSamples
	into.PeakInflight += other.PeakInflight
	return into, nil
}

// The window is the union: generators that ran side by side cover the same
// wall clock, and the effective rate has to be read over the time the load was
// actually being applied.
func addRun(into, other Run) Run {
	if other.Start.Before(into.Start) {
		into.Start = other.Start
	}
	if other.End.After(into.End) {
		into.End = other.End
	}
	into.DurationMs = into.End.Sub(into.Start).Milliseconds()
	into.MaxInflight += other.MaxInflight
	into.AuthObtains += other.AuthObtains
	return into
}

func addCounts(into, other map[string]int64) map[string]int64 {
	if len(other) == 0 {
		return into
	}
	if into == nil {
		into = map[string]int64{}
	}
	for key, count := range other {
		into[key] += count
	}
	return into
}
