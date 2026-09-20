package deps

import (
	"sort"

	"barista/internal/workspace"
)

type Graph struct {
	order    []string
	index    map[string]int
	edges    map[string][]string
	dangling map[string][]string
}

func Build(repos []workspace.Repo) *Graph {
	g := &Graph{
		index:    make(map[string]int, len(repos)),
		edges:    make(map[string][]string, len(repos)),
		dangling: make(map[string][]string),
	}
	for i, r := range repos {
		g.order = append(g.order, r.Name)
		g.index[r.Name] = i
	}
	for _, r := range repos {
		for _, d := range r.Deps {
			if _, ok := g.index[d]; ok {
				g.edges[r.Name] = append(g.edges[r.Name], d)
			} else {
				g.dangling[r.Name] = append(g.dangling[r.Name], d)
			}
		}
	}
	return g
}

func (g *Graph) Names() []string { return g.order }

func (g *Graph) Index(name string) int { return g.index[name] }

func (g *Graph) Deps(name string) []string { return g.edges[name] }

func (g *Graph) Dangling(name string) []string { return g.dangling[name] }

func (g *Graph) HasDangling() bool {
	for _, name := range g.order {
		if len(g.dangling[name]) > 0 {
			return true
		}
	}
	return false
}

func (g *Graph) DependedBy(name string) []string {
	var out []string
	for _, n := range g.order {
		for _, d := range g.edges[n] {
			if d == name {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// Cycles returns the strongly connected components with more than one
// member, plus self-loops, each sorted by declaration order.
func (g *Graph) Cycles() [][]string {
	sccs := g.tarjan()
	var out [][]string
	for _, scc := range sccs {
		if len(scc) > 1 {
			out = append(out, sortedByDecl(scc, g.index))
			continue
		}
		for _, d := range g.edges[scc[0]] {
			if d == scc[0] {
				out = append(out, scc)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.index[out[i][0]] < g.index[out[j][0]] })
	return out
}

// CycleOf returns the cycle members containing name, or nil.
func (g *Graph) CycleOf(name string, cycles [][]string) []string {
	for _, c := range cycles {
		for _, n := range c {
			if n == name {
				return c
			}
		}
	}
	return nil
}

// Levels assigns every repo a 1-based build level: a repo's level is one
// above the deepest dependency chain below it. Cycle members share a level.
func (g *Graph) Levels() map[string]int {
	sccs := g.tarjan()
	compOf := make(map[string]int, len(g.order))
	for i, scc := range sccs {
		for _, n := range scc {
			compOf[n] = i
		}
	}
	compDeps := make([][]int, len(sccs))
	seen := make([]map[int]bool, len(sccs))
	for i := range seen {
		seen[i] = map[int]bool{}
	}
	for from, deps := range g.edges {
		for _, d := range deps {
			c, dc := compOf[from], compOf[d]
			if c != dc && !seen[c][dc] {
				seen[c][dc] = true
				compDeps[c] = append(compDeps[c], dc)
			}
		}
	}
	compLevel := make([]int, len(sccs))
	var level func(c int) int
	level = func(c int) int {
		if compLevel[c] > 0 {
			return compLevel[c]
		}
		lvl := 1
		for _, dc := range compDeps[c] {
			if v := level(dc) + 1; v > lvl {
				lvl = v
			}
		}
		compLevel[c] = lvl
		return lvl
	}
	levels := make(map[string]int, len(g.order))
	for _, n := range g.order {
		levels[n] = level(compOf[n])
	}
	return levels
}

// SortedByLevel returns repo names ordered by (level, declaration order).
func (g *Graph) SortedByLevel(levels map[string]int) []string {
	out := make([]string, len(g.order))
	copy(out, g.order)
	sort.SliceStable(out, func(i, j int) bool {
		return levels[out[i]] < levels[out[j]]
	})
	return out
}

func (g *Graph) tarjan() [][]string {
	var (
		sccs    [][]string
		stack   []string
		onStack = map[string]bool{}
		idx     = map[string]int{}
		low     = map[string]int{}
		counter int
	)
	var visit func(n string)
	visit = func(n string) {
		idx[n], low[n] = counter, counter
		counter++
		stack = append(stack, n)
		onStack[n] = true
		for _, d := range g.edges[n] {
			if _, seen := idx[d]; !seen {
				visit(d)
				low[n] = min(low[n], low[d])
			} else if onStack[d] {
				low[n] = min(low[n], idx[d])
			}
		}
		if low[n] == idx[n] {
			var scc []string
			for {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[top] = false
				scc = append(scc, top)
				if top == n {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}
	for _, n := range g.order {
		if _, seen := idx[n]; !seen {
			visit(n)
		}
	}
	return sccs
}

func sortedByDecl(names []string, index map[string]int) []string {
	out := make([]string, len(names))
	copy(out, names)
	sort.Slice(out, func(i, j int) bool { return index[out[i]] < index[out[j]] })
	return out
}
