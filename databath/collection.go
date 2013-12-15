package databath

import (
	"log"
)

type Collection struct {
	Fields         map[string]Field
	FieldSets      map[string][]FieldSetFieldDef
	CustomFields   map[string]FieldSetFieldDef
	Hooks          []Hook
	TableName      string
	SearchPrefixes map[string]*SearchPrefix
}

func (c *Collection) GetFieldSet(fieldSetNamePointer *string) ([]FieldSetFieldDef, error) {
	var fieldSetName string
	if fieldSetNamePointer == nil {
		fieldSetName = "default"
	} else {
		fieldSetName = *fieldSetNamePointer
	}

	fields, ok := c.FieldSets[fieldSetName]
	if !ok {
		return nil, QueryUserError{"Fieldset " + fieldSetName + " doesn't exist"}
	}
	log.Printf("Using fieldset: %s.%s\n", c.TableName, fieldSetName)

	return fields, nil
}

type SearchPrefix struct {
	Prefix    string
	Field     Field
	FieldName string
}