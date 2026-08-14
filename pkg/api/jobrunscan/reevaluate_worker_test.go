package jobrunscan

import (
	"testing"
)

func TestBuildInsertParams(t *testing.T) {
	buildIDs := []string{"111", "222", "333"}
	hash := "abc123"

	params, keys := BuildInsertParams(buildIDs, hash)

	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	for i, id := range buildIDs {
		args, ok := params[i].Args.(ReevaluateJobRunArgs)
		if !ok {
			t.Fatalf("params[%d].Args is not ReevaluateJobRunArgs", i)
		}
		if args.ProwJobBuildID != id {
			t.Errorf("params[%d].ProwJobBuildID = %q, want %q", i, args.ProwJobBuildID, id)
		}
		if args.SymptomHash != hash {
			t.Errorf("params[%d].SymptomHash = %q, want %q", i, args.SymptomHash, hash)
		}
		if keys[i] != id {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], id)
		}
	}
}

func TestReevaluateJobRunArgs_Kind(t *testing.T) {
	args := ReevaluateJobRunArgs{}
	if args.Kind() != ReevaluateJobKind {
		t.Errorf("Kind() = %q, want %q", args.Kind(), ReevaluateJobKind)
	}
}

func TestReevaluateJobRunArgs_InsertOpts(t *testing.T) {
	args := ReevaluateJobRunArgs{}
	opts := args.InsertOpts()

	if opts.Queue != ReevaluateQueue {
		t.Errorf("Queue = %q, want %q", opts.Queue, ReevaluateQueue)
	}
	if opts.MaxAttempts != ReevaluateMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", opts.MaxAttempts, ReevaluateMaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Error("UniqueOpts.ByArgs should be true")
	}
	if opts.UniqueOpts.ByPeriod != ReevaluateDedupPeriod {
		t.Errorf("UniqueOpts.ByPeriod = %v, want %v", opts.UniqueOpts.ByPeriod, ReevaluateDedupPeriod)
	}
}
