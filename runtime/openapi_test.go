package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/natuleadan/sdk-api/db"
)

type TestProduct struct {
	ID    int64   `db:"id,primary,auto" json:"id"`
	Name  string  `db:"name,required" json:"name"`
	Price float64 `db:"price" json:"price"`
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
		Name: "auth-svc",
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
		Name: "tagged-svc",
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
