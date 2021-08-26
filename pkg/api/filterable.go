package api

import (
	"fmt"
	"strconv"
	"strings"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"k8s.io/klog"
)

// LinkOperator determines how to chain multiple filters together, 'AND' and 'OR'
// are supported.
type LinkOperator string

const (
	LinkOperatorAnd LinkOperator = "and"
	LinkOperatorOr  LinkOperator = "or"
)

// Filter is a collection of FilterItem, with a link operator. It is used to chain
// filters together, for example: where name contains aws and runs > 10.
type Filter struct {
	Items        []FilterItem `json:"items"`
	LinkOperator LinkOperator `json:"linkOperator"`
}

// FilterItem is an individual filter consisting of a field, operator,
// and value. For example: name contains aws.
type FilterItem struct {
	Field    string `json:"columnField"`
	Operator string `json:"operatorValue"`
	Value    string `json:"value"`
}

// Filterable interface is for anything that can be filtered, it needs to
// support querying the type and value of fields.
type Filterable interface {
	GetFieldType(param string) apitype.ColumnType
	GetStringValue(param string) (string, error)
	GetNumericalValue(param string) (float64, error)
	GetArrayValue(param string) ([]string, error)
}

// Filter applies the selected filters to a filterable item.
func (filters Filter) Filter(item Filterable) (bool, error) {
	if len(filters.Items) == 0 {
		return true, nil
	}

	matches := make([]bool, 0)

	for _, filter := range filters.Items {
		klog.V(4).Infof("Applying filter: %s %s %s", filter.Field, filter.Operator, filter.Value)

		filterType := item.GetFieldType(filter.Field)
		switch filterType {
		case apitype.ColumnTypeString:
			klog.V(4).Infof("Column %s is of string type", filter.Field)
			result, err := filterString(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter string type: %s", err)
				return false, err
			}
			matches = append(matches, result)
		case apitype.ColumnTypeNumerical:
			klog.V(4).Infof("Column %s is of numerical type", filter.Field)
			result, err := filterNumerical(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter numerical type: %s", err)
				return false, err
			}
			matches = append(matches, result)
		case apitype.ColumnTypeArray:
			klog.V(4).Infof("Column %s is of array type", filter.Field)
			result, err := filterArray(filter, item)
			if err != nil {
				klog.V(4).Infof("Could not filter array type: %s", err)
				return false, err
			}
			matches = append(matches, result)
		default:
			klog.V(4).Infof("Unknown type of field %s", filter.Field)
			return false, fmt.Errorf("%s: unknown field or field type", filter.Field)
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
	case "contains":
		return strings.Contains(value, comparison), nil
	case "equals":
		return value == comparison, nil
	case "starts with":
		return strings.HasPrefix(value, comparison), nil
	case "ends with":
		return strings.HasSuffix(value, comparison), nil
	case "is empty":
		return value == "", nil
	case "is not empty":
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
	case "=":
		return value == comparison, nil
	case "!=":
		return value != comparison, nil
	case ">":
		return value > comparison, nil
	case "<":
		return value < comparison, nil
	case ">=":
		return value >= comparison, nil
	case "<=":
		return value <= comparison, nil
	case "is empty":
		return value == 0, nil
	case "is not empty":
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

func compare(a, b Filterable, sortField string) bool {
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
