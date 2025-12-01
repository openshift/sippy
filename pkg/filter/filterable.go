package filter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	apiparam "github.com/openshift/sippy/pkg/util/param"
)

// LinkOperator determines how to chain multiple filters together, 'AND' and 'OR'
// are supported.
type LinkOperator string

const (
	LinkOperatorAnd LinkOperator = "and"
	LinkOperatorOr  LinkOperator = "or"
)

// Operator defines an operator used for filter items such as equals, contains, etc,
// as well as the arithmetic operators like ==, !=, >, etc.
type Operator string

const (
	OperatorContains           Operator = "contains"
	OperatorEquals             Operator = "equals"
	OperatorStartsWith         Operator = "starts with"
	OperatorEndsWith           Operator = "ends with"
	OperatorHasEntry           Operator = "has entry"
	OperatorHasEntryContaining Operator = "has entry containing"
	OperatorIsEmpty            Operator = "is empty"
	OperatorIsNotEmpty         Operator = "is not empty"

	OperatorArithmeticEquals              Operator = "="
	OperatorArithmeticNotEquals           Operator = "!="
	OperatorArithmeticGreaterThan         Operator = ">"
	OperatorArithmeticGreaterThanOrEquals Operator = ">="
	OperatorArithmeticLessThan            Operator = "<"
	OperatorArithmeticLessThanOrEquals    Operator = "<="
)

// Filter is a collection of FilterItem, with a link operator. It is used to chain
// filters together, for example: where name contains aws and runs > 10.
type Filter struct {
	Items        []FilterItem `json:"items"`
	LinkOperator LinkOperator `json:"linkOperator"`
}

// FilterItem is an individual filter consisting of a field, operator,
// value and a not boolean that negates the operator. For example:
// name contains aws, or name not contains aws.
type FilterItem struct {
	Field    string   `json:"columnField"`
	Not      bool     `json:"not"`
	Operator Operator `json:"operatorValue"`
	Value    string   `json:"value"`
}

// helper for constructing opposing branches of SQL logic concisely
func optNot(not bool) string {
	if not {
		return "NOT"
	}
	return ""
}

// ilikeFilter returns the SQL filter and parameters for ILIKE pattern matching,
// handling both string fields (using ILIKE directly) and array fields (using unnest with EXISTS).
func ilikeFilter(field, pattern string, not bool, filterable Filterable, fieldName string) (string, interface{}) {
	if filterable != nil && filterable.GetFieldType(fieldName) == apitype.ColumnTypeArray {
		return fmt.Sprintf("%s EXISTS (SELECT 1 FROM unnest(%s) AS elem WHERE elem ILIKE ?)", optNot(not), field), pattern
	}
	return fmt.Sprintf("%s %s ILIKE ?", field, optNot(not)), pattern
}

// applyIlikeFilter applies an ILIKE filter to a GORM DB handle, handling both string and array fields.
func applyIlikeFilter(db *gorm.DB, field, pattern string, not bool, filterable Filterable, fieldName string) *gorm.DB {
	filterSQL, params := ilikeFilter(field, pattern, not, filterable, fieldName)
	return db.Where(filterSQL, params)
}

func (f FilterItem) isEmptyFilter(field string, filterable Filterable, forBQ bool) string {
	sql := fmt.Sprintf("%s IS NULL", field)
	// should work for null/empty arrays in addition to null strings
	if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeArray {
		sql = fmt.Sprintf("(%s IS NULL or %s = '{}')", field, field)
		if forBQ {
			sql = fmt.Sprintf("(%s IS NULL or ARRAY_LENGTH(%s) = 0)", field, field)
		}
	}
	if f.Not {
		return fmt.Sprintf("NOT(%s)", sql)
	}
	return sql
}

func (f FilterItem) orFilterToSQL(db *gorm.DB, filterable Filterable) (orFilter string, orParams interface{}) { //nolint
	field := fmt.Sprintf("%q", f.Field)
	if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeTimestamp {
		field = fmt.Sprintf("extract(epoch from %s at time zone 'utc') * 1000", f.Field)
	}

	switch f.Operator {
	case OperatorHasEntry:
		if f.Not {
			return fmt.Sprintf("%s IS NULL OR ? != ALL(%s)", field, field), f.Value
		}
		return fmt.Sprintf("? = ANY(%s)", field), f.Value
	case OperatorHasEntryContaining, OperatorContains:
		return ilikeFilter(field, fmt.Sprintf("%%%s%%", f.Value), f.Not, filterable, f.Field)
	case OperatorEquals, OperatorArithmeticEquals:
		if f.Not {
			return fmt.Sprintf("%s != ?", field), f.Value
		}
		return fmt.Sprintf("%s = ?", field), f.Value
	case OperatorArithmeticGreaterThan:
		if f.Not {
			return fmt.Sprintf("%s <= ?", field), f.Value
		}
		return fmt.Sprintf("%s > ?", field), f.Value
	case OperatorArithmeticGreaterThanOrEquals:
		if f.Not {
			return fmt.Sprintf("%s < ?", field), f.Value
		}
		return fmt.Sprintf("%s >= ?", field), f.Value
	case OperatorArithmeticLessThan:
		if f.Not {
			return fmt.Sprintf("%s >= ?", field), f.Value
		}
		return fmt.Sprintf("%s < ?", field), f.Value
	case OperatorArithmeticLessThanOrEquals:
		if f.Not {
			return fmt.Sprintf("%s > ?", field), f.Value
		}
		return fmt.Sprintf("%s <= ?", field), f.Value
	case OperatorArithmeticNotEquals:
		if f.Not {
			return fmt.Sprintf("%s = ?", field), f.Value
		}
		return fmt.Sprintf("%s <> ?", field), f.Value
	case OperatorStartsWith:
		return ilikeFilter(field, fmt.Sprintf("%s%%", f.Value), f.Not, filterable, f.Field)
	case OperatorEndsWith:
		return ilikeFilter(field, fmt.Sprintf("%%%s", f.Value), f.Not, filterable, f.Field)
	case OperatorIsEmpty:
		return f.isEmptyFilter(field, filterable, false), nil
	case OperatorIsNotEmpty:
		return fmt.Sprintf("%s IS %s NULL", field, optNot(!f.Not)), nil
	}

	return "UnknownFilterOperator()", nil // cause SQL to fail in obvious way
}

func (f FilterItem) andFilterToSQL(db *gorm.DB, filterable Filterable) *gorm.DB { //nolint
	field := fmt.Sprintf("%q", f.Field)
	if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeTimestamp {
		field = fmt.Sprintf("extract(epoch from %s at time zone 'utc') * 1000", f.Field)
	}

	switch f.Operator {
	case OperatorHasEntry:
		if f.Not {
			db = db.Where(fmt.Sprintf("%s IS NULL OR ? != ALL(%s)", field, field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("? = ANY(%s)", field), f.Value)
		}
	case OperatorHasEntryContaining, OperatorContains:
		db = applyIlikeFilter(db, field, fmt.Sprintf("%%%s%%", f.Value), f.Not, filterable, f.Field)
	case OperatorEquals, OperatorArithmeticEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s = ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s = ?", field), f.Value)
		}
	case OperatorArithmeticGreaterThan:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s > ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s > ?", field), f.Value)
		}
	case OperatorArithmeticGreaterThanOrEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s >= ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s >= ?", field), f.Value)
		}
	case OperatorArithmeticLessThan:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s < ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s < ?", field), f.Value)
		}
	case OperatorArithmeticLessThanOrEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s <= ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s <= ?", field), f.Value)
		}
	case OperatorArithmeticNotEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%s <> ?", field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%s <> ?", field), f.Value)
		}
	case OperatorStartsWith:
		db = applyIlikeFilter(db, field, fmt.Sprintf("%s%%", f.Value), f.Not, filterable, f.Field)
	case OperatorEndsWith:
		db = applyIlikeFilter(db, field, fmt.Sprintf("%%%s", f.Value), f.Not, filterable, f.Field)
	case OperatorIsEmpty:
		db = db.Where(f.isEmptyFilter(field, filterable, false))
	case OperatorIsNotEmpty:
		db = db.Where(fmt.Sprintf("%s IS %s NULL", field, optNot(!f.Not)))
	}

	return db
}

func (f FilterItem) toBQStr(filterable Filterable, paramIndex int) (sql string, params []bigquery.QueryParameter) { //nolint
	field := strings.ReplaceAll(fmt.Sprintf("%q", f.Field), "\"", "")
	if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeTimestamp {
		field = fmt.Sprintf("extract(epoch from %s at time zone 'utc') * 1000", f.Field)
	}

	// Helper to create a parameter
	paramName := fmt.Sprintf("filterParam%d", paramIndex+1)
	makeParam := func(value interface{}) []bigquery.QueryParameter {
		return []bigquery.QueryParameter{
			{
				Name:  paramName,
				Value: value,
			},
		}
	}
	// BQ does not automagically cast string parameters to numbers like postgres does
	makeNumParam := func() []bigquery.QueryParameter {
		num, err := strconv.ParseFloat(f.Value, 64)
		if err != nil {
			return makeParam("NOT A NUMBER: " + f.Value) // which will break appropriately
		}
		return makeParam(num)
	}

	switch f.Operator {
	case OperatorHasEntry:
		if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeArray {
			return fmt.Sprintf("%s @%s in UNNEST(%s)", optNot(f.Not), paramName, field), makeParam(f.Value)
		}
	case OperatorContains, OperatorHasEntryContaining:
		if filterable != nil && filterable.GetFieldType(f.Field) == apitype.ColumnTypeArray {
			exists := fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS item WHERE item LIKE @%s)", field, paramName)
			return fmt.Sprintf("%s %s", optNot(f.Not), exists), makeParam(f.Value)
		}
		return fmt.Sprintf("%s LOWER(%s) LIKE @%s", optNot(f.Not), field, paramName), makeParam(fmt.Sprintf("%%%s%%", f.Value))
	case OperatorEquals:
		if f.Not {
			return fmt.Sprintf("%s != @%s", field, paramName), makeParam(f.Value)
		}
		return fmt.Sprintf("%s = @%s", field, paramName), makeParam(f.Value)
	case OperatorArithmeticEquals:
		if f.Not {
			return fmt.Sprintf("%s != @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s = @%s", field, paramName), makeNumParam()
	case OperatorArithmeticGreaterThan:
		if f.Not {
			return fmt.Sprintf("%s <= @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s > @%s", field, paramName), makeNumParam()
	case OperatorArithmeticGreaterThanOrEquals:
		if f.Not {
			return fmt.Sprintf("%s < @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s >= @%s", field, paramName), makeNumParam()
	case OperatorArithmeticLessThan:
		if f.Not {
			return fmt.Sprintf("%s >= @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s < @%s", field, paramName), makeNumParam()
	case OperatorArithmeticLessThanOrEquals:
		if f.Not {
			return fmt.Sprintf("%s > @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s <= @%s", field, paramName), makeNumParam()
	case OperatorArithmeticNotEquals:
		if f.Not {
			return fmt.Sprintf("%s = @%s", field, paramName), makeNumParam()
		}
		return fmt.Sprintf("%s != @%s", field, paramName), makeNumParam()
	case OperatorStartsWith:
		return fmt.Sprintf("%s LOWER(%s) LIKE @%s", optNot(f.Not), field, paramName), makeParam(fmt.Sprintf("%s%%", f.Value))
	case OperatorEndsWith:
		return fmt.Sprintf("%s LOWER(%s) LIKE @%s", optNot(f.Not), field, paramName), makeParam(fmt.Sprintf("%%%s", f.Value))
	case OperatorIsEmpty:
		return f.isEmptyFilter(field, filterable, true), nil
	case OperatorIsNotEmpty:
		return fmt.Sprintf("%s IS %s NULL", field, optNot(!f.Not)), nil
	}

	return "UnknownFilterOperator()", nil // cause SQL to fail in obvious way
}

// Filterable interface is for anything that can be filtered, it needs to
// support querying the type and value of fields.
type Filterable interface {
	GetFieldType(param string) apitype.ColumnType
	GetStringValue(param string) (string, error)
	GetNumericalValue(param string) (float64, error)
	GetArrayValue(param string) ([]string, error)
}

type FilterOptions struct {
	Filter    *Filter
	SortField string
	Sort      apitype.Sort
	Limit     int
}

func FilterOptionsFromRequest(req *http.Request, defaultSortField string, defaultSort apitype.Sort) (filterOpts *FilterOptions, err error) {
	filterOpts = &FilterOptions{}
	queryFilter := req.URL.Query().Get("filter")
	filter := &Filter{}
	if queryFilter != "" {
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			return filterOpts, fmt.Errorf("could not marshal filter: %w", err)
		}
	}
	filterOpts.Filter = filter

	limitParam := req.URL.Query().Get("limit")
	if limitParam == "" {
		filterOpts.Limit = 0
	} else {
		limit, err := strconv.Atoi(limitParam)
		if err != nil {
			return filterOpts, fmt.Errorf("error parsing limit param: %s", err)
		}
		filterOpts.Limit = limit
	}

	sortField := apiparam.SafeRead(req, "sortField")
	sort := apitype.Sort(apiparam.SafeRead(req, "sort"))
	if sortField == "" {
		sortField = defaultSortField
	}
	if sort == "" {
		sort = defaultSort
	}
	filterOpts.Sort = sort
	filterOpts.SortField = sortField
	return filterOpts, nil
}

// TODO: merge with FilterOptionsFromRequest
func ExtractFilters(req *http.Request) (*Filter, error) {
	filter := &Filter{}
	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			return nil, fmt.Errorf("could not unmarshal filter: %w", err)
		}
	}

	return filter, nil
}

func ApplyFilters(
	filter *Filter,
	sortField string,
	sort apitype.Sort,
	limit int,
	dbClient *gorm.DB,
	filterable Filterable) (*gorm.DB, error) {

	q := filter.ToSQL(dbClient, filterable)
	if limit > 0 {
		q = q.Limit(limit)
	}

	q.Order(clause.OrderByColumn{
		Column: clause.Column{Name: sortField},
		Desc:   sort == apitype.SortDescending})

	return q, nil
}

func FilterableDBResult(dbClient *gorm.DB, filterOpts *FilterOptions, filterable Filterable) (*gorm.DB, error) {
	q := filterOpts.Filter.ToSQL(dbClient, filterable)
	if filterOpts.Limit > 0 {
		q = q.Limit(filterOpts.Limit)
	}

	sort := apitype.SortDescending
	if filterOpts.Sort == apitype.SortAscending {
		sort = apitype.SortAscending
	}
	if len(filterOpts.SortField) > 0 {
		q.Order(fmt.Sprintf("%s %s NULLS LAST", pq.QuoteIdentifier(filterOpts.SortField), sort))
	}

	return q, nil
}

// Split extracts certain filter items into their own filter. Can be used
// for rare occurrences  when filters need to be applied separately, i.e.
// as part of pre and post-processing.
func (filters Filter) Split(fields []string) (newFilter, oldFilter *Filter) {
	newFilter = &Filter{
		Items:        []FilterItem{},
		LinkOperator: filters.LinkOperator,
	}
	oldFilter = &Filter{
		Items:        []FilterItem{},
		LinkOperator: filters.LinkOperator,
	}

filterOuterLoop:
	for _, item := range filters.Items {
		for _, field := range fields {
			if item.Field == field {
				newFilter.Items = append(newFilter.Items, item)
				continue filterOuterLoop
			}
		}
		oldFilter.Items = append(oldFilter.Items, item)
	}

	return newFilter, oldFilter
}

func (filters Filter) ToSQL(db *gorm.DB, filterable Filterable) *gorm.DB {

	orFilters := []string{}
	orFilterParams := []interface{}{}

	for _, f := range filters.Items {
		if filters.LinkOperator == LinkOperatorAnd || filters.LinkOperator == "" {
			db = f.andFilterToSQL(db, filterable)
		} else if filters.LinkOperator == LinkOperatorOr {
			q, p := f.orFilterToSQL(db, filterable)
			orFilters = append(orFilters, q)
			if p != nil {
				orFilterParams = append(orFilterParams, p)
			}
		}
	}

	// Filter ORs require special handling because they can be mixed into a query that already has
	// an AND (i.e. AND release="4.12"), which we can't then start adding ORs to or we match everything
	// unintentionally. ORs will be batched together, and then ANDed with the query.
	queryStr := strings.Join(orFilters, " or ")
	log.Debugf("final query string: %s", queryStr)
	db = db.Where(queryStr, orFilterParams...)

	return db
}

// BQFilterResult contains the WHERE clause SQL and the BigQuery parameters to use with it.
// The Parameters slice contains bigquery.QueryParameter structs that can be directly
// assigned to a BigQuery query.
type BQFilterResult struct {
	SQL        string
	Parameters []bigquery.QueryParameter
}

// ToBQStr generates a parameterized BigQuery WHERE clause with safe parameter binding
// to prevent SQL injection. Returns the SQL string and BigQuery parameters ready to use.
func (filters Filter) ToBQStr(filterable Filterable, paramIndex *int) BQFilterResult {
	items := []string{}
	allParams := []bigquery.QueryParameter{}

	for _, f := range filters.Items {
		sql, params := f.toBQStr(filterable, *paramIndex)
		items = append(items, sql)
		if params != nil {
			allParams = append(allParams, params...)
			*paramIndex += len(params)
		}
	}

	operator := " AND "
	if filters.LinkOperator == LinkOperatorOr {
		operator = " OR "
	}
	queryStr := strings.Join(items, operator)
	queryStr = "(" + queryStr + ")"
	log.Debugf("final query string: %s with %d parameters", queryStr, len(allParams))

	return BQFilterResult{
		SQL:        queryStr,
		Parameters: allParams,
	}
}

// Filter applies the selected filters to a filterable item.
func (filters Filter) Filter(item Filterable) (bool, error) {
	if len(filters.Items) == 0 {
		return true, nil
	}

	matches := make([]bool, 0)

	for _, filter := range filters.Items {
		var result bool
		var err error

		log.Debugf("Applying filter: %s %s %s", filter.Field, filter.Operator, filter.Value)
		filterType := item.GetFieldType(filter.Field)
		switch filterType {
		case apitype.ColumnTypeString:
			log.Debugf("Column %s is of string type", filter.Field)
			result, err = filterString(filter, item)
			if err != nil {
				log.Debugf("Could not filter string type: %s", err)
				return false, err
			}
		case apitype.ColumnTypeNumerical:
			log.Debugf("Column %s is of numerical type", filter.Field)
			result, err = filterNumerical(filter, item)
			if err != nil {
				log.Debugf("Could not filter numerical type: %s", err)
				return false, err
			}
		case apitype.ColumnTypeArray:
			log.Debugf("Column %s is of array type", filter.Field)
			result, err = filterArray(filter, item)
			if err != nil {
				log.Debugf("Could not filter array type: %s", err)
				return false, err
			}
		default:
			log.Debugf("Unknown type of field %s", filter.Field)
			return false, fmt.Errorf("%s: unknown field or field type", filter.Field)
		}

		if filter.Not {
			matches = append(matches, !result)
		} else {
			matches = append(matches, result)
		}
	}

	if filters.LinkOperator == LinkOperatorOr {
		for _, value := range matches {
			if value {
				log.Debugf("Filter matched")
				return true, nil
			}
		}

		log.Debugf("Filter did not match")
		return false, nil
	}

	// LinkOperator as "and" is the default:
	for _, value := range matches {
		if !value {
			log.Debugf("Filter did not match")
			return false, nil
		}
	}

	log.Debugf("Filter did match")
	return true, nil
}

func filterString(filter FilterItem, item Filterable) (bool, error) {
	value, err := item.GetStringValue(filter.Field)
	if err != nil {
		return false, err
	}
	log.Debugf("Got value for %s=%s", filter.Field, value)

	comparison := filter.Value

	switch filter.Operator {
	case OperatorContains:
		return strings.Contains(value, comparison), nil
	case OperatorEquals:
		// We've seen tests sneak in with trailing whitespace, handle this for equals comparisons:
		return strings.TrimSpace(value) == comparison, nil
	case OperatorStartsWith:
		return strings.HasPrefix(value, comparison), nil
	case OperatorEndsWith:
		return strings.HasSuffix(value, comparison), nil
	case OperatorIsEmpty:
		return value == "", nil
	case OperatorIsNotEmpty:
		return value != "", nil
	default:
		return false, fmt.Errorf("unknown string field operator %s", filter.Operator)
	}
}

func filterNumerical(filter FilterItem, item Filterable) (bool, error) {
	if filter.Value == "" {
		return true, nil
	}

	value, err := item.GetNumericalValue(filter.Field)
	if err != nil {
		return false, err
	}

	comparison, err := strconv.ParseFloat(filter.Value, 64)
	if err != nil {
		return false, err
	}

	switch filter.Operator {
	case OperatorArithmeticEquals:
		return value == comparison, nil
	case OperatorArithmeticNotEquals:
		return value != comparison, nil
	case OperatorArithmeticGreaterThan:
		return value > comparison, nil
	case OperatorArithmeticLessThan:
		return value < comparison, nil
	case OperatorArithmeticGreaterThanOrEquals:
		return value >= comparison, nil
	case OperatorArithmeticLessThanOrEquals:
		return value <= comparison, nil
	case OperatorIsEmpty:
		return value == 0, nil
	case OperatorIsNotEmpty:
		return value != 0, nil
	default:
		return false, fmt.Errorf("unknown numeric field operator %s", filter.Operator)
	}
}

func filterArray(filter FilterItem, item Filterable) (bool, error) {
	list, err := item.GetArrayValue(filter.Field)
	if err != nil {
		return false, err
	}

	for _, value := range list {
		if strings.Contains(value, filter.Value) {
			return true, nil
		}
	}

	return false, nil
}

func Compare(a, b Filterable, sortField string) bool {
	kind := a.GetFieldType(sortField)

	if kind == apitype.ColumnTypeNumerical {
		val1, err := a.GetNumericalValue(sortField)
		if err != nil {
			log.Error(err)
		}

		val2, err := b.GetNumericalValue(sortField)
		if err != nil {
			log.Error(err)
		}

		return val1 < val2
	}

	if kind == apitype.ColumnTypeString {
		val1, err := a.GetStringValue(sortField)
		if err != nil {
			log.Error(err)
		}

		val2, err := b.GetStringValue(sortField)
		if err != nil {
			log.Error(err)
		}

		return val1 < val2
	}

	return false
}
