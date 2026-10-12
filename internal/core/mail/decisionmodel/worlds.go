// Package decisionmodel is the operator decision's world as the model states it
// (docs/v2/stage3-decision-model.md), written apart from the code that records
// and recovers a decision: its dimensions and values, the combinations the
// model excludes, the schedules partitioned into groups that do not interact,
// and the outcome each world must have. A test builds a world from it and
// checks what the code did against what it predicts, so a defect of the
// protocol shows as a difference from the model rather than as agreement with
// itself. The manifest (docs/v2/stage3-decision-manifest.md) publishes the
// counts this package enumerates, and a test holds the two equal.
package decisionmodel

import "slices"

// Dimension is one dimension of a partition and the values it takes.
type Dimension struct {
	Name   string
	Values []string
}

// World is one combination of a partition's values, one per dimension in the
// partition's order.
type World struct {
	Partition string
	Values    []string
	dims      []Dimension
}

// Value is the world's value of the named dimension, empty when the
// partition has no such dimension.
func (w World) Value(name string) string {
	for i, d := range w.dims {
		if d.Name == name {
			return w.Values[i]
		}
	}
	return ""
}

// Name is the world as a test's name.
func (w World) Name() string {
	name := w.Partition
	for _, v := range w.Values {
		name += "/" + v
	}
	return name
}

// Partition is a group of dimensions whose values interact, enumerated as a
// product apart from every other group: a schedule of one partition holds the
// others at a single value, which the partition names.
type Partition struct {
	Name       string
	Dimensions []Dimension
	// Feasible says whether the model allows the world: an excluded
	// combination is never built and never accepted.
	Feasible func(World) bool
}

// All is every feasible world of the partition, in a fixed order.
func (p Partition) All() []World {
	var all []World
	var walk func(i int, values []string)
	walk = func(i int, values []string) {
		if i == len(p.Dimensions) {
			w := World{Partition: p.Name, Values: slices.Clone(values), dims: p.Dimensions}
			if p.Feasible(w) {
				all = append(all, w)
			}
			return
		}
		for _, v := range p.Dimensions[i].Values {
			walk(i+1, append(values, v))
		}
	}
	walk(0, nil)
	return all
}

// pair is two values of two dimensions.
type pair struct {
	i, j int
	a, b string
}

func pairsIn(w World) []pair {
	var pairs []pair
	for i := range w.Values {
		for j := i + 1; j < len(w.Values); j++ {
			pairs = append(pairs, pair{i, j, w.Values[i], w.Values[j]})
		}
	}
	return pairs
}

// Pairs is every pair of values some feasible world of the partition holds.
func (p Partition) Pairs() int { return len(pairsOf(p.All())) }

func pairsOf(worlds []World) map[pair]bool {
	pairs := map[pair]bool{}
	for _, w := range worlds {
		for _, pr := range pairsIn(w) {
			pairs[pr] = true
		}
	}
	return pairs
}

// PairCover picks, greedily and in a fixed order, feasible worlds until every
// pair of values some feasible world holds is held by a chosen one. It is
// generated, never written by hand, and Covers verifies it.
func (p Partition) PairCover() []World {
	all := p.All()
	uncovered := pairsOf(all)
	var chosen []World
	for len(uncovered) > 0 {
		best, gain := -1, 0
		for index, w := range all {
			n := 0
			for _, pr := range pairsIn(w) {
				if uncovered[pr] {
					n++
				}
			}
			if n > gain {
				best, gain = index, n
			}
		}
		chosen = append(chosen, all[best])
		for _, pr := range pairsIn(all[best]) {
			delete(uncovered, pr)
		}
	}
	return chosen
}

// Covers says whether worlds hold every pair of values the partition's
// feasible worlds hold, and only feasible worlds.
func (p Partition) Covers(worlds []World) bool {
	all := p.All()
	for _, w := range worlds {
		if !slices.ContainsFunc(all, func(f World) bool { return slices.Equal(f.Values, w.Values) }) {
			return false
		}
	}
	held := pairsOf(worlds)
	for pr := range pairsOf(all) {
		if !held[pr] {
			return false
		}
	}
	return true
}

func values(v ...string) []string { return v }
