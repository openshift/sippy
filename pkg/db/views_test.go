package db

import (
	"testing"
)

func TestMatviewSourcesRefreshBeforeDependents(t *testing.T) {
	phaseByName := make(map[string]int)
	for _, mv := range PostgresMatViews {
		phaseByName[mv.Name] = mv.RefreshPhase
	}

	for _, mv := range PostgresMatViews {
		for _, v := range mv.ReplaceStrings {
			sourcePhase, ok := phaseByName[v]
			if !ok {
				continue
			}
			if sourcePhase >= mv.RefreshPhase {
				t.Errorf("%s (phase %d) reads from %s (phase %d), but source must be in an earlier phase",
					mv.Name, mv.RefreshPhase, v, sourcePhase)
			}
		}
	}
}

func TestRefreshByPhase(t *testing.T) {
	matviews := []PostgresView{
		{Name: "c", RefreshPhase: 2},
		{Name: "a1", RefreshPhase: 0},
		{Name: "b1", RefreshPhase: 1},
		{Name: "a2", RefreshPhase: 0},
		{Name: "b2", RefreshPhase: 1},
	}

	var callOrder [][]string
	RefreshByPhase(matviews, func(phase []PostgresView) {
		var names []string
		for _, mv := range phase {
			names = append(names, mv.Name)
		}
		callOrder = append(callOrder, names)
	})

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(callOrder))
	}
	if len(callOrder[0]) != 2 {
		t.Errorf("phase 0: expected 2 matviews, got %d", len(callOrder[0]))
	}
	if len(callOrder[1]) != 2 {
		t.Errorf("phase 1: expected 2 matviews, got %d", len(callOrder[1]))
	}
	if len(callOrder[2]) != 1 || callOrder[2][0] != "c" {
		t.Errorf("phase 2: expected [c], got %v", callOrder[2])
	}
}
