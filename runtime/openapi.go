package runtime

import (
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/natuleadan/sdk-api/db"
)

// BuildOpenAPI generates an OpenAPI 3.0.3 spec from the service config and registered models.
func BuildOpenAPI(cfg *ServiceConfig, models map[string]*db.TableInfo) (*openapi3.T, error) {
	version := "1.0.0"
	prefix := cfg.Server.APIPrefix

	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   cfg.Name,
			Version: version,
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{},
		},
	}

	for _, entry := range cfg.Entry {
		switch entry.Type {
		case "crud":
			addCRUDPaths(doc, &entry, models, prefix)
		case "rest":
			addRestPath(doc, &entry, prefix)
		case "webhook":
			addRestPath(doc, &entry, prefix)
		case "websocket":
			addWSPath(doc, &entry, prefix)
		case "sse":
			addSSEPath(doc, &entry, prefix)
		case "file":
			addFilePath(doc, &entry, prefix)
		}
	}

	return doc, nil
}

func addCRUDPaths(doc *openapi3.T, entry *EntryDef, models map[string]*db.TableInfo, prefix string) {
	info := models[entry.Model]
	resource := entry.Resource
	if resource == "" {
		resource = plural(entry.Table)
	}
	base := prefix + "/" + resource

	// Register schema if model info available
	if info != nil {
		doc.Components.Schemas[entry.Model] = &openapi3.SchemaRef{Value: buildSchema(info)}
	}

	schemaRef := &openapi3.SchemaRef{Value: &openapi3.Schema{}}
	if info != nil {
		schemaRef = &openapi3.SchemaRef{Value: buildSchema(info)}
	}

	// GET list
	doc.Paths.Set(base, &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "List " + resource,
			OperationID: "list" + pascal(resource),
			Parameters: openapi3.Parameters{
				param("page", "query", "integer"),
				param("size", "query", "integer"),
				param("sort", "query", "string"),
			},
			Responses: responses200(schemaRef),
		},
		Post: &openapi3.Operation{
			Summary:     "Create " + resource,
			OperationID: "create" + pascal(resource),
			RequestBody: jsonBody(schemaRef),
			Responses:   responses201(schemaRef),
		},
	})

	// GET/PATCH/DELETE by ID
	idPath := base + "/:id"
	if strings.Contains(entry.Path, ":id") {
		idPath = prefix + entry.Path
	}
	doc.Paths.Set(idPath, &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "Get " + resource + " by ID",
			OperationID: "get" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses:   responses200(schemaRef),
		},
		Patch: &openapi3.Operation{
			Summary:     "Update " + resource,
			OperationID: "update" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses:   okResp(),
		},
		Delete: &openapi3.Operation{
			Summary:     "Delete " + resource,
			OperationID: "delete" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(204, &openapi3.ResponseRef{
					Value: &openapi3.Response{Description: pstr("Deleted")},
				}),
			),
		},
	})
}

func addRestPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	op := &openapi3.Operation{
		Summary:     entry.Handler,
		OperationID: entry.Handler,
		Responses:   okResp(),
	}
	doc.Paths.Set(path, &openapi3.PathItem{})
	switch entry.Method {
	case "GET":
		doc.Paths.Value(path).Get = op
	case "POST":
		doc.Paths.Value(path).Post = op
	case "PUT":
		doc.Paths.Value(path).Put = op
	case "PATCH":
		doc.Paths.Value(path).Patch = op
	case "DELETE":
		doc.Paths.Value(path).Delete = op
	}
}

func addWSPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	doc.Paths.Set(path, &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "WebSocket: " + entry.Handler,
			OperationID: entry.Handler,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(101, &openapi3.ResponseRef{
					Value: &openapi3.Response{Description: pstr("Switching Protocols")},
				}),
			),
		},
	})
}

func addSSEPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	doc.Paths.Set(path, &openapi3.PathItem{
		Get: &openapi3.Operation{
			Summary:     "SSE stream: " + entry.Handler,
			OperationID: entry.Handler,
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(200, &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: pstr("SSE stream"),
						Content: openapi3.NewContentWithJSONSchema(&openapi3.Schema{
							Type: oapiTypes("string"),
						}),
					},
				}),
			),
		},
	})
}

func addFilePath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	op := &openapi3.Operation{
		Summary:     "File: " + entry.Handler,
		OperationID: entry.Handler,
		Responses:   okResp(),
	}
	doc.Paths.Set(path, &openapi3.PathItem{})
	switch entry.Method {
	case "GET":
		doc.Paths.Value(path).Get = op
	case "POST":
		doc.Paths.Value(path).Post = op
	case "PUT":
		doc.Paths.Value(path).Put = op
	case "PATCH":
		doc.Paths.Value(path).Patch = op
	case "DELETE":
		doc.Paths.Value(path).Delete = op
	}
}

// ---- Schema builders ----

func buildSchema(info *db.TableInfo) *openapi3.Schema {
	s := &openapi3.Schema{
		Type:       oapiTypes("object"),
		Properties: openapi3.Schemas{},
	}
	for _, f := range info.Fields {
		if f.Skip {
			continue
		}
		prop := fieldToSchema(f.FieldType)
		jsonName := f.Column
		if tag := f.Tags.Get("json"); tag != "" {
			if name, _, _ := strings.Cut(tag, ","); name != "" && name != "-" {
				jsonName = name
			}
		}
		s.Properties[jsonName] = &openapi3.SchemaRef{Value: prop}
	}
	return s
}

func fieldToSchema(t reflect.Type) *openapi3.Schema {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &openapi3.Schema{Type: oapiTypes("integer")}
	case reflect.Float32, reflect.Float64:
		return &openapi3.Schema{Type: oapiTypes("number")}
	case reflect.String:
		return &openapi3.Schema{Type: oapiTypes("string")}
	case reflect.Bool:
		return &openapi3.Schema{Type: oapiTypes("boolean")}
	case reflect.Struct:
		if t.String() == "time.Time" {
			return &openapi3.Schema{Type: oapiTypes("string"), Format: "date-time"}
		}
		return &openapi3.Schema{Type: oapiTypes("object")}
	default:
		return &openapi3.Schema{Type: oapiTypes("string")}
	}
}

// ---- Helpers ----

func param(name, in, typ string) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{
		Value: &openapi3.Parameter{
			Name:   name,
			In:     in,
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: oapiTypes(typ)}},
		},
	}
}

func jsonBody(schema *openapi3.SchemaRef) *openapi3.RequestBodyRef {
	return &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Content: openapi3.NewContentWithJSONSchemaRef(schema),
		},
	}
}

func responses200(schema *openapi3.SchemaRef) *openapi3.Responses {
	return openapi3.NewResponses(
		openapi3.WithStatus(200, &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: pstr("OK"),
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}),
	)
}

func responses201(schema *openapi3.SchemaRef) *openapi3.Responses {
	return openapi3.NewResponses(
		openapi3.WithStatus(201, &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: pstr("Created"),
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}),
	)
}

func okResp() *openapi3.Responses {
	return openapi3.NewResponses(
		openapi3.WithStatus(200, &openapi3.ResponseRef{
			Value: &openapi3.Response{Description: pstr("OK")},
		}),
	)
}

func pascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func pstr(s string) *string { return &s }

func oapiTypes(s ...string) *openapi3.Types {
	t := openapi3.Types(s)
	return &t
}
