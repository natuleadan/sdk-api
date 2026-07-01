package runtime

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/natuleadan/sdk-api/db"
)

var goTypeToGQL = map[reflect.Kind]*graphql.Scalar{
	reflect.Int:     graphql.Int,
	reflect.Int8:    graphql.Int,
	reflect.Int16:   graphql.Int,
	reflect.Int32:   graphql.Int,
	reflect.Int64:   graphql.Int,
	reflect.Float32: graphql.Float,
	reflect.Float64: graphql.Float,
	reflect.String:  graphql.String,
	reflect.Bool:    graphql.Boolean,
}

func goFieldToGQLType(field db.FieldInfo) graphql.Output {
	if field.FieldType.Kind() == reflect.Pointer {
		return goFieldToGQLType(db.FieldInfo{FieldType: field.FieldType.Elem()})
	}
	if field.FieldType.Kind() == reflect.Slice {
		elem := field.FieldType.Elem()
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		return graphql.NewList(goFieldToGQLType(db.FieldInfo{FieldType: elem}))
	}
	if s, ok := goTypeToGQL[field.FieldType.Kind()]; ok {
		return s
	}
	return graphql.String
}

func modelToGQLObject(name string, info *db.TableInfo) *graphql.Object {
	fields := graphql.Fields{}
	for _, f := range info.Fields {
		if f.Skip {
			continue
		}
		gqlName := toCamel(f.Column)
		gqlField := &graphql.Field{
			Type: goFieldToGQLType(f),
		}
		if f.Primary {
			gqlField.Type = graphql.ID
		}
		fields[gqlName] = gqlField
	}
	return graphql.NewObject(graphql.ObjectConfig{
		Name:   name,
		Fields: fields,
	})
}

func buildQueryFields(models map[string]*db.TableInfo, providers map[string]CRUDProvider, objectTypes map[string]*graphql.Object) graphql.Fields {
	queryFields := graphql.Fields{}
	for name := range models {
		obj := objectTypes[name]
		singleName := toCamel(name)
		pluralName := toPlural(singleName)

		queryFields[singleName] = &graphql.Field{
			Type: obj,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			},
			Resolve: func(p graphql.ResolveParams) (any, error) {
				return resolveGet(p, name, providers)
			},
		}
		queryFields[pluralName] = &graphql.Field{
			Type: graphql.NewList(obj),
			Args: graphql.FieldConfigArgument{
				"page": &graphql.ArgumentConfig{Type: graphql.Int},
				"size": &graphql.ArgumentConfig{Type: graphql.Int},
				"sort": &graphql.ArgumentConfig{Type: graphql.String},
			},
			Resolve: func(p graphql.ResolveParams) (any, error) {
				return resolveList(p, name, providers)
			},
		}
	}
	return queryFields
}

func buildMutationFields(models map[string]*db.TableInfo, providers map[string]CRUDProvider, objectTypes map[string]*graphql.Object, inputTypes map[string]*graphql.InputObject) graphql.Fields {
	mutationFields := graphql.Fields{}
	for name := range models {
		obj := objectTypes[name]
		camelName := toCamel(name)

		createInput := inputTypes[name]
		updateInput := inputTypes[name]

		mutationFields["create"+camelName] = &graphql.Field{
			Type: obj,
			Args: graphql.FieldConfigArgument{
				"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createInput)},
			},
			Resolve: func(p graphql.ResolveParams) (any, error) {
				return resolveCreate(p, name, providers)
			},
		}
		mutationFields["update"+camelName] = &graphql.Field{
			Type: obj,
			Args: graphql.FieldConfigArgument{
				"id":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(updateInput)},
			},
			Resolve: func(p graphql.ResolveParams) (any, error) {
				return resolveUpdate(p, name, providers)
			},
		}
		mutationFields["delete"+camelName] = &graphql.Field{
			Type: graphql.Boolean,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
			},
			Resolve: func(p graphql.ResolveParams) (any, error) {
				return resolveDelete(p, name, providers)
			},
		}
	}
	return mutationFields
}

func buildInputType(name string, info *db.TableInfo) *graphql.InputObject {
	fields := graphql.InputObjectConfigFieldMap{}
	for _, f := range info.Fields {
		if f.Skip || f.Auto || f.Primary {
			continue
		}
		gqlName := toCamel(f.Column)
		gqlField := &graphql.InputObjectFieldConfig{
			Type: goFieldToGQLType(f),
		}
		fields[gqlName] = gqlField
	}
	return graphql.NewInputObject(graphql.InputObjectConfig{
		Name:   name + "Input",
		Fields: fields,
	})
}

func buildGraphQLSchema(handlers *EntryHandlers, models map[string]*db.TableInfo) (*graphql.Schema, error) {
	providers := map[string]CRUDProvider{}
	if handlers != nil {
		providers = handlers.CRUD
	}

	// Build object types once, reuse for Query and Mutation
	objectTypes := map[string]*graphql.Object{}
	inputTypes := map[string]*graphql.InputObject{}
	for name, info := range models {
		objectTypes[name] = modelToGQLObject(name, info)
		inputTypes[name] = buildInputType(name, info)
	}

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Query",
		Fields: buildQueryFields(models, providers, objectTypes),
	})
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "Mutation",
		Fields: buildMutationFields(models, providers, objectTypes, inputTypes),
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
	if err != nil {
		return nil, err
	}
	return &schema, nil
}

func resolveGet(p graphql.ResolveParams, modelName string, providers map[string]CRUDProvider) (any, error) {
	provider, ok := providers[modelName]
	if !ok {
		return nil, fmt.Errorf("no provider for model %q", modelName)
	}
	id, _ := p.Args["id"].(string)
	// simulate fiber.Ctx with a mock that captures the result
	result, err := callCRUDGet(provider, id)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveList(p graphql.ResolveParams, modelName string, providers map[string]CRUDProvider) (any, error) {
	provider, ok := providers[modelName]
	if !ok {
		return nil, fmt.Errorf("no provider for model %q", modelName)
	}
	page := 1
	if v, ok := p.Args["page"].(int); ok && v > 0 {
		page = v
	}
	size := 10
	if v, ok := p.Args["size"].(int); ok && v > 0 {
		size = v
	}
	sort := "id"
	if v, ok := p.Args["sort"].(string); ok && v != "" {
		sort = v
	}
	result, err := callCRUDList(provider, page, size, sort)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveCreate(p graphql.ResolveParams, modelName string, providers map[string]CRUDProvider) (any, error) {
	provider, ok := providers[modelName]
	if !ok {
		return nil, fmt.Errorf("no provider for model %q", modelName)
	}
	input := p.Args["input"]
	result, err := callCRUDCreate(provider, input)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveUpdate(p graphql.ResolveParams, modelName string, providers map[string]CRUDProvider) (any, error) {
	provider, ok := providers[modelName]
	if !ok {
		return nil, fmt.Errorf("no provider for model %q", modelName)
	}
	id, _ := p.Args["id"].(string)
	input := p.Args["input"]
	result, err := callCRUDUpdate(provider, id, input)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func resolveDelete(p graphql.ResolveParams, modelName string, providers map[string]CRUDProvider) (any, error) {
	provider, ok := providers[modelName]
	if !ok {
		return nil, fmt.Errorf("no provider for model %q", modelName)
	}
	id, _ := p.Args["id"].(string)
	err := callCRUDDelete(provider, id)
	if err != nil {
		return false, err
	}
	return true, nil
}

func toCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func toPlural(s string) string {
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(s[len(s)-2]) {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func isVowel(b byte) bool {
	return b == 'a' || b == 'e' || b == 'i' || b == 'o' || b == 'u' ||
		b == 'A' || b == 'E' || b == 'I' || b == 'O' || b == 'U'
}
