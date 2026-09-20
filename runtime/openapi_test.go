package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/natuleadan/sdk-api/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestProduct struct {
	ID    int64   `db:"id,primary,auto" json:"id"`
	Name  string  `db:"name,required" json:"name"`
	Price float64 `db:"price" json:"price"`
}

type TestError struct {
	Message string `db:"message" json:"message"`
	Code    string `db:"code" json:"code"`
}

func TestBuildOpenAPI_CRUD(t *testing.T) {
	info, err := db.ParseStructReflect(reflect.TypeFor[TestProduct]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}

	cfg := &ServiceConfig{
		Name: "test-svc",
		Server: ServerConf{
			APIPrefix: "/api/v1",
			OpenAPI:   &OpenAPIConf{Enabled: true, Version: "1.0.0"},
		},
		Entry: []EntryDef{
			{Type: "crud", Model: "Product", Table: "products", Resource: "products", Path: "/products"},
		},
	}

	models := map[string]*db.TableInfo{"Product": info}
	spec, err := BuildOpenAPI(cfg, models)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("OpenAPI version = %q", spec.OpenAPI)
	}
	if spec.Info.Title != "test-svc" {
		t.Errorf("Title = %q", spec.Info.Title)
	}

	// Verify CRUD paths exist
	paths := []string{"/api/v1/products", "/api/v1/products/:id"}
	for _, p := range paths {
		if spec.Paths.Find(p) == nil {
			t.Errorf("path %q not found", p)
		}
	}

	// Verify schema exists
	if _, ok := spec.Components.Schemas["Product"]; !ok {
		t.Error("Product schema not found in components")
	}
	schema := spec.Components.Schemas["Product"].Value
	if schema.Type == nil || schema.Type.Slice()[0] != "object" {
		t.Errorf("Product schema type = %v", schema.Type)
	}
	if _, ok := schema.Properties["name"]; !ok {
		t.Error("name property not found in Product schema")
	}
	if _, ok := schema.Properties["price"]; !ok {
		t.Error("price property not found in Product schema")
	}
}

func TestBuildOpenAPI_REST(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "rest-svc",
		Server: ServerConf{APIPrefix: "/api/v1"},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/ping", Handler: "ping"},
			{Type: "rest", Method: "POST", Path: "/data", Handler: "createData"},
		},
	}

	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	pingPath := spec.Paths.Find("/api/v1/ping")
	if pingPath == nil {
		t.Error("/api/v1/ping not found")
	} else if pingPath.Get == nil {
		t.Error("/api/v1/ping GET not found")
	}

	dataPath := spec.Paths.Find("/api/v1/data")
	if dataPath == nil {
		t.Error("/api/v1/data not found")
	} else if dataPath.Post == nil {
		t.Error("/api/v1/data POST not found")
	}
}

func TestBuildOpenAPI_Webhook(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "webhook-svc",
		Server: ServerConf{APIPrefix: ""},
		Entry: []EntryDef{
			{Type: "webhook", Method: "POST", Path: "/webhooks/test", Handler: "onWebhook"},
		},
	}

	spec, _ := BuildOpenAPI(cfg, nil)

	whPath := spec.Paths.Find("/webhooks/test")
	if whPath == nil {
		t.Fatal("/webhooks/test not found")
	}
	if whPath.Post == nil {
		t.Error("webhook should be POST")
	}
}

func TestBuildOpenAPI_WebSocket(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "ws-svc",
		Server: ServerConf{APIPrefix: ""},
		Entry: []EntryDef{
			{Type: "websocket", Path: "/ws/chat", Handler: "chat"},
		},
	}

	spec, _ := BuildOpenAPI(cfg, nil)

	wsPath := spec.Paths.Find("/ws/chat")
	if wsPath == nil {
		t.Fatal("/ws/chat not found")
	}
	if wsPath.Get == nil {
		t.Error("WS should be GET")
	}
	// Should have 101 Switching Protocols response
	if wsPath.Get.Responses.Value("101") == nil {
		t.Error("WS should have 101 response")
	}
}

func TestBuildOpenAPI_SSE(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "sse-svc",
		Server: ServerConf{APIPrefix: ""},
		Entry: []EntryDef{
			{Type: "sse", Path: "/events/stream", Handler: "stream"},
		},
	}

	spec, _ := BuildOpenAPI(cfg, nil)
	ssePath := spec.Paths.Find("/events/stream")
	if ssePath == nil {
		t.Fatal("/events/stream not found")
	}
	if ssePath.Get == nil {
		t.Error("SSE should be GET")
	}
}

func TestBuildOpenAPI_File(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "file-svc",
		Server: ServerConf{APIPrefix: ""},
		Entry: []EntryDef{
			{Type: "file", Method: "POST", Path: "/files/upload", Handler: "upload"},
			{Type: "file", Method: "GET", Path: "/files/:id/download", Handler: "download"},
		},
	}

	spec, _ := BuildOpenAPI(cfg, nil)

	uploadPath := spec.Paths.Find("/files/upload")
	if uploadPath == nil || uploadPath.Post == nil {
		t.Error("/files/upload POST not found")
	}
	downloadPath := spec.Paths.Find("/files/:id/download")
	if downloadPath == nil || downloadPath.Get == nil {
		t.Error("/files/:id/download GET not found")
	}
}

func TestBuildOpenAPI_MixedTypes(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "mixed",
		Server: ServerConf{APIPrefix: "/api/v1"},
		Entry: []EntryDef{
			{Type: "crud", Model: "Product", Table: "products", Resource: "products", Path: "/products"},
			{Type: "rest", Method: "GET", Path: "/health", Handler: "healthCheck"},
			{Type: "webhook", Method: "POST", Path: "/webhooks/github", Handler: "onPush"},
			{Type: "websocket", Path: "/ws/chat", Handler: "chatHandler"},
			{Type: "sse", Path: "/events/stream", Handler: "streamHandler"},
			{Type: "file", Method: "POST", Path: "/files/upload", Handler: "fileUpload"},
		},
	}

	info, err := db.ParseStructReflect(reflect.TypeFor[TestProduct]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}
	models := map[string]*db.TableInfo{"Product": info}

	spec, err := BuildOpenAPI(cfg, models)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	// Should have 6+ paths
	data, _ := json.Marshal(spec)
	jsonStr := string(data)

	for _, expected := range []string{"/products", "/health", "/webhooks/github", "/ws/chat", "/events/stream", "/files/upload"} {
		if !strings.Contains(jsonStr, expected) {
			t.Errorf("expected %q in spec", expected)
		}
	}

	// Product schema should exist
	if _, ok := spec.Components.Schemas["Product"]; !ok {
		t.Error("Product schema missing in mixed spec")
	}
}

func TestBuildOpenAPI_Empty(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "empty",
		Server: ServerConf{APIPrefix: "/api/v1"},
	}

	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	if spec.Paths.Len() != 0 {
		t.Errorf("expected 0 paths, got %d", spec.Paths.Len())
	}
}

func TestBuildSchema_Fields(t *testing.T) {
	info, err := db.ParseStructReflect(reflect.TypeFor[TestProduct]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}

	schema := buildSchema(info)
	if schema.Type.Slice()[0] != "object" {
		t.Errorf("type = %v", schema.Type)
	}

	// id field
	idProp := schema.Properties["id"]
	if idProp == nil {
		t.Error("id property missing")
	} else if idProp.Value.Type.Slice()[0] != "integer" {
		t.Errorf("id type = %v", idProp.Value.Type)
	}

	// name field
	nameProp := schema.Properties["name"]
	if nameProp == nil {
		t.Error("name property missing")
	} else if nameProp.Value.Type.Slice()[0] != "string" {
		t.Errorf("name type = %v", nameProp.Value.Type)
	}

	// price field
	priceProp := schema.Properties["price"]
	if priceProp == nil {
		t.Error("price property missing")
	} else if priceProp.Value.Type.Slice()[0] != "number" {
		t.Errorf("price type = %v", priceProp.Value.Type)
	}
}

func TestService_RegisterModel(t *testing.T) {
	svc := &Service{config: &ServiceConfig{Name: "test", Port: 19070}}

	svc.RegisterModel("Product", (*TestProduct)(nil))

	if svc.models == nil {
		t.Fatal("models map is nil")
	}
	if svc.models["Product"] == nil {
		t.Error("Product model not registered")
	}
	if svc.models["Product"].PrimaryKey != "id" {
		t.Errorf("PrimaryKey = %q", svc.models["Product"].PrimaryKey)
	}
}

func TestService_RegisterModel_InvalidType(t *testing.T) {
	svc := &Service{config: &ServiceConfig{Name: "test", Port: 19071}}
	svc.RegisterModel("Bad", "not a struct")
	if svc.models != nil && svc.models["Bad"] != nil {
		t.Error("should not register non-struct model")
	}
}

// ---- Spec completion: async, graphql, securitySchemes, servers, tags ----

func TestBuildOpenAPI_Async(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "async-svc",
		Port:   23198,
		Server: ServerConf{APIPrefix: "/api", Host: "0.0.0.0"},
		Entry: []EntryDef{
			{Type: "async", Path: "/jobs", Handler: "report"},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	want := []string{"/api/jobs", "/api/jobs/:job_id", "/api/jobs/:job_id/status"}
	for _, p := range want {
		if spec.Paths.Find(p) == nil {
			t.Errorf("async path %q missing", p)
		}
	}
	list := spec.Paths.Find("/api/jobs")
	if list.Get == nil || list.Post == nil {
		t.Error("async base path must expose POST submit and GET list")
	}
	job := spec.Paths.Find("/api/jobs/:job_id")
	if job.Get == nil || job.Delete == nil {
		t.Error("async job path must expose GET status and DELETE cancel")
	}
	sse := spec.Paths.Find("/api/jobs/:job_id/status")
	if sse == nil || sse.Get == nil {
		t.Error("async SSE status path missing")
	}
}

func TestBuildOpenAPI_AuthSchemes(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "auth-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/open", Handler: "open"},
			{Type: "rest", Method: "GET", Path: "/protected", Handler: "protected", AuthModes: []string{"jwt", "apikey"}},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	if spec.Components == nil || spec.Components.SecuritySchemes == nil {
		t.Fatal("securitySchemes missing")
	}
	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Error("bearerAuth scheme missing for jwt auth mode")
	}
	if _, ok := spec.Components.SecuritySchemes["apiKeyAuth"]; !ok {
		t.Error("apiKeyAuth scheme missing for apikey auth mode")
	}
	if spec.Security == nil || len(spec.Security) == 0 {
		t.Error("global security requirements missing")
	}
}

func TestBuildOpenAPI_PerOperationSecurity(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "sec-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{Type: "rest", Method: "POST", Path: "/login", Handler: "login"},
			{Type: "rest", Method: "GET", Path: "/profile", Handler: "profile", AuthModes: []string{"jwt"}},
			{Type: "rest", Method: "GET", Path: "/key-data", Handler: "keydata", AuthModes: []string{"jwt", "apikey"}},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	login := spec.Paths.Find("/api/login")
	if login == nil || login.Post == nil {
		t.Fatal("login operation missing")
	}
	if login.Post.Security == nil || len(*login.Post.Security) != 0 {
		t.Error("public operation must carry an explicitly empty security requirement")
	}

	profile := spec.Paths.Find("/api/profile")
	if profile == nil || profile.Get == nil {
		t.Fatal("profile operation missing")
	}
	if profile.Get.Security == nil || len(*profile.Get.Security) != 1 {
		t.Fatalf("protected operation must carry one requirement, got %+v", profile.Get.Security)
	}
	if _, ok := (*profile.Get.Security)[0]["bearerAuth"]; !ok {
		t.Errorf("jwt mode must map to bearerAuth, got %+v", (*profile.Get.Security)[0])
	}

	keydata := spec.Paths.Find("/api/key-data")
	if keydata == nil || keydata.Get == nil {
		t.Fatal("key-data operation missing")
	}
	if keydata.Get.Security == nil || len(*keydata.Get.Security) != 2 {
		t.Fatalf("dual-mode operation must carry two requirements, got %+v", keydata.Get.Security)
	}
}

func TestBuildOpenAPI_NoAuth(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "plain-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/ping", Handler: "ping"},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	if spec.Security != nil {
		t.Error("no-auth service must not declare security requirements")
	}
	if len(spec.Components.SecuritySchemes) != 0 {
		t.Error("no security schemes expected without auth modes")
	}
}

func TestBuildOpenAPI_ServersBlock(t *testing.T) {
	cfg := &ServiceConfig{
		Name: "srv-svc",
		Port: 23197,
		Server: ServerConf{
			Host:      "0.0.0.0",
			APIPrefix: "/api",
		},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/ping", Handler: "ping"},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	if len(spec.Servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(spec.Servers))
	}
	if spec.Servers[0].URL != "http://localhost:23197" {
		t.Errorf("server url = %q, want http://localhost:23197 (0.0.0.0 → localhost)", spec.Servers[0].URL)
	}
}

func TestBuildOpenAPI_TagsAndDocs(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "tagged-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{
				Type:        "rest",
				Method:      "GET",
				Path:        "/ping",
				Handler:     "ping",
				Summary:     "Ping the service",
				Description: "Returns pong with latency info",
			},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	item := spec.Paths.Find("/api/ping")
	if item == nil || item.Get == nil {
		t.Fatal("/api/ping GET missing")
	}
	if len(item.Get.Tags) != 1 || item.Get.Tags[0] != "rest" {
		t.Errorf("tags = %v, want [rest]", item.Get.Tags)
	}
	if item.Get.Summary != "Ping the service" {
		t.Errorf("summary = %q, want entry summary", item.Get.Summary)
	}
	if item.Get.Description != "Returns pong with latency info" {
		t.Errorf("description = %q", item.Get.Description)
	}
}

func TestBuildOpenAPI_GraphQL(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "gql-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{Type: "graphql", Path: "/graphql", Handler: "graphqlHandler"},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	item := spec.Paths.Find("/api/graphql")
	if item == nil || item.Post == nil {
		t.Fatal("/api/graphql POST missing")
	}
}

// TestOperationDocs_BodyTitleDescription asserts the contract infra asked for:
// every non-CRUD operation documents a summary (title), a description, and a
// response for every status it can return — including a body for write
// methods. Guards against regressions where a new entry type silently ships
// schema-less, undocumented operations.
func TestOperationDocs_BodyTitleDescription(t *testing.T) {
	info, err := db.ParseStructReflect(reflect.TypeFor[TestProduct]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}
	cfg := &ServiceConfig{
		Name:   "docs-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{
				Type:          "rest",
				Method:        "POST",
				Path:          "/widgets",
				Handler:       "createWidget",
				Summary:       "Create a widget",
				Description:   "Creates a widget and returns it.",
				RequestModel:  "Product",
				ResponseModel: "Product",
				Responses: map[string]string{
					"400": "Invalid payload",
					"401": "Missing or invalid token",
					"500": "Internal error",
				},
				Tags: []string{"widgets"},
			},
			{
				Type:          "rest",
				Method:        "GET",
				Path:          "/widgets/:id",
				Handler:       "getWidget",
				Summary:       "Get a widget",
				Description:   "Returns a single widget by ID.",
				ResponseModel: "Product",
				Responses:     map[string]string{"404": "Widget not found"},
			},
		},
	}
	models := map[string]*db.TableInfo{"Product": info}
	spec, err := BuildOpenAPI(cfg, models)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}

	post := spec.Paths.Find("/api/widgets").Post
	if post == nil {
		t.Fatal("POST /api/widgets missing")
	}
	if post.Summary == "" {
		t.Error("POST: missing summary (title)")
	}
	if post.Description == "" {
		t.Error("POST: missing description")
	}
	if len(post.Tags) != 1 || post.Tags[0] != "widgets" {
		t.Errorf("POST: tags = %v, want [widgets]", post.Tags)
	}
	if post.RequestBody == nil {
		t.Error("POST: missing request body")
	}
	if _, ok := post.Responses.Map()["201"]; !ok {
		t.Error("POST: missing 201 success response")
	}
	for _, code := range []string{"400", "401", "500"} {
		if _, ok := post.Responses.Map()[code]; !ok {
			t.Errorf("POST: missing %s response", code)
		}
	}
	// The body must be a $ref into components.schemas, populated even though
	// Product is not a CRUD entry.
	if _, ok := spec.Components.Schemas["Product"]; !ok {
		t.Error("Product schema not registered in components")
	}

	get := spec.Paths.Find("/api/widgets/:id").Get
	if get == nil {
		t.Fatal("GET /api/widgets/:id missing")
	}
	if get.RequestBody != nil {
		t.Error("GET: request body must not be set")
	}
	if _, ok := get.Responses.Map()["200"]; !ok {
		t.Error("GET: missing 200 success response")
	}
	if _, ok := get.Responses.Map()["404"]; !ok {
		t.Error("GET: missing documented 404 response")
	}
}

// TestOperationDocs_UnknownModelDegrades softens the contract: an entry that
// names an unregistered model must still produce a valid operation rather
// than panic or drop the path.
func TestOperationDocs_UnknownModelDegrades(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "degrade-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{
				Type:          "rest",
				Method:        "POST",
				Path:          "/things",
				Handler:       "createThing",
				Summary:       "Create a thing",
				Description:   "Creates a thing.",
				RequestModel:  "DoesNotExist",
				ResponseModel: "DoesNotExist",
				Responses:     map[string]string{"500": "Internal error"},
			},
		},
	}
	spec, err := BuildOpenAPI(cfg, nil)
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	post := spec.Paths.Find("/api/things").Post
	if post == nil {
		t.Fatal("POST /api/things missing")
	}
	if post.RequestBody != nil {
		t.Error("unknown request model: body must be omitted, not malformed")
	}
	if _, ok := post.Responses.Map()["201"]; !ok {
		t.Error("unknown response model: success response still required")
	}
}

// TestOperationDocs_SharedPathKeepsBothMethods guards the regression infra hit:
// two entries on the same path (GET + POST /subjects) must both survive in the
// spec. Before the merge helper the later entry replaced the whole PathItem.
func TestOperationDocs_SharedPathKeepsBothMethods(t *testing.T) {
	info, err := db.ParseStructReflect(reflect.TypeFor[TestProduct]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}
	cfg := &ServiceConfig{
		Name:   "shared-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{Type: "rest", Method: "POST", Path: "/subjects", Handler: "CreateSubject", Summary: "C", Description: "d", RequestModel: "Product"},
			{Type: "rest", Method: "GET", Path: "/subjects", Handler: "ListSubjects", Summary: "L", Description: "d"},
			{Type: "rest", Method: "GET", Path: "/subjects/:id", Handler: "GetSubject", Summary: "G", Description: "d"},
			{Type: "rest", Method: "PATCH", Path: "/subjects/:id", Handler: "LinkExternalId", Summary: "P", Description: "d"},
		},
	}
	spec, err := BuildOpenAPI(cfg, map[string]*db.TableInfo{"Product": info})
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	subjects := spec.Paths.Find("/api/subjects")
	if subjects == nil || subjects.Get == nil || subjects.Post == nil {
		t.Fatalf("shared path lost a method: %+v", subjects)
	}
	if subjects.Get.OperationID != "ListSubjects" || subjects.Post.OperationID != "CreateSubject" {
		t.Errorf("wrong op ids: get=%q post=%q", subjects.Get.OperationID, subjects.Post.OperationID)
	}
	byID := spec.Paths.Find("/api/subjects/:id")
	if byID == nil || byID.Get == nil || byID.Patch == nil {
		t.Fatalf("shared path /:id lost a method: %+v", byID)
	}
}

// TestOperationDocs_ErrorModelOnFailures documents the shared error envelope:
// when entry.error_model names a registered model, every declared 4xx/5xx
// response carries its schema, while the success response does not.
func TestOperationDocs_ErrorModelOnFailures(t *testing.T) {
	info, err := db.ParseStructReflect(reflect.TypeFor[TestError]())
	if err != nil {
		t.Fatalf("ParseStructReflect: %v", err)
	}
	cfg := &ServiceConfig{
		Name:   "err-svc",
		Server: ServerConf{APIPrefix: "/api"},
		Entry: []EntryDef{
			{
				Type:        "rest",
				Method:      "POST",
				Path:        "/things",
				Handler:     "createThing",
				Summary:     "Create a thing",
				Description: "Creates a thing.",
				ErrorModel:  "ErrorEnvelope",
				Responses: map[string]string{
					"400": "Invalid payload",
					"401": "Missing token",
					"500": "Internal error",
				},
			},
		},
	}
	spec, err := BuildOpenAPI(cfg, map[string]*db.TableInfo{"ErrorEnvelope": info})
	if err != nil {
		t.Fatalf("BuildOpenAPI: %v", err)
	}
	post := spec.Paths.Find("/api/things").Post
	if _, ok := spec.Components.Schemas["ErrorEnvelope"]; !ok {
		t.Fatal("error model not registered in components")
	}
	for _, code := range []string{"400", "401", "500"} {
		ref := post.Responses.Map()[code]
		if ref == nil || ref.Value == nil || ref.Value.Content == nil {
			t.Errorf("%s: missing error body", code)
			continue
		}
		mt := ref.Value.Content["application/json"]
		if mt == nil || mt.Schema == nil || mt.Schema.Ref != "#/components/schemas/ErrorEnvelope" {
			t.Errorf("%s: error body not a $ref to ErrorEnvelope", code)
		}
	}
	okRef := post.Responses.Map()["201"]
	if okRef.Value != nil && okRef.Value.Content != nil {
		t.Error("201 must not carry the error envelope")
	}
}

func TestBuildOpenAPI_HiddenEntry(t *testing.T) {
	cfg := &ServiceConfig{
		Name:   "svc",
		Server: ServerConf{APIPrefix: "/v1", OpenAPI: &OpenAPIConf{Enabled: true}},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/public", Handler: "pub", Summary: "Public", AuthModes: []string{"jwt"}},
			{Type: "rest", Method: "PUT", Path: "/system/secret", Handler: "sys", Summary: "System", Hidden: true, AuthModes: []string{"apikey"}},
		},
	}
	spec, err := BuildOpenAPI(cfg, map[string]*db.TableInfo{})
	require.NoError(t, err)

	assert.NotNil(t, spec.Paths.Find("/v1/public"))
	assert.Nil(t, spec.Paths.Find("/v1/system/secret"), "hidden path leaked into the spec")

	_, hasAPIKey := spec.Components.SecuritySchemes["apiKeyAuth"]
	assert.False(t, hasAPIKey, "hidden entry apikey scheme leaked into the spec")
	_, hasBearer := spec.Components.SecuritySchemes["bearerAuth"]
	assert.True(t, hasBearer, "visible entry bearer scheme missing")
}

func TestBuildOpenAPI_ExcludePathsAndTags(t *testing.T) {
	cfg := &ServiceConfig{
		Name: "svc",
		Server: ServerConf{APIPrefix: "/v1", OpenAPI: &OpenAPIConf{
			Enabled:      true,
			ExcludePaths: []string{"/v1/system/*", "/v1/internal"},
			ExcludeTags:  []string{"Internal"},
		}},
		Entry: []EntryDef{
			{Type: "rest", Method: "GET", Path: "/public", Handler: "pub", Summary: "Public"},
			{Type: "rest", Method: "GET", Path: "/system/a", Handler: "a", Summary: "A"},
			{Type: "rest", Method: "GET", Path: "/system/b", Handler: "b", Summary: "B"},
			{Type: "rest", Method: "GET", Path: "/internal", Handler: "c", Summary: "C"},
			{Type: "rest", Method: "GET", Path: "/hidden-by-tag", Handler: "d", Summary: "D", Tags: []string{"Internal"}},
		},
	}
	spec, err := BuildOpenAPI(cfg, map[string]*db.TableInfo{})
	require.NoError(t, err)

	assert.NotNil(t, spec.Paths.Find("/v1/public"))
	for _, p := range []string{"/v1/system/a", "/v1/system/b", "/v1/internal", "/v1/hidden-by-tag"} {
		assert.Nil(t, spec.Paths.Find(p), "path %q should have been excluded", p)
	}
}
