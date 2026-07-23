package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/db"
)

// buildVariantFilterClause generates the WHERE clause fragment and bind args
// for filtering variant_combinations by includeVariants. Each key produces an
// array-overlap condition: variants && ARRAY['Key:v1','Key:v2']::text[].
func buildVariantFilterClause(includeVariants map[string][]string) (string, []any) {
	var clauses []string
	var args []any

	keys := make([]string, 0, len(includeVariants))
	for k := range includeVariants {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := includeVariants[key]
		if len(values) == 0 {
			continue
		}
		placeholders := make([]string, len(values))
		for i, v := range values {
			placeholders[i] = "?"
			args = append(args, key+":"+v)
		}
		clauses = append(clauses, fmt.Sprintf(
			"variants && ARRAY[%s]::text[]", strings.Join(placeholders, ", ")))
	}
	return strings.Join(clauses, " AND "), args
}

// lookupVariantValues queries variant_combinations matching the filter and
// returns a map from variant_combination_id to the extracted variant values
// for each dbGroupBy key. This runs as a small, fast query (~6ms for ~400
// rows) and the result is used to enrich aggregated rows in Go.
func lookupVariantValues(
	ctx context.Context,
	dbc *db.DB,
	includeVariants map[string][]string,
	dbGroupBy sets.Set[string],
) (map[uint]map[string]string, error) {
	filterClause, args := buildVariantFilterClause(includeVariants)

	query := "SELECT id, variants FROM variant_combinations"
	if filterClause != "" {
		query += " WHERE " + filterClause
	}

	type vcRow struct {
		ID       uint           `gorm:"column:id"`
		Variants pq.StringArray `gorm:"column:variants;type:text[]"`
	}

	var rows []vcRow
	if err := dbc.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("looking up variant values: %w", err)
	}

	result := make(map[uint]map[string]string, len(rows))
	for _, row := range rows {
		parsed := parseVariants(row.Variants)
		filtered := make(map[string]string, dbGroupBy.Len())
		for k, v := range parsed {
			if dbGroupBy.Has(k) {
				filtered[k] = v
			}
		}
		result[row.ID] = filtered
	}
	return result, nil
}

// variantGroupMapping holds the result of grouping variant_combination_ids by
// their dbGroupBy dimension values. Multiple VCIDs that share the same
// (Platform, Architecture, Network, ...) combo get the same group ID.
type variantGroupMapping struct {
	// valuesClause is a SQL fragment like "VALUES (1,0),(2,0),(3,1),..."
	// for joining as (vcid, group_id) in the query.
	valuesClause string

	// groupToVariants maps each group ID to its dimension key-value pairs.
	groupToVariants map[int]map[string]string
}

// buildVariantGroupMapping assigns a sequential group ID to each unique
// combination of dbGroupBy dimension values across all VCIDs. The resulting
// VALUES clause can be joined in SQL to push dimension-level GROUP BY into the
// database, reducing ~500K rows to ~81K (matching BQ's output granularity).
func buildVariantGroupMapping(variantLookup map[uint]map[string]string) variantGroupMapping {
	type groupEntry struct {
		groupID  int
		variants map[string]string
	}

	seen := make(map[string]groupEntry)
	groupToVariants := make(map[int]map[string]string)
	nextGroupID := 0

	var valuePairs []string

	vcids := make([]uint, 0, len(variantLookup))
	for vcid := range variantLookup {
		vcids = append(vcids, vcid)
	}
	sort.Slice(vcids, func(i, j int) bool { return vcids[i] < vcids[j] })

	for _, vcid := range vcids {
		dims := variantLookup[vcid]
		keyStr := crtest.EncodeVariants(dims)

		entry, exists := seen[keyStr]
		if !exists {
			entry = groupEntry{
				groupID:  nextGroupID,
				variants: dims,
			}
			seen[keyStr] = entry
			groupToVariants[nextGroupID] = dims
			nextGroupID++
		}
		valuePairs = append(valuePairs, fmt.Sprintf("(%d,%d)", vcid, entry.groupID))
	}

	valuesClause := ""
	if len(valuePairs) > 0 {
		valuesClause = "VALUES " + strings.Join(valuePairs, ",")
	}

	return variantGroupMapping{
		valuesClause:    valuesClause,
		groupToVariants: groupToVariants,
	}
}

// columnGroupMapping maps each real group_id to a synthetic col_group_id that
// has only columnGroupBy dimensions. Multiple group_ids that share the same
// column-level projection get the same col_group_id. The synthetic IDs start
// above the real group IDs to avoid collision, and their variant maps are
// registered in groupToVariants for scanGroupedResults to resolve.
type columnGroupMapping struct {
	valuesClause string
	colGroupIDs  map[int]int // group_id -> col_group_id
}

func buildColumnGroupMapping(
	groupToVariants map[int]map[string]string,
	columnGroupBy sets.Set[string],
) columnGroupMapping {
	nextColGroupID := len(groupToVariants)
	seen := make(map[string]int) // serialized column variants -> col_group_id
	colGroupIDs := make(map[int]int, len(groupToVariants))

	var valuePairs []string

	groupIDs := make([]int, 0, len(groupToVariants))
	for gid := range groupToVariants {
		groupIDs = append(groupIDs, gid)
	}
	sort.Ints(groupIDs)

	for _, gid := range groupIDs {
		variants := groupToVariants[gid]
		colVariants := make(map[string]string, columnGroupBy.Len())
		for k, v := range variants {
			if columnGroupBy.Has(k) {
				colVariants[k] = v
			}
		}
		keyStr := crtest.EncodeVariants(colVariants)

		colGID, exists := seen[keyStr]
		if !exists {
			colGID = nextColGroupID
			seen[keyStr] = colGID
			groupToVariants[colGID] = colVariants
			nextColGroupID++
		}
		colGroupIDs[gid] = colGID
		valuePairs = append(valuePairs, fmt.Sprintf("(%d,%d)", gid, colGID))
	}

	valuesClause := ""
	if len(valuePairs) > 0 {
		valuesClause = "VALUES " + strings.Join(valuePairs, ",")
	}

	return columnGroupMapping{
		valuesClause: valuesClause,
		colGroupIDs:  colGroupIDs,
	}
}
