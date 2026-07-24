package cumulativesummary

import (
	"testing"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
)

var testToday = civil.Date{Year: 2026, Month: 7, Day: 9}

func TestResolveStartDate_UsesEarliestChanged(t *testing.T) {
	earliestChanged := testToday.AddDays(-3)

	result := resolveStartDate(earliestChanged, nil, testToday)

	assert.Equal(t, earliestChanged, result)
}

func TestResolveStartDate_UsesMaxPlusOneWhenEarlier(t *testing.T) {
	earliestChanged := testToday.AddDays(-1)
	maxExisting := testToday.AddDays(-5)

	result := resolveStartDate(earliestChanged, &maxExisting, testToday)

	assert.Equal(t, maxExisting.AddDays(1), result)
}

func TestResolveStartDate_UsesEarliestChangedWhenEarlierThanMax(t *testing.T) {
	earliestChanged := testToday.AddDays(-5)
	maxExisting := testToday.AddDays(-2)

	result := resolveStartDate(earliestChanged, &maxExisting, testToday)

	assert.Equal(t, earliestChanged, result)
}

func TestResolveStartDate_ClampsToFloor(t *testing.T) {
	earliestChanged := testToday.AddDays(-30)

	result := resolveStartDate(earliestChanged, nil, testToday)

	assert.Equal(t, testToday.AddDays(-maxAutoFillDays), result)
}

func TestResolveStartDate_ClampsMaxPlusOneToFloor(t *testing.T) {
	earliestChanged := testToday.AddDays(-1)
	maxExisting := testToday.AddDays(-30)

	result := resolveStartDate(earliestChanged, &maxExisting, testToday)

	assert.Equal(t, testToday.AddDays(-maxAutoFillDays), result)
}

func TestResolveStartDate_DoesNotClampWhenWithinFloor(t *testing.T) {
	earliestChanged := testToday.AddDays(-7)

	result := resolveStartDate(earliestChanged, nil, testToday)

	assert.Equal(t, earliestChanged, result)
}
