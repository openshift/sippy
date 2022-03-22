package filter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/gorm/clause"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"gorm.io/gorm"
	"k8s.io/klog"
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
	OperatorContains   Operator = "contains"
	OperatorEquals     Operator = "equals"
	OperatorStartsWith Operator = "starts with"
	OperatorEndsWith   Operator = "ends with"
	OperatorIsEmpty    Operator = "is empty"
	OperatorIsNotEmpty Operator = "is not empty"

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

func (f FilterItem) orFilterToSQL(db *gorm.DB, filterable Filterable) *gorm.DB { //nolint
	switch f.Operator {
	case OperatorContains:
		// "contains" is an overloaded operator: 1) see if an array field contains an item,
		// 2) string contains a substring, so we need to know the field type.
		switch filterable.GetFieldType(f.Field) {
		case apitype.ColumnTypeArray:
			if f.Not {
				db = db.Or(fmt.Sprintf("? != ALL(%s)", f.Field), f.Value)
			} else {
				db = db.Or(fmt.Sprintf("? = ANY(%s)", f.Field), f.Value)
			}
		default:
			if f.Not {
				db = db.Or(fmt.Sprintf("%q NOT LIKE ?", f.Field), fmt.Sprintf("%%%s%%", f.Value))
			} else {
				db = db.Or(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s%%", f.Value))
			}
		}

	case OperatorEquals, OperatorArithmeticEquals:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q != ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q = ?", f.Field), f.Value)
		}
	case OperatorArithmeticGreaterThan:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q <= ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q > ?", f.Field), f.Value)
		}
	case OperatorArithmeticGreaterThanOrEquals:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q < ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q >= ?", f.Field), f.Value)
		}
	case OperatorArithmeticLessThan:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q >= ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q < ?", f.Field), f.Value)
		}
	case OperatorArithmeticLessThanOrEquals:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q > ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q <= ?", f.Field), f.Value)
		}
	case OperatorArithmeticNotEquals:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q = ?", f.Field), f.Value)
		} else {
			db = db.Or(fmt.Sprintf("%q <> ?", f.Field), f.Value)
		}
	case OperatorStartsWith:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q NOT LIKE ?", f.Field), fmt.Sprintf("%s%%", f.Value))
		} else {
			db = db.Or(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%s%%", f.Value))
		}
	case OperatorEndsWith:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q NOT LIKE ?", f.Field), fmt.Sprintf("%%%s", f.Value))
		} else {
			db = db.Or(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s", f.Value))
		}
	case OperatorIsEmpty:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q != ?", f.Field), nil)
		} else {
			db = db.Or(fmt.Sprintf("%q = ?", f.Field), nil)
		}
	case OperatorIsNotEmpty:
		if f.Not {
			db = db.Or(fmt.Sprintf("%q = ?", f.Field), nil)
		} else {
			db = db.Or(fmt.Sprintf("%q != ?", f.Field), nil)
		}
	}
	return db
}

func (f FilterItem) andFilterToSQL(db *gorm.DB, filterable Filterable) *gorm.DB { //nolint
	switch f.Operator {
	case OperatorContains:
		switch filterable.GetFieldType(f.Field) {
		case apitype.ColumnTypeArray:
			if f.Not {
				db = db.Not(fmt.Sprintf("? = ANY(%s)", f.Field), f.Value)
			} else {
				db = db.Where(fmt.Sprintf("? = ANY(%s)", f.Field), f.Value)
			}
		default:
			if f.Not {
				db = db.Not(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s%%", f.Value))
			} else {
				db = db.Where(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s%%", f.Value))
			}
		}
	case OperatorEquals, OperatorArithmeticEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q = ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q = ?", f.Field), f.Value)
		}
	case OperatorArithmeticGreaterThan:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q > ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q > ?", f.Field), f.Value)
		}
	case OperatorArithmeticGreaterThanOrEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q >= ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q >= ?", f.Field), f.Value)
		}
	case OperatorArithmeticLessThan:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q < ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q < ?", f.Field), f.Value)
		}
	case OperatorArithmeticLessThanOrEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q <= ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q <= ?", f.Field), f.Value)
		}
	case OperatorArithmeticNotEquals:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q <> ?", f.Field), f.Value)
		} else {
			db = db.Where(fmt.Sprintf("%q <> ?", f.Field), f.Value)
		}
	case OperatorStartsWith:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%s%%", f.Value))
		} else {
			db = db.Where(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%s%%", f.Value))
		}
	case OperatorEndsWith:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s", f.Value))
		} else {
			db = db.Where(fmt.Sprintf("%q LIKE ?", f.Field), fmt.Sprintf("%%%s", f.Value))
		}
	case OperatorIsEmpty:
		if f.Not {
			db = db.Not(fmt.Sprintf("%q = ?", f.Field), nil)
		} else {
			db = db.Where(fmt.Sprintf("%q = ?", f.Field), nil)
		}
	case OperatorIsNotEmpty:
		if f.Not {
			db = db.Where(fmt.Sprintf("%q = ?", f.Field), nil)
		} else {
			db = db.Not(fmt.Sprintf("%q = ?", f.Field), nil)
		}
	}

	return db
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

	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))
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

func FilterableDBResult(dbClient *gorm.DB, filterOpts *FilterOptions, filterable Filterable) (*gorm.DB, error) {
	q := filterOpts.Filter.ToSQL(dbClient, filterable)
	if filterOpts.Limit > 0 {
		q = q.Limit(filterOpts.Limit)
	}

	q.Order(clause.OrderByColumn{Column: clause.Column{Name: filterOpts.SortField}, Desc: filterOpts.Sort == "desc"})

	return q, nil
}

func (filters Filter) ToSQL(db *gorm.DB, filterable Filterable) *gorm.DB {
	for _, f := range filters.Items {
		if filters.LinkOperator == LinkOperatorAnd || filters.LinkOperator == "" {
			db = f.andFilterToSQL(db, filterable)
		} else if filters.LinkOperator == LinkOperatorOr {
			db = f.orFilterToSQL(db, filterable)
		}
	}

	return db
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

		klog.V(4).Infof("Applying filter: %s %s %s", filter.Field, filter.Operator, filter.Value)
		filterType := item.GetFieldType(filter.Field)
		switch filterType {
		case apitype.ColumnTypeString:
			klog.V(4).Infof("Column %s is of string type", filter.Field)
			result, err = filterString(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter string type: %s", err)
				return false, err
			}
		case apitype.ColumnTypeNumerical:
			klog.V(4).Infof("Column %s is of numerical type", filter.Field)
			result, err = filterNumerical(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter numerical type: %s", err)
				return false, err
			}
		case apitype.ColumnTypeArray:
			klog.V(4).Infof("Column %s is of array type", filter.Field)
			result, err = filterArray(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter array type: %s", err)
				return false, err
			}
		default:
			klog.V(4).Infof("Unknown type of field %s", filter.Field)
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
				klog.V(4).Infof("Filter matched")
				return true, nil
			}
		}

		klog.V(4).Infof("Filter did not match")
		return false, nil
	}

	// LinkOperator as "and" is the default:
	for _, value := range matches {
		if !value {
			klog.V(4).Infof("Filter did not match")
			return false, nil
		}
	}

	klog.V(4).Infof("Filter did match")
	return true, nil
}

func filterString(filter FilterItem, item Filterable) (bool, error) {
	value, err := item.GetStringValue(filter.Field)
	if err != nil {
		return false, err
	}
	klog.V(4).Infof("Got value for %s=%s", filter.Field, value)

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
			klog.Error(err)
		}

		val2, err := b.GetNumericalValue(sortField)
		if err != nil {
			klog.Error(err)
		}

		return val1 < val2
	}

	if kind == apitype.ColumnTypeString {
		val1, err := a.GetStringValue(sortField)
		if err != nil {
			klog.Error(err)
		}

		val2, err := b.GetStringValue(sortField)
		if err != nil {
			klog.Error(err)
		}

		return val1 < val2
	}

	return false
}
