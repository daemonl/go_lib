package databath

import (
	"fmt"
	"log"
	"strings"
)

type Model struct {
	Collections      map[string]*Collection
	CustomQueries    map[string]*CustomQuery
	DynamicFunctions map[string]*DynamicFunction
}

type DynamicFunction struct {
	Filename   string   `json:"filename"`
	Parameters []string `json:"parameters"`
}

func (m *Model) GetIdentityString(bath *Bath, collectionName string, pk uint64) (string, error) {
	fs := "identity"
	var lim int64 = 1
	qc := QueryConditions{
		collection: collectionName,
		fieldset:   &fs,
		pk:         &pk,
		limit:      &lim,
	}
	context := MapContext{}
	q, err := GetQuery(&context, m, &qc)
	if err != nil {
		log.Println(err)
		return "", err
	}
	sql, parameters, err := q.BuildSelect()
	if err != nil {
		log.Println(err)
		return "", err
	}
	res, err := q.RunQueryWithSingleResult(bath, sql, parameters)
	if err != nil {
		log.Println(err)
		return "", err
	}
	allParts := make([]string, 0, 0)
	for path, field := range res {
		if path != "id" &&
			path != "sortIndex" &&
			len(path) > 0 && !strings.HasSuffix(path, ".id") {
			allParts = append(allParts, fmt.Sprintf("%v", field))
		}
	}
	return strings.Join(allParts, ", "), nil
}
