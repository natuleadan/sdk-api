package runtime

import (
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strconv"
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
			// Extensions is pre-initialized so SpecMutator hooks can attach
			// x-* markers without nil-map panics.
			Extensions: map[string]any{},
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{},
		},
	}

	for i := range cfg.Entry {
		entry := &cfg.Entry[i]
		if entry.Hidden {
			continue
		}
		addEntryPaths(doc, entry, models, prefix)
	}
	applyEntrySecurity(doc, cfg.Entry, prefix)

	doc.Security = securityRequirements(cfg.Entry)
	sessionCookie := "sid"
	if cfg.Auth != nil && cfg.Auth.Session != nil && cfg.Auth.Session.Cookie != "" {
		sessionCookie = cfg.Auth.Session.Cookie
	}
	if schemes := buildSecuritySchemes(cfg.Entry, sessionCookie); len(schemes) > 0 {
		doc.Components.SecuritySchemes = schemes
	}
	if server := specServer(cfg); server != nil {
		doc.Servers = openapi3.Servers{server}
	}

	applySpecExclusions(doc, cfg)

	return doc, nil
}

// addEntryPaths renders the OpenAPI paths for one entry type. Extracted from
// BuildOpenAPI to keep its cyclomatic complexity in check.
func addEntryPaths(doc *openapi3.T, entry *EntryDef, models map[string]*db.TableInfo, prefix string) {
	switch entry.Type {
	case "crud":
		addCRUDPaths(doc, entry, models, prefix)
	case "rest", "webhook":
		addRestPath(doc, entry, models, prefix)
	case "websocket":
		addWSPath(doc, entry, prefix)
	case "sse":
		addSSEPath(doc, entry, prefix)
	case "file":
		addFilePath(doc, entry, prefix)
	case "async":
		addAsyncPaths(doc, entry, prefix)
	case "graphql":
		addGraphQLPath(doc, entry, prefix)
	}
}

// applyEntrySecurity stamps every documented operation with the security
// requirement of the entry that produced it, so readers see per-route locks:
// protected routes name their schemes, public routes carry an explicitly empty
// requirement instead of inheriting the document-level one. Entries that share
// a path keep the first writer's requirement, mirroring the pathItem merge.
func applyEntrySecurity(doc *openapi3.T, entries []EntryDef, prefix string) {
	type opKey struct {
		method string
		path   string
	}
	modes := map[opKey][]string{}
	remember := func(method, path string, entryModes []string) {
		key := opKey{method: strings.ToUpper(method), path: path}
		if _, ok := modes[key]; !ok {
			modes[key] = entryModes
		}
	}
	for i := range entries {
		entry := &entries[i]
		if entry.Hidden {
			continue
		}
		switch entry.Type {
		case "rest", "webhook":
			if entry.Method != "" {
				remember(entry.Method, prefix+entry.Path, entry.AuthModes)
			}
		case "crud":
			resource := entry.Resource
			if resource == "" {
				resource = plural(entry.Table)
			}
			base := prefix + "/" + resource
			remember("GET", base, entry.AuthModes)
			remember("POST", base, entry.AuthModes)
			idPath := base + "/:id"
			if strings.Contains(entry.Path, ":id") {
				idPath = prefix + entry.Path
			}
			remember("GET", idPath, entry.AuthModes)
			remember("PATCH", idPath, entry.AuthModes)
			remember("DELETE", idPath, entry.AuthModes)
		}
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range map[string]*openapi3.Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put,
			"PATCH": item.Patch, "DELETE": item.Delete,
		} {
			if op == nil {
				continue
			}
			if entryModes, ok := modes[opKey{method: method, path: path}]; ok {
				op.Security = requirementForModes(entryModes)
			}
		}
	}
}

// applySpecExclusions prunes the generated spec without touching the runtime
// routes: entries marked hidden never reach the spec, path patterns listed in
// openapi.exclude_paths are removed (a trailing "*" matches a prefix) and any
// operation carrying an openapi.exclude_tags tag is dropped. A path left with
// no operations is removed as well.
func applySpecExclusions(doc *openapi3.T, cfg *ServiceConfig) {
	if cfg.Server.OpenAPI == nil || doc.Paths == nil {
		return
	}
	oai := cfg.Server.OpenAPI
	for _, pattern := range oai.ExcludePaths {
		pruneSpecPaths(doc, pattern)
	}
	if len(oai.ExcludeTags) == 0 {
		return
	}
	excluded := make(map[string]bool, len(oai.ExcludeTags))
	for _, tag := range oai.ExcludeTags {
		excluded[tag] = true
	}
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if operationHasTag(op, excluded) {
				item.SetOperation(method, nil)
			}
		}
		if len(item.Operations()) == 0 {
			doc.Paths.Delete(path)
		}
	}
}

// pruneSpecPaths removes one path pattern: exact match, or prefix when the
// pattern ends with "*".
func pruneSpecPaths(doc *openapi3.T, pattern string) {
	if pattern == "" {
		return
	}
	if before, ok := strings.CutSuffix(pattern, "*"); ok {
		prefix := before
		for path := range doc.Paths.Map() {
			if strings.HasPrefix(path, prefix) {
				doc.Paths.Delete(path)
			}
		}
		return
	}
	doc.Paths.Delete(pattern)
}

func operationHasTag(op *openapi3.Operation, excluded map[string]bool) bool {
	if op == nil {
		return false
	}
	for _, tag := range op.Tags {
		if excluded[tag] {
			return true
		}
	}
	return false
}

// authModeSchemes is the single mapping from entry auth modes to OpenAPI
// security scheme names, shared by the global requirements, the per-operation
// pass and the scheme definitions below.
var authModeSchemes = []struct {
	mode string
	name string
}{
	{"jwt", "bearerAuth"},
	{"apikey", "apiKeyAuth"},
	{"basic", "basicAuth"},
	{"oauth", "oauthAuth"},
	{"session", "sessionAuth"},
}

// requirementForModes maps entry auth modes onto one OpenAPI security
// requirement. The result is always non-nil: an empty requirement means the
// operation is openly accessible.
func requirementForModes(modes []string) *openapi3.SecurityRequirements {
	reqs := make(openapi3.SecurityRequirements, 0, len(modes))
	for _, m := range authModeSchemes {
		if slices.Contains(modes, m.mode) {
			reqs = append(reqs, openapi3.SecurityRequirement{m.name: []string{}})
		}
	}
	return &reqs
}

// securityRequirements derives the operation-level security requirements from
// the union of all entry auth modes. Entries without auth stay public.
func securityRequirements(entries []EntryDef) openapi3.SecurityRequirements {
	has := map[string]bool{}
	for _, entry := range entries {
		if entry.Hidden {
			continue
		}
		for _, mode := range entry.AuthModes {
			has[mode] = true
		}
	}
	modes := make([]string, 0, len(has))
	for mode := range has {
		modes = append(modes, mode)
	}
	if len(modes) == 0 {
		return nil
	}
	return *requirementForModes(modes)
}

// buildSecuritySchemes maps the entry auth modes onto OpenAPI security
// schemes: jwt → HTTP bearer, apikey → apiKey header (Authorization),
// basic → HTTP basic, oauth → HTTP bearer (opaque token), session → apiKey
// cookie (name from auth.session.cookie).
func buildSecuritySchemes(entries []EntryDef, sessionCookie string) openapi3.SecuritySchemes {
	has := map[string]bool{}
	for _, entry := range entries {
		if entry.Hidden {
			continue
		}
		for _, mode := range entry.AuthModes {
			has[mode] = true
		}
	}
	if len(has) == 0 {
		return nil
	}
	schemes := openapi3.SecuritySchemes{}
	if has["jwt"] {
		schemes["bearerAuth"] = &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "JWT bearer token from the auth service",
		}}
	}
	if has["apikey"] {
		schemes["apiKeyAuth"] = &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "Authorization",
			Description: "API key (default header: Authorization)",
		}}
	}
	if has["basic"] {
		schemes["basicAuth"] = &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:        "http",
			Scheme:      "basic",
			Description: "HTTP Basic credentials (RFC 7617)",
		}}
	}
	if has["oauth"] {
		schemes["oauthAuth"] = &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
			Description:  "Opaque OAuth access token (RFC 7662 introspection)",
		}}
	}
	if has["session"] {
		if sessionCookie == "" {
			sessionCookie = "sid"
		}
		schemes["sessionAuth"] = &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:        "apiKey",
			In:          "cookie",
			Name:        sessionCookie,
			Description: "Server-side session cookie",
		}}
	}
	return schemes
}

// specServer builds the servers block from the server host/port config.
func specServer(cfg *ServiceConfig) *openapi3.Server {
	if cfg.Server.Host == "" {
		return nil
	}
	host := cfg.Server.Host
	if host == "0.0.0.0" {
		host = "localhost"
	}
	u := "http://" + host
	if cfg.Port != 0 {
		u += fmt.Sprintf(":%d", cfg.Port)
	}
	return &openapi3.Server{URL: u, Description: "This service"}
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
	baseItem := pathItem(doc, base)
	if baseItem.Get == nil {
		baseItem.Get = &openapi3.Operation{
			Summary:     "List " + resource,
			OperationID: "list" + pascal(resource),
			Parameters: openapi3.Parameters{
				param("page", "query", "integer"),
				param("size", "query", "integer"),
				param("cursor", "query", "string"),
				param("sort", "query", "string"),
			},
			Responses: responses200(schemaRef),
		}
	}
	if baseItem.Post == nil {
		baseItem.Post = &openapi3.Operation{
			Summary:     "Create " + resource,
			OperationID: "create" + pascal(resource),
			RequestBody: jsonBody(schemaRef),
			Responses:   responses201(schemaRef),
		}
	}

	// GET/PATCH/DELETE by ID
	idPath := base + "/:id"
	if strings.Contains(entry.Path, ":id") {
		idPath = prefix + entry.Path
	}
	idItem := pathItem(doc, idPath)
	if idItem.Get == nil {
		idItem.Get = &openapi3.Operation{
			Summary:     "Get " + resource + " by ID",
			OperationID: "get" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses:   responses200(schemaRef),
		}
	}
	if idItem.Patch == nil {
		idItem.Patch = &openapi3.Operation{
			Summary:     "Update " + resource,
			OperationID: "update" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses:   okResp(),
		}
	}
	if idItem.Delete == nil {
		idItem.Delete = &openapi3.Operation{
			Summary:     "Delete " + resource,
			OperationID: "delete" + pascal(resource),
			Parameters:  openapi3.Parameters{param("id", "path", "string")},
			Responses: openapi3.NewResponses(
				openapi3.WithStatus(204, &openapi3.ResponseRef{
					Value: &openapi3.Response{Description: new("Deleted")},
				}),
			),
		}
	}
}

func addRestPath(doc *openapi3.T, entry *EntryDef, models map[string]*db.TableInfo, prefix string) {
	path := prefix + entry.Path
	summary := entry.Summary
	if summary == "" {
		summary = entry.Handler
	}
	tags := entry.Tags
	if len(tags) == 0 {
		tags = []string{entry.Type}
	}

	var reqSchema *openapi3.SchemaRef
	if entry.RequestModel != "" {
		reqSchema = registerModelSchema(doc, entry.RequestModel, models)
	}
	var respSchema *openapi3.SchemaRef
	if entry.ResponseModel != "" {
		respSchema = registerModelSchema(doc, entry.ResponseModel, models)
	}

	op := &openapi3.Operation{
		Summary:     summary,
		OperationID: entry.Handler,
		Tags:        tags,
		Responses:   operationResponses(doc, entry, respSchema, models),
	}
	if entry.Description != "" {
		op.Description = entry.Description
	}
	if entry.Method != "" && entry.Method != "GET" && entry.Method != "DELETE" && reqSchema != nil {
		op.RequestBody = jsonBody(reqSchema)
	}
	// Merge into any existing PathItem: several entries may share one path
	// (e.g. GET + POST on /subjects), and each contributes its own method.
	item := pathItem(doc, path)
	switch entry.Method {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "PATCH":
		item.Patch = op
	case "DELETE":
		item.Delete = op
	}
}

// registerModelSchema registers a named model in components.schemas (once) and
// returns a reference to it. Unknown names return nil so the caller can fall
// back to a schema-less operation.
func registerModelSchema(doc *openapi3.T, name string, models map[string]*db.TableInfo) *openapi3.SchemaRef {
	info := models[name]
	if info == nil {
		return nil
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = openapi3.Schemas{}
	}
	if _, ok := doc.Components.Schemas[name]; !ok {
		doc.Components.Schemas[name] = &openapi3.SchemaRef{Value: buildSchema(info)}
	}
	return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name}
}

// operationResponses builds the responses object for a non-CRUD operation:
// a success code (200 for GET/PUT/PATCH/DELETE, 201 for POST) plus every
// status documented in entry.Responses, sorted for stable output. When
// entry.ErrorModel names a registered model, every non-2xx response carries
// that schema as its body (the shared error envelope).
func operationResponses(doc *openapi3.T, entry *EntryDef, successSchema *openapi3.SchemaRef, models map[string]*db.TableInfo) *openapi3.Responses {
	success := 200
	successDesc := "OK"
	if entry.Method == "POST" {
		success = 201
		successDesc = "Created"
	}
	successResp := &openapi3.Response{Description: new(successDesc)}
	if successSchema != nil {
		successResp.Content = openapi3.NewContentWithJSONSchemaRef(successSchema)
	}
	responses := openapi3.NewResponses(
		openapi3.WithStatus(success, &openapi3.ResponseRef{Value: successResp}),
	)
	var errSchema *openapi3.SchemaRef
	if entry.ErrorModel != "" {
		errSchema = registerModelSchema(doc, entry.ErrorModel, models)
	}
	codes := make([]string, 0, len(entry.Responses))
	for code := range entry.Responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		n, err := strconv.Atoi(code)
		if err != nil {
			continue
		}
		desc := entry.Responses[code]
		if desc == "" {
			desc = http.StatusText(n)
		}
		resp := &openapi3.Response{Description: new(desc)}
		if errSchema != nil && n >= 400 {
			resp.Content = openapi3.NewContentWithJSONSchemaRef(errSchema)
		}
		responses.Set(code, &openapi3.ResponseRef{Value: resp})
	}
	return responses
}

// pathItem returns the PathItem for a path, creating it on first use. Entries
// that share a path (GET + POST on /subjects, etc.) each contribute their own
// method; without this the last entry would replace the whole PathItem and
// silently drop the earlier operations from the spec.
func pathItem(doc *openapi3.T, path string) *openapi3.PathItem {
	if existing := doc.Paths.Value(path); existing != nil {
		return existing
	}
	doc.Paths.Set(path, &openapi3.PathItem{})
	return doc.Paths.Value(path)
}

func addWSPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	pathItem(doc, path).Get = &openapi3.Operation{
		Summary:     "WebSocket: " + entry.Handler,
		OperationID: entry.Handler,
		Responses: openapi3.NewResponses(
			openapi3.WithStatus(101, &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: new("Switching Protocols")},
			}),
		),
	}
}

func addSSEPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	pathItem(doc, path).Get = &openapi3.Operation{
		Summary:     "SSE stream: " + entry.Handler,
		OperationID: entry.Handler,
		Responses: openapi3.NewResponses(
			openapi3.WithStatus(200, &openapi3.ResponseRef{
				Value: &openapi3.Response{
					Description: new("SSE stream"),
					Content: openapi3.NewContentWithJSONSchema(&openapi3.Schema{
						Type: oapiTypes("string"),
					}),
				},
			}),
		),
	}
}

func addFilePath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	op := &openapi3.Operation{
		Summary:     "File: " + entry.Handler,
		OperationID: entry.Handler,
		Responses:   okResp(),
	}
	switch entry.Method {
	case "GET":
		pathItem(doc, path).Get = op
	case "POST":
		pathItem(doc, path).Post = op
	case "PUT":
		pathItem(doc, path).Put = op
	case "PATCH":
		pathItem(doc, path).Patch = op
	case "DELETE":
		pathItem(doc, path).Delete = op
	}
}

// addAsyncPaths documents the routes auto-registered for async entries:
// submit, list, status, cancel and the SSE status stream.
func addAsyncPaths(doc *openapi3.T, entry *EntryDef, prefix string) {
	base := prefix + entry.Path

	submit := &openapi3.Operation{
		Summary:     "Submit " + entry.Handler + " job",
		OperationID: "submit" + pascal(entry.Handler),
		Tags:        []string{"async"},
		Responses: openapi3.NewResponses(
			openapi3.WithStatus(202, &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: new("Job accepted")},
			}),
		),
	}
	if entry.Description != "" {
		submit.Description = entry.Description
	}

	baseItem := pathItem(doc, base)
	if baseItem.Post == nil {
		baseItem.Post = submit
	}
	if baseItem.Get == nil {
		baseItem.Get = &openapi3.Operation{
			Summary:     "List " + entry.Handler + " jobs",
			OperationID: "list" + pascal(entry.Handler) + "Jobs",
			Tags:        []string{"async"},
			Responses:   okResp(),
		}
	}

	jobPath := base + "/:job_id"
	jobItem := pathItem(doc, jobPath)
	if jobItem.Get == nil {
		jobItem.Get = &openapi3.Operation{
			Summary:     "Get " + entry.Handler + " job status",
			OperationID: "get" + pascal(entry.Handler) + "Job",
			Tags:        []string{"async"},
			Parameters:  openapi3.Parameters{param("job_id", "path", "string")},
			Responses:   okResp(),
		}
	}
	if jobItem.Delete == nil {
		jobItem.Delete = &openapi3.Operation{
			Summary:     "Cancel " + entry.Handler + " job",
			OperationID: "cancel" + pascal(entry.Handler) + "Job",
			Tags:        []string{"async"},
			Parameters:  openapi3.Parameters{param("job_id", "path", "string")},
			Responses:   okResp(),
		}
	}

	ssePath := jobPath + "/status"
	sseItem := pathItem(doc, ssePath)
	if sseItem.Get == nil {
		sseItem.Get = &openapi3.Operation{
			Summary:     "Stream " + entry.Handler + " job status (SSE)",
			OperationID: "stream" + pascal(entry.Handler) + "JobStatus",
			Tags:        []string{"async", "sse"},
			Parameters:  openapi3.Parameters{param("job_id", "path", "string")},
			Responses:   okResp(),
		}
	}
}

// addGraphQLPath documents a GraphQL entry as a POST operation.
func addGraphQLPath(doc *openapi3.T, entry *EntryDef, prefix string) {
	path := prefix + entry.Path
	summary := entry.Summary
	if summary == "" {
		summary = "GraphQL: " + entry.Handler
	}
	querySchema := &openapi3.Schema{
		Type: oapiTypes("object"),
		Properties: openapi3.Schemas{
			"query":     {Value: &openapi3.Schema{Type: oapiTypes("string")}},
			"variables": {Value: &openapi3.Schema{Type: oapiTypes("object")}},
		},
	}
	op := &openapi3.Operation{
		Summary:     summary,
		OperationID: entry.Handler,
		Tags:        []string{"graphql"},
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Content: openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Value: querySchema}),
			},
		},
		Responses: okResp(),
	}
	if entry.Description != "" {
		op.Description = entry.Description
	}
	doc.Paths.Set(path, &openapi3.PathItem{Post: op})
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
	if t.Kind() == reflect.Pointer {
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
				Description: new("OK"),
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}),
	)
}

func responses201(schema *openapi3.SchemaRef) *openapi3.Responses {
	return openapi3.NewResponses(
		openapi3.WithStatus(201, &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: new("Created"),
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}),
	)
}

func okResp() *openapi3.Responses {
	return openapi3.NewResponses(
		openapi3.WithStatus(200, &openapi3.ResponseRef{
			Value: &openapi3.Response{Description: new("OK")},
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

//go:fix inline

func oapiTypes(s ...string) *openapi3.Types {
	t := openapi3.Types(s)
	return &t
}
