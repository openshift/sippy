package installhtml

import (
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"
)

func TestDetailTestsFromDB(dbc *db.DB, release string, testSubstrings []string) (string, error) {
	// TODO: use the new approach from install_by_operators.go
	dataForTestsByVariant, err := getDataForTestsByVariantFromDB(dbc, release, testSubstrings)
	if err != nil {
		return "", err
	}

	variants := sets.String{}
	for _, byVariant := range dataForTestsByVariant.testNameToVariantToTestResult {
		variants.Insert(sets.StringKeySet(byVariant).UnsortedList()...)
	}

	return dataForTestsByVariant.getTableJSON("Details for Tests", "Test Details by Variants",
		variants.List(), noChange), nil

}
