package databath

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func ParseErrF(format string, parameters ...interface{}) error {
	return errors.New(fmt.Sprintf(format, parameters...))
}

type rawModel struct {
	Collections   map[string]rawCollection  `json:"collections"`
	CustomQueries map[string]rawCustomQuery `json:"customQueries"`
	Hooks         []Hook                    `json:"hooks"`
}

type rawCollection struct {
	Fields         map[string]map[string]interface{} `json:"fields"`
	FieldSets      map[string][]string               `json:"fieldsets"`
	CustomFields   map[string]interface{}            `json:"custom"`
	SearchPrefixes map[string]*rawSearchPrefix       `json:"searchPrefixes"`
}
type rawCustomQuery struct {
	Query     string                            `json:"query"`
	InFields  []map[string]interface{}          `json:"parameters"`
	OutFields map[string]map[string]interface{} `json:"columns"`
	Type      string                            `json:"type"`
}
type rawSearchPrefix struct {
	Field string `json:"field"`
}

type Hook struct {
	Collection   string                 `json:"collection"`
	When         HookWhen               `json:"when"`
	Set          map[string]interface{} `json:"set"`
	Email        *HookEmail             `json:"email"`
	Raw          *rawCustomQuery        `json:"raw"`
	CustomAction *CustomQuery
}
type HookWhen struct {
	Field string `json:"field"`
	What  string `json:"what"`
}
type HookEmail struct {
	Recipient string `json:"recipient"`
	Template  string `json:"template"`
}

func ReadModelFromReader(modelReader io.ReadCloser) (*Model, error) {
	log.Println("\n==========\nBegin Model Init\n==========")

	var model rawModel
	decoder := json.NewDecoder(modelReader)
	err := decoder.Decode(&model)
	if err != nil {
		return nil, err
	}

	customQueries := make(map[string]*CustomQuery)
	for queryName, rawQuery := range model.CustomQueries {
		log.Printf("Custom Query: %s", queryName)
		cq := CustomQuery{
			Query:     rawQuery.Query,
			InFields:  make([]Field, len(rawQuery.InFields), len(rawQuery.InFields)),
			OutFields: make(map[string]Field),
			Type:      rawQuery.Type,
		}
		for i, rawField := range rawQuery.InFields {
			field, err := FieldFromDef(rawField)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error parsing %s.[in][%s] - %s", queryName, i, err.Error()))
			}
			cq.InFields[i] = field
		}
		for i, rawField := range rawQuery.OutFields {
			field, err := FieldFromDef(rawField)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error parsing %s.[out][%s] - %s", queryName, i, err.Error()))
			}
			cq.OutFields[i] = field
		}
		customQueries[queryName] = &cq
	}

	collections := make(map[string]*Collection)

	for collectionName, rawCollection := range model.Collections {
		log.Printf("Read Collection %s\n", collectionName)
		fields := make(map[string]Field)
		for fieldName, rawField := range rawCollection.Fields {

			field, err := FieldFromDef(rawField)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Error parsing %s.%s - %s", collectionName, fieldName, err.Error()))
			}
			fields[fieldName] = field
		}

		customFields := make(map[string]FieldSetFieldDef)

		for name, rawCustomField := range rawCollection.CustomFields {
			fsfd, err := getFieldSetFieldDef(rawCustomField)
			if err != nil {
				log.Printf(err.Error())
				return nil, err
			}
			customFields[name] = fsfd
		}

		fieldSets := make(map[string][]FieldSetFieldDef)

		_, hasDefaultFieldset := rawCollection.FieldSets["default"]
		if !hasDefaultFieldset {
			allFieldNames := make([]string, 0, 0)
			for fieldName, _ := range rawCollection.Fields {
				allFieldNames = append(allFieldNames, fieldName)
			}
			rawCollection.FieldSets["default"] = allFieldNames

		}

		_, hasIdentityFieldset := rawCollection.FieldSets["identity"]
		if !hasIdentityFieldset {
			_, exists := rawCollection.Fields["name"]
			if !exists {
				return nil, errors.New(fmt.Sprintf("No identity fieldset, and collection (%s) doesn't have a 'name' field to fall back upon.", collectionName))
			}

			rawCollection.FieldSets["identity"] = []string{"name"}
		}

		for name, rawSet := range rawCollection.FieldSets {
			log.Printf("Evaluate Fieldset: %s", name)
			rawSet = append(rawSet, "id")

			fieldSetDefs := make([]FieldSetFieldDef, len(rawSet), len(rawSet))
			for i, fieldName := range rawSet {

				customField, ok := customFields[fieldName]
				if ok {
					fieldSetDefs[i] = customField
					continue
				}

				fsfd := FieldSetFieldDefNormal{
					path:      fieldName,
					pathSplit: strings.Split(fieldName, "."),
				}
				fieldSetDefs[i] = &fsfd

				//return nil, UserErrorF("No field or custom field for %s in %s", fieldName, collectionName)

			}
			fieldSets[name] = fieldSetDefs
		}

		searchPrefixes := make(map[string]*SearchPrefix)
		for prefixStr, rawPrefix := range rawCollection.SearchPrefixes {
			//field, ok := fields[rawPrefix.Field]
			//if !ok {
			//	return nil, ParseErrF("Prefix referenced field '%s' which doesn't exist", rawPrefix.Field)
			//}
			prefix := SearchPrefix{
				//Field:     field,
				Prefix:    prefixStr,
				FieldName: rawPrefix.Field,
			}
			searchPrefixes[prefixStr] = &prefix
		}

		collection := Collection{
			Fields:         fields,
			FieldSets:      fieldSets,
			TableName:      collectionName,
			CustomFields:   customFields,
			SearchPrefixes: searchPrefixes,
		}
		collections[collectionName] = &collection
	}

	for _, h := range model.Hooks {

		if h.Raw != nil {
			rawQuery := h.Raw
			log.Println("Custom Query in Hook")
			cq := CustomQuery{
				Query:     rawQuery.Query,
				InFields:  make([]Field, len(rawQuery.InFields), len(rawQuery.InFields)),
				OutFields: make(map[string]Field),
				Type:      rawQuery.Type,
			}
			for i, rawField := range rawQuery.InFields {
				field, err := FieldFromDef(rawField)
				if err != nil {
					log.Println(err)
					return nil, errors.New(fmt.Sprintf("Error parsing hook ", err.Error()))
				}
				cq.InFields[i] = field
			}
			log.Println("DONE")
			h.CustomAction = &cq
		}

		collection, ok := collections[h.Collection]
		if !ok {
			UserErrorF("Hook on non existing collection %s", h.Collection)
		}
		collection.Hooks = append(collection.Hooks, h)

	}

	returnModel := Model{
		Collections:   collections,
		CustomQueries: customQueries,
	}
	log.Println("\n==========\nEnd Model Init\n==========")
	return &returnModel, err
}

func ReadModelFromFile(filename string) (*Model, error) {
	modelFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	m, err := ReadModelFromReader(modelFile)
	modelFile.Close()
	return m, err
}

func getFieldParamString(rawField map[string]interface{}, paramName string) (*string, error) {
	val, ok := rawField[paramName]
	if !ok {
		return nil, nil
	}
	str, ok := val.(string)
	if !ok {
		return nil, errors.New(fmt.Sprintf("param %s value must be a string", paramName))
	}
	return &str, nil
}

func getFieldParamInt(rawField map[string]interface{}, paramName string) (*int64, error) {
	val, ok := rawField[paramName]
	if !ok {
		return nil, nil
	}
	intval, ok := val.(int64)
	if !ok {
		return nil, errors.New(fmt.Sprintf("param %s value must be an integer", paramName))
	}
	return &intval, nil
}