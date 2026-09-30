package domain

import "sort"

// The per-layer breakdown of a run (RFC 0005 §12.3).
//
// The exit criterion of the milestone is that one commit's `glossa
// check` and its pull-request check agree on the conclusion, the error
// count and the counts per layer. Two surfaces that each grouped their
// own findings would agree by coincidence until the day they didn't, so
// the grouping lives here, once, beside the finding it groups.

// LayerCount is one layer's findings, counted by how the policy left
// them. Waived is counted on its own and is never inside Errors or
// Warnings, exactly as a run's Counts are (RFC 0005 §2.3).
type LayerCount struct {
	Layer  Layer  `json:"layer"`
	Counts Counts `json:"counts"`
}

// Total is every finding this layer reported.
func (l LayerCount) Total() int { return l.Counts.Total() }

// ByLayer groups fs by layer, in Layers order.
//
// A layer that found nothing is left out: this says what was reported,
// and which layers ran at all is CheckRun.Layers — a different question,
// and the one that tells "clean" from "not looked at". A finding from a
// layer this build has never heard of is kept, after the known ones, so
// that the columns always sum to the run's own counts.
func ByLayer(fs []Finding) []LayerCount {
	counts := map[Layer]*Counts{}
	for i := range fs {
		c, ok := counts[fs[i].Layer]
		if !ok {
			c = &Counts{}
			counts[fs[i].Layer] = c
		}
		c.Count(fs[i])
	}
	out := make([]LayerCount, 0, len(counts))
	for _, l := range Layers {
		if c, ok := counts[l]; ok {
			out = append(out, LayerCount{Layer: l, Counts: *c})
			delete(counts, l)
		}
	}
	rest := make([]Layer, 0, len(counts))
	for l := range counts {
		rest = append(rest, l)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
	for _, l := range rest {
		out = append(out, LayerCount{Layer: l, Counts: *counts[l]})
	}
	return out
}
