package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/db/verify"
)

func TestVerifyCommandDefaultsAndSelection(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 30, 0, 0, time.FixedZone("west", -7*60*60))
	tests := []struct {
		name       string
		args       []string
		wantDate   civil.Date
		wantChecks []verify.Check
		wantRel    string
	}{
		{
			name:       "UTC day before yesterday and all checks",
			wantDate:   civil.Date{Year: 2026, Month: 8, Day: 26},
			wantChecks: verify.AllChecks,
		},
		{
			name:       "explicit repeatable selection",
			args:       []string{"--date=2024-02-29", "--check=daily-totals", "--check=bq-completeness", "--release=4.20"},
			wantDate:   civil.Date{Year: 2024, Month: 2, Day: 29},
			wantChecks: []verify.Check{verify.CheckBQCompleteness, verify.CheckDailyTotals},
			wantRel:    "4.20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			cmd := newVerifyCommandWithDependencies(verifyCommandDependencies{
				now: now,
				run: func(_ context.Context, flags *VerifyFlags, date civil.Date, checks []verify.Check) (verify.Result, error) {
					called = true
					assert.Equal(t, tt.wantDate, date)
					assert.Equal(t, tt.wantChecks, checks)
					assert.Equal(t, tt.wantRel, flags.Release)
					return verify.Result{Summaries: []verify.Summary{{Check: checks[0], Date: date, Passed: true}}}, nil
				},
			})
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			assert.True(t, called)
		})
	}
}

func TestVerifyCommandValidationAndExit(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		result  verify.Result
		wantErr string
		called  bool
	}{
		{name: "invalid date", args: []string{"--date=nope"}, wantErr: "invalid --date", called: false},
		{name: "invalid check", args: []string{"--check=nope"}, wantErr: "invalid --check", called: false},
		{name: "mismatch returns failure", result: verify.Result{Summaries: []verify.Summary{{Check: verify.CheckDailyTotals, Date: civil.Date{Year: 2026, Month: 1, Day: 1}, Passed: false}}}, wantErr: "one or more verification checks failed", called: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			cmd := newVerifyCommandWithDependencies(verifyCommandDependencies{
				now: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC),
				run: func(context.Context, *VerifyFlags, civil.Date, []verify.Check) (verify.Result, error) {
					called = true
					return tt.result, nil
				},
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, tt.called, called)
		})
	}
}

func TestVerifyCommandHasNoFixFlag(t *testing.T) {
	cmd := NewVerifyCommand()
	assert.Nil(t, cmd.Flags().Lookup("fix"))
	assert.Nil(t, cmd.PersistentFlags().Lookup("fix"))
}

func TestVerifyCommandLogsSummaryOnRunError(t *testing.T) {
	logger := log.StandardLogger()
	var output bytes.Buffer
	previousOutput := logger.Out
	previousFormatter := logger.Formatter
	previousLevel := logger.Level
	logger.SetOutput(&output)
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetLevel(log.InfoLevel)
	t.Cleanup(func() {
		logger.SetOutput(previousOutput)
		logger.SetFormatter(previousFormatter)
		logger.SetLevel(previousLevel)
	})

	runErr := errors.New("database unavailable")
	cmd := newVerifyCommandWithDependencies(verifyCommandDependencies{
		now: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC),
		run: func(context.Context, *VerifyFlags, civil.Date, []verify.Check) (verify.Result, error) {
			return verify.Result{}, runErr
		},
	})
	cmd.SetArgs([]string{"--check=daily-totals", "--release=4.20"})
	require.ErrorIs(t, cmd.Execute(), runErr)

	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	assert.Equal(t, "verification summary", record["msg"])
	assert.Equal(t, "error", record["level"])
	assert.Equal(t, "daily-totals", record["check"])
	assert.Equal(t, "4.20", record["release"])
	assert.Equal(t, "2026-08-25", record["date"])
	assert.Equal(t, false, record["passed"])
	assert.Equal(t, runErr.Error(), record["error"])
}
