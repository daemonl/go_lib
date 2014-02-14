package databath

import (
	"fmt"
	"log"
	"strings"
)

type FieldSetFieldDefRaw struct {
	Query    string  `json:"query"`
	DataType string  `json:"dataType"`
	Path     string  `json:"path"`
	Join     *string `json:"join"`
}

func (f *FieldSetFieldDefRaw) init() error { return nil }

func (f *FieldSetFieldDefRaw) GetPath() string { return f.Path }

func (f *FieldSetFieldDefRaw) walkField(query *Query, baseTable *MappedTable, index int) error {

	field, err := FieldByType(f.DataType)
	if err != nil {
		return err
	}

	sel := ""
	mappedField, err := query.includeField(f.Path, field, baseTable, &sel)
	mappedField.AllowSearch = false

	var replError error
	replFunc := func(in string) string {
		log.Println("Walk: " + in)
		parts := strings.Split(in[1:len(in)-1], ".")
		currentTable := baseTable
		for i, tableJump := range parts[:len(parts)-1] {
			log.Println("Walk " + tableJump)
			currentTable, err = query.leftJoin(currentTable, parts[:i+1], parts[i])
			if err != nil {
				replError = err
				return ""
			}

		}
		return currentTable.alias + "." + parts[len(parts)-1]
	}

	joinReplFunc := func(in string) string {
		log.Println("Collection Walk: " + in)
		collectionName := in[1 : len(in)-1]
		mapped, ok := query.map_table[collectionName]
		if ok {
			log.Printf("Alias: %s->%s\n", collectionName, mapped.alias)
			return mapped.alias

		}
		fmt.Println(query.map_table)
		log.Printf("No Alias: %s\n", collectionName)

		return collectionName
	}

	if f.Join != nil {
		joinReplaced := re_fieldInSquares.ReplaceAllStringFunc(*f.Join, joinReplFunc)
		query.joins = append(query.joins, joinReplaced)
	}

	raw := re_fieldInSquares.ReplaceAllStringFunc(f.Query, replFunc)

	if replError != nil {
		return replError
	}

	sel = raw + " AS " + mappedField.alias

	return nil
}