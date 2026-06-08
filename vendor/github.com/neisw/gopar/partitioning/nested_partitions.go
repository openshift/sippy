package partitioning

import (
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// maxPartitionNameLen is the safe maximum for partition table names.
// PostgreSQL identifiers can be up to 63 characters (NAMEDATALEN-1), but
// internally PG creates an array type prefixed with "_". A 63-character
// table name produces a 64-character array type name which exceeds the
// limit, causing SQLSTATE 42710. We cap at 62 to leave room.
const maxPartitionNameLen = 62

// hashSuffixLen is the length of the "_xxxx" hash suffix appended when
// a table name prefix must be shortened (underscore + 4 hex digits).
const hashSuffixLen = 5

// isPartitionOverlapError returns true if err represents PostgreSQL SQLSTATE
// 42P17 (invalid_object_definition), which is raised when a new partition's
// bounds overlap an existing partition. It first tries errors.As with
// *pq.Error; if that fails (e.g. because the error was wrapped by an
// intermediate layer that breaks the chain) it falls back to string matching.
func isPartitionOverlapError(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "42P17"
	}
	return strings.Contains(err.Error(), "42P17")
}

func validatePartitionNameLength(name string) error {
	if len(name) > maxPartitionNameLen {
		return fmt.Errorf("partition name %q is %d characters, exceeds safe limit of %d",
			name, len(name), maxPartitionNameLen)
	}
	return nil
}

// dateSuffixLen is the length added by a daily partition date suffix: "_YYYY_MM_DD" (11) or "_pYYYY_MM_DD" (12)
func dateSuffixLen(usePartmanFormat bool) int {
	if usePartmanFormat {
		return 12
	}
	return 11
}

// shortenTablePrefix truncates a table name prefix to fit within maxLen and
// appends a 4-hex-digit FNV hash of the original name for uniqueness.
// If the name already fits it is returned unchanged.
//
// Example:
//
//	shortenTablePrefix("prow_job_run_annotations_new", 20)
//	→ "prow_job_run_an_8f3a"   (15 chars of prefix + "_" + 4 hex = 20)
func shortenTablePrefix(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}

	h := fnv.New32a()
	h.Write([]byte(name))
	hash := fmt.Sprintf("%04x", h.Sum32()&0xFFFF)

	truncLen := maxLen - hashSuffixLen
	if truncLen < 1 {
		truncLen = 1
	}

	// Avoid ending on an underscore before the hash separator
	prefix := name[:truncLen]
	prefix = strings.TrimRight(prefix, "_")

	return prefix + "_" + hash
}

// buildNestedPartitionPrefix computes the intermediate partition prefix for a
// given table and release name, shortening the table name if the resulting
// daily partition names would exceed PostgreSQL's identifier limit.
// Returns the intermediate prefix and the (possibly shortened) table prefix.
func buildNestedPartitionPrefix(tableName, release string, usePartmanFormat bool) string {
	safeName := sanitizePartitionName(release)
	full := tableName + "_p" + safeName

	// Check if the longest daily name fits
	maxDaily := len(full) + dateSuffixLen(usePartmanFormat)
	if maxDaily <= maxPartitionNameLen {
		return full
	}

	// Calculate how much space the table prefix can use:
	// total = tablePrefix + "_p" + safeName + dateSuffix
	available := maxPartitionNameLen - dateSuffixLen(usePartmanFormat) - 2 - len(safeName)
	shortened := shortenTablePrefix(tableName, available)
	return shortened + "_p" + safeName
}

// GetPartitionHierarchy returns the complete partition hierarchy for a table
// including intermediate partitions and leaf partitions
func (dbp *DB_PARTITIONS) GetPartitionHierarchy(tableName string) ([]PartitionHierarchyInfo, error) {
	start := time.Now()

	query := `
		WITH RECURSIVE partition_tree AS (
			-- Base case: root partitioned table
			SELECT
				c.relname AS table_name,
				NULL::name AS parent_name,
				0 AS level,
				CASE pp.partstrat
					WHEN 'r' THEN 'RANGE'
					WHEN 'l' THEN 'LIST'
					WHEN 'h' THEN 'HASH'
					ELSE 'UNKNOWN'
				END AS strategy,
				pg_get_expr(pp.partexprs, pp.partrelid) AS partition_key,
				NULL::text AS partition_bounds,
				EXISTS(SELECT 1 FROM pg_partitioned_table WHERE partrelid = c.oid) AS is_partitioned,
				c.oid AS partition_oid
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			LEFT JOIN pg_partitioned_table pp ON pp.partrelid = c.oid
			WHERE c.relname = $1 AND n.nspname = 'public'

			UNION ALL

			-- Recursive case: child partitions
			SELECT
				child.relname AS table_name,
				parent.relname AS parent_name,
				pt.level + 1 AS level,
				CASE pp.partstrat
					WHEN 'r' THEN 'RANGE'
					WHEN 'l' THEN 'LIST'
					WHEN 'h' THEN 'HASH'
					ELSE NULL
				END AS strategy,
				pg_get_expr(pp.partexprs, pp.partrelid) AS partition_key,
				pg_get_expr(child.relpartbound, child.oid) COLLATE "default" AS partition_bounds,
				EXISTS(SELECT 1 FROM pg_partitioned_table WHERE partrelid = child.oid) AS is_partitioned,
				child.oid AS partition_oid
			FROM partition_tree pt
			JOIN pg_class parent ON parent.relname = pt.table_name
			JOIN pg_inherits i ON i.inhparent = parent.oid
			JOIN pg_class child ON child.oid = i.inhrelid
			LEFT JOIN pg_partitioned_table pp ON pp.partrelid = child.oid
		)
		SELECT
			pt.table_name,
			pt.parent_name,
			pt.level,
			pt.strategy,
			COALESCE(pt.partition_key, '') AS partition_key,
			COALESCE(pt.partition_bounds, '') AS partition_bounds,
			pt.is_partitioned,
			pg_total_relation_size('public.' || pt.table_name) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.' || pt.table_name)) AS size_pretty,
			COALESCE(s.n_live_tup, 0) AS row_estimate
		FROM partition_tree pt
		LEFT JOIN pg_stat_user_tables s ON s.relname = pt.table_name AND s.schemaname = 'public'
		ORDER BY pt.level, pt.table_name
	`

	rows, err := dbp.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query partition hierarchy: %w", err)
	}

	type hierarchyRow struct {
		tableName       string
		parentName      sql.NullString
		level           int
		strategy        sql.NullString
		partitionKey    string
		partitionBounds string
		isPartitioned   bool
		sizeBytes       int64
		sizePretty      string
		rowEstimate     int64
	}

	results, err := scanRows(rows, func(r *sql.Rows) (hierarchyRow, error) {
		var h hierarchyRow
		err := r.Scan(&h.tableName, &h.parentName, &h.level, &h.strategy,
			&h.partitionKey, &h.partitionBounds, &h.isPartitioned,
			&h.sizeBytes, &h.sizePretty, &h.rowEstimate)
		return h, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query partition hierarchy: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("table %s not found or not partitioned", tableName)
	}

	// Convert to PartitionHierarchyInfo
	var hierarchy []PartitionHierarchyInfo
	for _, r := range results {
		info := PartitionHierarchyInfo{
			TableName:       r.tableName,
			Level:           PartitionLevel(r.level),
			IsPartitioned:   r.isPartitioned,
			IsLeaf:          !r.isPartitioned && r.level > 0,
			PartitionKey:    r.partitionKey,
			PartitionBounds: r.partitionBounds,
			SizeBytes:       r.sizeBytes,
			SizePretty:      r.sizePretty,
			RowEstimate:     r.rowEstimate,
		}

		if r.parentName.Valid {
			info.ParentTable = r.parentName.String
		}

		if r.strategy.Valid {
			info.Strategy = r.strategy.String
		}

		if date := extractDateFromPartitionName(r.tableName); date != nil {
			info.PartitionDate = date
		}

		hierarchy = append(hierarchy, info)
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"count":   len(hierarchy),
		"elapsed": elapsed,
	}).Info("retrieved partition hierarchy")

	return hierarchy, nil
}

// ListLeafPartitions returns only the leaf partitions (those that actually hold data)
// for a partitioned table, regardless of nesting level
func (dbp *DB_PARTITIONS) ListLeafPartitions(tableName string) ([]PartitionHierarchyInfo, error) {
	start := time.Now()

	query := `
		WITH RECURSIVE partition_tree AS (
			SELECT
				c.relname AS table_name,
				0 AS level,
				c.oid AS partition_oid,
				NULL::name AS parent_name
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = $1 AND n.nspname = 'public'

			UNION ALL

			SELECT
				child.relname AS table_name,
				pt.level + 1 AS level,
				child.oid AS partition_oid,
				parent.relname AS parent_name
			FROM partition_tree pt
			JOIN pg_class parent ON parent.relname = pt.table_name
			JOIN pg_inherits i ON i.inhparent = parent.oid
			JOIN pg_class child ON child.oid = i.inhrelid
		)
		SELECT
			pt.table_name,
			pt.parent_name,
			pt.level,
			pg_total_relation_size('public.' || pt.table_name) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.' || pt.table_name)) AS size_pretty,
			COALESCE(s.n_live_tup, 0) AS row_estimate,
			pg_get_expr(c.relpartbound, c.oid) AS partition_bounds
		FROM partition_tree pt
		JOIN pg_class c ON c.relname = pt.table_name
		LEFT JOIN pg_stat_user_tables s ON s.relname = pt.table_name AND s.schemaname = 'public'
		-- Only leaf partitions (not further partitioned)
		WHERE NOT EXISTS (
			SELECT 1 FROM pg_partitioned_table pp WHERE pp.partrelid = c.oid
		)
		AND pt.level > 0  -- Exclude root table
		ORDER BY pt.level, pt.table_name
	`

	rows, err := dbp.db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query leaf partitions: %w", err)
	}

	type leafRow struct {
		tableName       string
		parentName      sql.NullString
		level           int
		sizeBytes       int64
		sizePretty      string
		rowEstimate     int64
		partitionBounds sql.NullString
	}

	results, err := scanRows(rows, func(r *sql.Rows) (leafRow, error) {
		var l leafRow
		err := r.Scan(&l.tableName, &l.parentName, &l.level,
			&l.sizeBytes, &l.sizePretty, &l.rowEstimate, &l.partitionBounds)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query leaf partitions: %w", err)
	}

	// Convert to PartitionHierarchyInfo
	var leaves []PartitionHierarchyInfo
	for _, r := range results {
		info := PartitionHierarchyInfo{
			TableName:     r.tableName,
			Level:         PartitionLevel(r.level),
			IsLeaf:        true,
			IsPartitioned: false,
			SizeBytes:     r.sizeBytes,
			SizePretty:    r.sizePretty,
			RowEstimate:   r.rowEstimate,
		}

		if r.parentName.Valid {
			info.ParentTable = r.parentName.String
		}

		if r.partitionBounds.Valid {
			info.PartitionBounds = r.partitionBounds.String
		}

		if date := extractDateFromPartitionName(r.tableName); date != nil {
			info.PartitionDate = date
		}

		leaves = append(leaves, info)
	}

	elapsed := time.Since(start)
	log.WithFields(log.Fields{
		"table":   tableName,
		"count":   len(leaves),
		"elapsed": elapsed,
	}).Info("listed leaf partitions")

	return leaves, nil
}

// GetPartitionLevel returns the nesting level of a partition
// Returns 0 for the root table, 1 for first-level partitions, etc.
func (dbp *DB_PARTITIONS) GetPartitionLevel(partitionName string) (int, error) {
	query := `
		WITH RECURSIVE partition_path AS (
			SELECT
				c.relname AS table_name,
				0 AS level
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = $1 AND n.nspname = 'public'

			UNION ALL

			SELECT
				parent.relname AS table_name,
				pp.level + 1 AS level
			FROM partition_path pp
			JOIN pg_class child ON child.relname = pp.table_name
			JOIN pg_inherits i ON i.inhrelid = child.oid
			JOIN pg_class parent ON parent.oid = i.inhparent
		)
		SELECT COALESCE(MAX(level), 0) FROM partition_path
	`

	var level int
	err := dbp.db.QueryRow(query, partitionName).Scan(&level)
	if err != nil {
		return 0, fmt.Errorf("failed to get partition level: %w", err)
	}

	return level, nil
}

// extractDateFromPartitionName extracts date from partition name
// Supports formats: tablename_YYYY_MM_DD, tablename_suffix_YYYY_MM_DD,
// tablename_pYYYY_MM_DD (pg_partman), tablename_suffix_pYYYY_MM_DD
func extractDateFromPartitionName(partitionName string) *time.Time {
	// Find last occurrence of pattern _YYYY_MM_DD or _pYYYY_MM_DD
	parts := strings.Split(partitionName, "_")
	if len(parts) < 3 {
		return nil
	}

	// Take last 3 parts as potential date
	dateStr := strings.Join(parts[len(parts)-3:], "_")

	// Check for pg_partman format (pYYYY_MM_DD)
	if len(dateStr) > 1 && dateStr[0] == 'p' && dateStr[1] >= '0' && dateStr[1] <= '9' {
		// Strip the 'p' prefix
		dateStr = dateStr[1:]
	}

	t, err := time.Parse("2006_01_02", dateStr)
	if err != nil {
		return nil
	}

	return &t
}

// partitionExists checks if a partition table exists
func (dbp *DB_PARTITIONS) partitionExists(partitionName string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = $1 AND n.nspname = 'public'
		)
	`

	err := dbp.db.QueryRow(query, partitionName).Scan(&exists)
	return exists, err
}

// CreateMissingPartitionsListToRange creates LIST → RANGE nested partitions
// For each release value, creates an intermediate partition that is RANGE-partitioned by date
func (dbp *DB_PARTITIONS) CreateMissingPartitionsListToRange(
	tableName string,
	releases []string,
	startDate, endDate time.Time,
	dateColumn string,
	usePartmanFormat bool,
	dryRun bool,
) (int, error) {
	createdCount := 0

	l := log.WithFields(log.Fields{
		"table":      tableName,
		"releases":   releases,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"dry_run":    dryRun,
	})

	l.Info("creating LIST → RANGE nested partitions")

	// For each release, create intermediate partition and its daily sub-partitions
	for _, release := range releases {
		intermediatePartition := buildNestedPartitionPrefix(tableName, release, usePartmanFormat)

		if intermediatePartition != tableName+"_p"+sanitizePartitionName(release) {
			l.WithFields(log.Fields{
				"original":  tableName + "_p" + sanitizePartitionName(release),
				"shortened": intermediatePartition,
			}).Info("shortened partition prefix to fit PostgreSQL identifier limit")
		}

		// Check if intermediate partition already exists
		exists, err := dbp.partitionExists(intermediatePartition)
		if err != nil {
			return createdCount, fmt.Errorf("failed to check if %s exists: %w", intermediatePartition, err)
		}

		if !exists {
			// Create intermediate partition (LIST member that is RANGE-partitioned)
			if dryRun {
				l.WithFields(log.Fields{
					"partition": intermediatePartition,
					"release":   release,
				}).Info("[DRY RUN] would create intermediate LIST partition with RANGE sub-partitioning")
				createdCount++
			} else {
				err := dbp.createListMemberWithRangeSubPartitions(
					tableName,
					intermediatePartition,
					release,
					dateColumn,
				)
				if err != nil {
					// SQLSTATE 42P17 = partition overlap. This happens when a
					// partition for this list value already exists under a
					// different name — e.g. after a table rename where a
					// hash-shortened name no longer matches the computed name.
					if isPartitionOverlapError(err) {
						existingName, findErr := dbp.findPartitionForListValue(tableName, release)
						if findErr == nil && existingName != "" {
							l.WithFields(log.Fields{
								"computed": intermediatePartition,
								"existing": existingName,
								"release":  release,
							}).Warn("partition for list value already exists under a different name, using existing partition")
							intermediatePartition = existingName
						} else {
							return createdCount, fmt.Errorf("failed to create intermediate partition %s: %w", intermediatePartition, err)
						}
					} else {
						return createdCount, fmt.Errorf("failed to create intermediate partition %s: %w", intermediatePartition, err)
					}
				} else {
					l.WithField("partition", intermediatePartition).Info("created intermediate partition")
					createdCount++
				}
			}
		}

		// Create daily partitions under this release
		if !dryRun {
			dailyCount, err := dbp.createDailyPartitionsUnder(
				intermediatePartition,
				startDate,
				endDate,
				usePartmanFormat,
				dryRun,
			)
			if err != nil {
				return createdCount, fmt.Errorf("failed to create daily partitions under %s: %w", intermediatePartition, err)
			}
			createdCount += dailyCount
			l.WithFields(log.Fields{
				"intermediate": intermediatePartition,
				"daily_count":  dailyCount,
			}).Info("created daily partitions")
		}
	}

	l.WithField("total_created", createdCount).Info("completed LIST → RANGE partition creation")
	return createdCount, nil
}

// findPartitionForListValue finds an existing partition of parentTable whose
// list bounds match the given value. Returns the partition name or "" if none found.
func (dbp *DB_PARTITIONS) findPartitionForListValue(parentTable, listValue string) (string, error) {
	query := `
		SELECT child.relname
		FROM pg_class parent
		JOIN pg_namespace n ON n.oid = parent.relnamespace
		JOIN pg_inherits i ON i.inhparent = parent.oid
		JOIN pg_class child ON child.oid = i.inhrelid
		WHERE parent.relname = $1
		  AND n.nspname = 'public'
		  AND pg_get_expr(child.relpartbound, child.oid) = 'FOR VALUES IN (' || quote_literal($2) || ')'
		LIMIT 1
	`
	var partitionName string
	err := dbp.db.QueryRow(query, parentTable, listValue).Scan(&partitionName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return partitionName, nil
}

// createListMemberWithRangeSubPartitions creates a LIST partition member that is itself RANGE-partitioned
func (dbp *DB_PARTITIONS) createListMemberWithRangeSubPartitions(
	parentTable string,
	partitionName string,
	listValue string,
	rangeColumn string,
) error {
	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES IN (%s) PARTITION BY RANGE (%s)",
		pq.QuoteIdentifier(partitionName),
		pq.QuoteIdentifier(parentTable),
		pq.QuoteLiteral(listValue),
		pq.QuoteIdentifier(rangeColumn),
	)

	log.WithFields(log.Fields{
		"sql":       query,
		"partition": partitionName,
		"value":     listValue,
	}).Debug("creating LIST member with RANGE sub-partitioning")

	if _, err := dbp.db.Exec(query); err != nil {
		return fmt.Errorf("failed to execute CREATE TABLE: %w", err)
	}

	return nil
}

// createDailyPartitionsUnder creates daily RANGE partitions under an intermediate partition
func (dbp *DB_PARTITIONS) createDailyPartitionsUnder(
	intermediatePartition string,
	startDate, endDate time.Time,
	usePartmanFormat bool,
	dryRun bool,
) (int, error) {
	createdCount := 0

	// Normalize dates to midnight UTC
	currentDate := startDate.UTC().Truncate(24 * time.Hour)
	endDateNormalized := endDate.UTC().Truncate(24 * time.Hour)

	for !currentDate.After(endDateNormalized) {
		nextDate := currentDate.AddDate(0, 0, 1)

		// Partition name: events_v1_0_2024_01_01 or events_v1_0_p2024_01_01
		var dailyPartition string
		if usePartmanFormat {
			dailyPartition = fmt.Sprintf("%s_p%s", intermediatePartition, currentDate.Format("2006_01_02"))
		} else {
			dailyPartition = fmt.Sprintf("%s_%s", intermediatePartition, currentDate.Format("2006_01_02"))
		}

		if err := validatePartitionNameLength(dailyPartition); err != nil {
			return createdCount, err
		}

		// Check if daily partition already exists
		exists, err := dbp.partitionExists(dailyPartition)
		if err != nil {
			return createdCount, err
		}

		if !exists {
			if dryRun {
				log.WithField("partition", dailyPartition).Info("[DRY RUN] would create daily partition")
			} else {
				query := fmt.Sprintf(
					"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM (%s) TO (%s)",
					pq.QuoteIdentifier(dailyPartition),
					pq.QuoteIdentifier(intermediatePartition),
					pq.QuoteLiteral(currentDate.Format("2006-01-02")),
					pq.QuoteLiteral(nextDate.Format("2006-01-02")),
				)

				if _, err := dbp.db.Exec(query); err != nil {
					return createdCount, fmt.Errorf("failed to create daily partition %s: %w", dailyPartition, err)
				}

				log.WithField("partition", dailyPartition).Debug("created daily partition")
			}
			createdCount++
		}

		currentDate = nextDate
	}

	return createdCount, nil
}

// sanitizePartitionName converts a release name into a valid partition name component
// Examples:
//
//	"v1.0" → "v1_0"
//	"2024.1" → "2024_1"
//	"Release 2.0" → "release_2_0"
func sanitizePartitionName(name string) string {
	// Replace common separators with underscore
	result := strings.ReplaceAll(name, ".", "_")
	result = strings.ReplaceAll(result, " ", "_")
	result = strings.ReplaceAll(result, "-", "_")
	result = strings.ToLower(result)

	// Remove any remaining non-alphanumeric characters except underscore
	var sanitized strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			sanitized.WriteRune(r)
		}
	}

	return sanitized.String()
}

// GetDailyPartitionsForRelease returns all daily partitions for a specific release
func (dbp *DB_PARTITIONS) GetDailyPartitionsForRelease(
	tableName string,
	release string,
	usePartmanFormat bool,
) ([]PartitionHierarchyInfo, error) {
	intermediatePartition := buildNestedPartitionPrefix(tableName, release, usePartmanFormat)

	// Get all children of the intermediate partition
	query := `
		SELECT
			child.relname AS table_name,
			parent.relname AS parent_name,
			2 AS level,
			pg_get_expr(child.relpartbound, child.oid) AS partition_bounds,
			pg_total_relation_size('public.' || child.relname) AS size_bytes,
			pg_size_pretty(pg_total_relation_size('public.' || child.relname)) AS size_pretty,
			COALESCE(s.n_live_tup, 0) AS row_estimate
		FROM pg_class parent
		JOIN pg_inherits i ON i.inhparent = parent.oid
		JOIN pg_class child ON child.oid = i.inhrelid
		LEFT JOIN pg_stat_user_tables s ON s.relname = child.relname AND s.schemaname = 'public'
		WHERE parent.relname = $1
		ORDER BY child.relname
	`

	rows, err := dbp.db.Query(query, intermediatePartition)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily partitions for release %s: %w", release, err)
	}

	type dailyRow struct {
		tableName       string
		parentName      string
		level           int
		partitionBounds string
		sizeBytes       int64
		sizePretty      string
		rowEstimate     int64
	}

	scanned, err := scanRows(rows, func(r *sql.Rows) (dailyRow, error) {
		var d dailyRow
		err := r.Scan(&d.tableName, &d.parentName, &d.level,
			&d.partitionBounds, &d.sizeBytes, &d.sizePretty, &d.rowEstimate)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get daily partitions for release %s: %w", release, err)
	}

	// Convert to PartitionHierarchyInfo
	var results []PartitionHierarchyInfo
	for _, p := range scanned {
		datePart := extractDateFromPartitionName(p.tableName)

		info := PartitionHierarchyInfo{
			TableName:       p.tableName,
			ParentTable:     p.parentName,
			Level:           PartitionLevel(p.level),
			IsLeaf:          true,
			IsPartitioned:   false,
			PartitionBounds: p.partitionBounds,
			SizeBytes:       p.sizeBytes,
			SizePretty:      p.sizePretty,
			RowEstimate:     p.rowEstimate,
		}

		if datePart != nil {
			info.PartitionDate = datePart
		}

		results = append(results, info)
	}

	return results, nil
}

var partitionBoundsFromRe = regexp.MustCompile(`FROM \('(\d{4}-\d{2}-\d{2})`)

// extractDateFromPartitionBounds parses the FROM date from a RANGE partition
// bounds expression like "FOR VALUES FROM ('2026-04-29') TO ('2026-04-30')".
func extractDateFromPartitionBounds(bounds string) *time.Time {
	m := partitionBoundsFromRe.FindStringSubmatch(bounds)
	if len(m) < 2 {
		return nil
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return nil
	}
	return &t
}

// RenamePartitionsToMatchConfig renames all partitions of tableName so that
// their names match what the current table name and configuration would produce.
// This is useful after a table swap where partitions created under the old name
// (possibly hash-shortened) no longer match the expected naming.
//
// When releases is non-empty, operates in LIST→RANGE mode: for each release,
// computes the expected intermediate name via buildNestedPartitionPrefix, finds
// the actual partition via catalog lookup, renames if different, then renames
// daily children under it.
//
// When releases is empty, operates in flat RANGE mode: walks child partitions
// of tableName directly, extracts dates, and renames to match the expected
// tableName + dateSuffix pattern (shortening the table prefix if needed).
func (dbp *DB_PARTITIONS) RenamePartitionsToMatchConfig(
	tableName string,
	releases []string,
	usePartmanFormat bool,
	dryRun bool,
) (int, error) {
	l := log.WithFields(log.Fields{
		"table":   tableName,
		"dry_run": dryRun,
	})

	if len(releases) == 0 {
		return dbp.renameRangePartitions(tableName, usePartmanFormat, dryRun, l)
	}
	return dbp.renameNestedPartitions(tableName, releases, usePartmanFormat, dryRun, l)
}

// renameRangePartitions handles flat RANGE tables: walks child partitions,
// extracts dates, computes expected names with truncation-aware prefix.
func (dbp *DB_PARTITIONS) renameRangePartitions(
	tableName string,
	usePartmanFormat bool,
	dryRun bool,
	l *log.Entry,
) (int, error) {
	renamedCount := 0

	l.Info("reconciling flat RANGE partition names")

	children, err := dbp.getChildPartitions(tableName)
	if err != nil {
		return 0, fmt.Errorf("failed to list children of %s: %w", tableName, err)
	}

	// Compute the table prefix, shortening if daily names would exceed the limit
	prefix := tableName
	maxDaily := len(tableName) + 1 + dateSuffixLen(usePartmanFormat)
	if maxDaily > maxPartitionNameLen {
		available := maxPartitionNameLen - dateSuffixLen(usePartmanFormat)
		prefix = shortenTablePrefix(tableName, available)
	}

	for _, child := range children {
		date := extractDateFromPartitionBounds(child.bounds)
		if date == nil {
			date = extractDateFromPartitionName(child.name)
		}
		if date == nil {
			l.WithField("partition", child.name).Warn("could not extract date from partition, skipping")
			continue
		}

		var expectedName string
		if usePartmanFormat {
			expectedName = fmt.Sprintf("%s_p%s", prefix, date.Format("2006_01_02"))
		} else {
			expectedName = fmt.Sprintf("%s_%s", prefix, date.Format("2006_01_02"))
		}

		if child.name == expectedName {
			continue
		}

		renamed, err := dbp.renamePartition(child.name, expectedName, dryRun, l)
		if err != nil {
			return renamedCount, err
		}
		if renamed {
			renamedCount++
		}
	}

	l.WithField("total_renamed", renamedCount).Info("partition name reconciliation complete")
	return renamedCount, nil
}

// renameNestedPartitions handles LIST→RANGE tables: for each release, renames
// the intermediate partition and its daily children to match the expected naming.
func (dbp *DB_PARTITIONS) renameNestedPartitions(
	tableName string,
	releases []string,
	usePartmanFormat bool,
	dryRun bool,
	l *log.Entry,
) (int, error) {
	renamedCount := 0

	l.Info("reconciling LIST → RANGE partition names")

	for _, release := range releases {
		expectedIntermediate := buildNestedPartitionPrefix(tableName, release, usePartmanFormat)

		actualIntermediate, err := dbp.findPartitionForListValue(tableName, release)
		if err != nil {
			return renamedCount, fmt.Errorf("failed to find partition for release %s: %w", release, err)
		}
		if actualIntermediate == "" {
			l.WithField("release", release).Debug("no partition exists for release, skipping")
			continue
		}

		// Rename the intermediate partition if needed
		if actualIntermediate != expectedIntermediate {
			renamed, err := dbp.renamePartition(actualIntermediate, expectedIntermediate, dryRun, l)
			if err != nil {
				return renamedCount, err
			}
			if renamed {
				renamedCount++
			}
		}

		// Find daily child partitions under the intermediate (use the name
		// that currently exists in the catalog — the renamed one if we just
		// renamed it, otherwise the original).
		currentIntermediate := expectedIntermediate
		if dryRun && actualIntermediate != expectedIntermediate {
			currentIntermediate = actualIntermediate
		}

		children, err := dbp.getChildPartitions(currentIntermediate)
		if err != nil {
			return renamedCount, fmt.Errorf("failed to list children of %s: %w", currentIntermediate, err)
		}

		for _, child := range children {
			date := extractDateFromPartitionBounds(child.bounds)
			if date == nil {
				date = extractDateFromPartitionName(child.name)
			}
			if date == nil {
				l.WithField("partition", child.name).Warn("could not extract date from partition, skipping")
				continue
			}

			var expectedDaily string
			if usePartmanFormat {
				expectedDaily = fmt.Sprintf("%s_p%s", expectedIntermediate, date.Format("2006_01_02"))
			} else {
				expectedDaily = fmt.Sprintf("%s_%s", expectedIntermediate, date.Format("2006_01_02"))
			}

			if child.name == expectedDaily {
				continue
			}

			renamed, err := dbp.renamePartition(child.name, expectedDaily, dryRun, l)
			if err != nil {
				return renamedCount, err
			}
			if renamed {
				renamedCount++
			}
		}
	}

	l.WithField("total_renamed", renamedCount).Info("partition name reconciliation complete")
	return renamedCount, nil
}

// renamePartition renames a single partition from oldName to newName.
// Returns true if the rename was performed (or would be in dry-run).
func (dbp *DB_PARTITIONS) renamePartition(oldName, newName string, dryRun bool, l *log.Entry) (bool, error) {
	if err := validatePartitionNameLength(newName); err != nil {
		return false, fmt.Errorf("cannot rename %s: target name too long: %w", oldName, err)
	}

	if dryRun {
		l.WithFields(log.Fields{
			"from": oldName,
			"to":   newName,
		}).Info("[DRY RUN] would rename partition")
		return true, nil
	}

	renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s",
		pq.QuoteIdentifier(oldName),
		pq.QuoteIdentifier(newName),
	)
	if _, err := dbp.db.Exec(renameSQL); err != nil {
		return false, fmt.Errorf("failed to rename %s to %s: %w", oldName, newName, err)
	}
	l.WithFields(log.Fields{
		"from": oldName,
		"to":   newName,
	}).Debug("renamed partition")
	return true, nil
}

type childPartition struct {
	name   string
	bounds string
}

// getChildPartitions returns the immediate child partitions of a table.
func (dbp *DB_PARTITIONS) getChildPartitions(parentName string) ([]childPartition, error) {
	query := `
		SELECT child.relname AS name,
		       pg_get_expr(child.relpartbound, child.oid) AS bounds
		FROM pg_class parent
		JOIN pg_namespace n ON n.oid = parent.relnamespace
		JOIN pg_inherits i ON i.inhparent = parent.oid
		JOIN pg_class child ON child.oid = i.inhrelid
		WHERE parent.relname = $1 AND n.nspname = 'public'
		ORDER BY child.relname
	`

	rows, err := dbp.db.Query(query, parentName)
	if err != nil {
		return nil, err
	}
	return scanRows(rows, func(r *sql.Rows) (childPartition, error) {
		var c childPartition
		err := r.Scan(&c.name, &c.bounds)
		return c, err
	})
}
