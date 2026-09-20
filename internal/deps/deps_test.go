package deps

import (
	"reflect"
	"testing"

	"barista/internal/workspace"
)

func repos(spec ...any) []workspace.Repo {
	var out []workspace.Repo
	for _, s := range spec {
		switch v := s.(type) {
		case string:
			out = append(out, workspace.Repo{Name: v})
		case []string:
			out[len(out)-1].Deps = v
		}
	}
	return out
}

func TestBuildEdgesAndDangling(t *testing.T) {
	g := Build(repos("a", []string{"b", "ghost"}, "b"))
	if got := g.Deps("a"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("Deps(a) = %v, want [b]", got)
	}
	if got := g.Dangling("a"); !reflect.DeepEqual(got, []string{"ghost"}) {
		t.Errorf("Dangling(a) = %v, want [ghost]", got)
	}
	if !g.HasDangling() {
		t.Error("HasDangling() = false, want true")
	}
	if g.Dangling("b") != nil {
		t.Errorf("Dangling(b) = %v, want nil", g.Dangling("b"))
	}
}

func TestDependedBy(t *testing.T) {
	g := Build(repos("lib", "a", []string{"lib"}, "b", []string{"lib"}))
	if got := g.DependedBy("lib"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("DependedBy(lib) = %v, want [a b]", got)
	}
	if got := g.DependedBy("a"); got != nil {
		t.Errorf("DependedBy(a) = %v, want nil", got)
	}
}

func TestLevelsDiamond(t *testing.T) {
	g := Build(repos(
		"lib",
		"a", []string{"lib"},
		"b", []string{"lib"},
		"app", []string{"a", "b"},
	))
	levels := g.Levels()
	want := map[string]int{"lib": 1, "a": 2, "b": 2, "app": 3}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("Levels() = %v, want %v", levels, want)
	}
	if got := g.SortedByLevel(levels); !reflect.DeepEqual(got, []string{"lib", "a", "b", "app"}) {
		t.Errorf("SortedByLevel() = %v", got)
	}
}

func TestLevelsLongestPath(t *testing.T) {
	g := Build(repos(
		"lib",
		"mid", []string{"lib"},
		"app", []string{"lib", "mid"},
	))
	if got := g.Levels()["app"]; got != 3 {
		t.Errorf("Levels()[app] = %d, want 3 (longest path wins)", got)
	}
}

func TestCycles(t *testing.T) {
	g := Build(repos(
		"a", []string{"b"},
		"b", []string{"a"},
		"self", []string{"self"},
		"free",
	))
	cycles := g.Cycles()
	want := [][]string{{"a", "b"}, {"self"}}
	if !reflect.DeepEqual(cycles, want) {
		t.Errorf("Cycles() = %v, want %v", cycles, want)
	}
	if got := g.CycleOf("b", cycles); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("CycleOf(b) = %v", got)
	}
	if got := g.CycleOf("free", cycles); got != nil {
		t.Errorf("CycleOf(free) = %v, want nil", got)
	}
}

func TestCycleMembersShareLevel(t *testing.T) {
	g := Build(repos(
		"base",
		"a", []string{"base", "b"},
		"b", []string{"a"},
		"top", []string{"a"},
	))
	levels := g.Levels()
	if levels["a"] != levels["b"] {
		t.Errorf("cycle members have different levels: a=%d b=%d", levels["a"], levels["b"])
	}
	want := map[string]int{"base": 1, "a": 2, "b": 2, "top": 3}
	if !reflect.DeepEqual(levels, want) {
		t.Errorf("Levels() = %v, want %v", levels, want)
	}
}

func TestEmptyGraph(t *testing.T) {
	g := Build(nil)
	if g.Cycles() != nil {
		t.Errorf("Cycles() = %v, want nil", g.Cycles())
	}
	if len(g.Levels()) != 0 {
		t.Errorf("Levels() not empty")
	}
	if g.HasDangling() {
		t.Error("HasDangling() = true, want false")
	}
}
