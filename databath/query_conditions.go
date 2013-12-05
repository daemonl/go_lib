package databath

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var re_notAlphaNumeric *regexp.Regexp
var re_numeric *regexp.Regexp
var re_questionmark *regexp.Regexp

func init() {
	re_notAlphaNumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)
	re_numeric = regexp.MustCompile(`^[0-9]*$`)
	re_questionmark = regexp.MustCompile(`\?`)
}

type QueryConditions struct {
	collection string
	where      []QueryCondition
	pk         *uint64
	fieldset   *string
	limit      *int64
	offset     *int64
	sort       []*QuerySort
	search     map[string]string
}

type QuerySort struct {
	Direction int32  `json:"direction"`
	FieldName string `json:"fieldName"`
}

type QueryCondition interface {
	GetConditionString(q *Query) (string, error)
}

type QueryConditionString struct {
	String string // No JSON. This CANNOT be exposed to the user, Utility Only.
}

func (qc *QueryConditionString) GetConditionString(q *Query) (string, error) {
	return "(" + qc.String + ")", nil
}

type QueryConditionWhere struct {
	Field string      `json:"field"`
	Cmp   string      `json:"cmp"`
	Val   interface{} `json:"val"`
}

func GetMinimalQueryConditions(collectionName string, fieldset string) *QueryConditions {
	qc := QueryConditions{
		collection: collectionName,
		where:      make([]QueryCondition, 0, 0),
		fieldset:   &fieldset,
	}
	return &qc
}

func (cq *CustomQuery) Run(bath *Bath, inFields []interface{}) ([]map[string]interface{}, error) {
	allRows := make([]map[string]interface{}, 0, 0)
	if len(inFields) != len(cq.InFields) {
		return allRows, UserErrorF("Could not run query, got %d parameters, expected %d", len(inFields), len(cq.InFields))
	}
	dbFields := make([]string, len(inFields), len(inFields))
	for i, field := range cq.InFields {
		dbStr, err := field.ToDb(inFields[i])
		if err != nil {
			return allRows, UserErrorF("Could not run query: %s", err.Error())
		}
		dbFields[i] = dbStr
	}
	currentIndex := 0
	replacer := func(q string) string {
		r := dbFields[currentIndex]
		currentIndex += 1
		return r
	}
	sqlString := re_questionmark.ReplaceAllStringFunc(cq.Query, replacer)

	log.Println("SQL: " + sqlString)
	c := bath.GetConnection()
	db := c.GetDB()
	defer c.Release()
	res, err := db.Query(sqlString)
	if err != nil {
		return allRows, err
	}

	for res.Next() {
		thisRow := make(map[string]interface{})
		cols := make([]interface{}, 0, 0)
		for colName, field := range cq.OutFields {
			r := field.GetScanReciever()
			// r is a pointer to a pointer of the correct type (**string, **float64 etc). (NOT a *interface{}, or **interface{} which are different things)
			thisRow[colName] = r
			cols = append(cols, r)

		}

		// Scan the values - copies the row result into the value pointed by the 'rawValue'
		err := res.Scan(cols...)
		if err != nil {
			return allRows, err
		}

		allRows = append(allRows, thisRow)
	}
	return allRows, nil
}

func (qc *QueryConditionWhere) GetConditionString(q *Query) (string, error) {
	//log.Println("GetConditionString")

	field, ok := q.map_field[qc.Field]
	if !ok {
		q.Dump()
		return "", QueryUserError{"Cannot query on non mapped field '" + qc.Field + "'."}
	}

	if qc.Cmp == "IN" {

		switch reflect.TypeOf(qc.Val).Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(qc.Val)
			length := s.Len()
			escapedSlice := make([]string, length, length)
			for i := 0; i < length; i++ {
				escaped, err := field.field.ToDb(s.Index(i).Interface())
				if err != nil {
					return "", err
				}
				escapedSlice[i] = escaped

			}
			return fmt.Sprintf("%s.%s IN (%s)", field.table.alias, field.fieldNameInTable, strings.Join(escapedSlice, ", ")), nil

		default:
			fmt.Printf("TYPE for IN: %v\n", qc.Val)
			return "", QueryUserError{"IN conditions require that val is an array"}
		}

	} else if qc.Cmp == "=" || qc.Cmp == "!=" || qc.Cmp == "<=" || qc.Cmp == ">=" || qc.Cmp == "<" || qc.Cmp == ">" {

		escaped, err := field.field.ToDb(qc.Val)
		if err != nil {
			return "", UserErrorF("%T.ToDb Error: %s", field.field, err.Error())
		}
		return fmt.Sprintf("%s.%s %s %s", field.table.alias, field.fieldNameInTable, qc.Cmp, escaped), nil
	} else if qc.Cmp == "LIKE" {
		escaped, err := field.field.ToDb(qc.Val)
		if err != nil {
			return "", err
		}
		escaped = escaped[1 : len(escaped)-1]
		return fmt.Sprintf("%s.%s LIKE \"%%%s%%\"", field.table.alias, field.fieldNameInTable, escaped), nil
	} else {
		return "", QueryUserError{"Compare method not allowed"}
	}

}

func (q *Query) JoinConditionsWith(conditions []QueryCondition, joiner string) (string, error) {
	//log.Println("Start Join")
	results := make([]string, len(conditions), len(conditions))
	var err error
	for i, condition := range conditions {
		//log.Printf("Join %d/%d", i+1, len(conditions))
		results[i], err = condition.GetConditionString(q)
		//log.Println("Join %d/%d Done", i+1, len(conditions))
		if err != nil {
			log.Printf("Where Condition Error: %s", err)
			return "", UserErrorF("building condition %d: %s", i, err.Error())
		}
	}
	//log.Println("End Join")
	return strings.Join(results, joiner), nil
}

func (q *Query) makeWhereString(conditions *QueryConditions) (string, error) {
	log.Println("Begin makeWhereString")

	if conditions.where == nil {
		conditions.where = make([]QueryCondition, 0, 0)
		log.Println("Add empty conditions.where")
	}
	if conditions.pk != nil {
		log.Println("Add PK condition")
		pkCondition := QueryConditionWhere{
			Field: "id",
			Cmp:   "=",
			Val:   *conditions.pk,
		}
		conditions.where = append(conditions.where, &pkCondition)
	}

	/*
		if conditions.filter != nil {
			for fieldName, value := range conditions.filter {
				filterCondition := QueryConditionWhere{
					Field: fieldName,
					Cmp:   "=",
					Val:   &value,
				}
				conditions.where = append(conditions.where, &filterCondition)
			}

		}
	*/
	if conditions.search != nil {

		for field, term := range conditions.search {

			parts := re_notAlphaNumeric.Split(term, -1)

			if field == "*" {
				if re_numeric.MatchString(term) {
					id, _ := strconv.ParseUint(term, 10, 32)
					filterCondition := QueryConditionWhere{
						Field: "id",
						Cmp:   "=",
						Val:   id,
					}
					conditions.where = append(conditions.where, &filterCondition)

				} else {
					allTextFields := make([]string, 0, 0)
					for path, mappedField := range q.map_field {
						if mappedField.field.IsSearchable() {
							allTextFields = append(allTextFields, path)
						}
					}

					for _, part := range parts {
						partVal := "%" + part + "%"
						partGroup := make([]QueryCondition, len(allTextFields), len(allTextFields))
						for i, fieldName := range allTextFields {
							condition := QueryConditionWhere{
								Field: fieldName,
								Cmp:   "LIKE",
								Val:   partVal,
							}
							partGroup[i] = &condition
						}
						joined, err := q.JoinConditionsWith(partGroup, " OR ")
						if err != nil {
							return "", err
						}
						strCondition := QueryConditionString{joined}
						conditions.where = append(conditions.where, &strCondition)
					}

				}
			} else {
				partGroup := make([]QueryCondition, len(parts), len(parts))
				for i, p := range parts {
					qc := QueryConditionWhere{
						Field: field,
						Cmp:   "LIKE",
						Val:   p,
					}
					partGroup[i] = &qc
				}
				joined, err := q.JoinConditionsWith(partGroup, " OR ")
				if err != nil {
					return "", err
				}
				strCondition := QueryConditionString{joined}
				conditions.where = append(conditions.where, &strCondition)
			}

		}

	}

	if len(conditions.where) < 1 {

		return "", nil
	}
	joined, err := q.JoinConditionsWith(conditions.where, " AND ")
	return "WHERE " + joined, err
}

func (q *Query) makePageString(conditions *QueryConditions) (string, error) {
	str := ""

	sorts := make([]string, len(conditions.sort), len(conditions.sort))
	for i, sort := range conditions.sort {
		direction := ""
		if sort.Direction < 0 {
			direction = "DESC"
		} else {
			direction = "ASC"
		}

		field, ok := q.map_field[sort.FieldName]
		if !ok {
			return "", UserErrorF("Sort referenced non mapped field %s", sort.FieldName)
		}
		sorts[i] = field.alias + " " + direction
	}

	if len(sorts) > 0 {
		str = str + " ORDER BY " + strings.Join(sorts, ", ")
	}

	if conditions.limit != nil {
		if *conditions.limit > 0 {
			str = str + fmt.Sprintf(" LIMIT %d", *conditions.limit)
		}
	}

	if conditions.offset != nil {
		str = str + fmt.Sprintf(" OFFSET %d", *conditions.offset)
	}

	return str, nil
}
