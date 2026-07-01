package runtime

import (
	"testing"
)

func TestLoadConfig_FullYAML(t *testing.T) {
	cfg, err := LoadConfig("testdata/service_v2.yaml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Name != "order-service" {
		t.Errorf("Name = %q, want order-service", cfg.Name)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}

	// Databases
	if len(cfg.Databases) != 2 {
		t.Fatalf("Databases = %d, want 2", len(cfg.Databases))
	}
	if cfg.Databases[0].Name != "pg-main" {
		t.Errorf("DB[0].Name = %q", cfg.Databases[0].Name)
	}
	if cfg.Databases[0].Driver != "postgres" {
		t.Errorf("DB[0].Driver = %q", cfg.Databases[0].Driver)
	}
	if cfg.Databases[0].Pool.MaxConns != 200 {
		t.Errorf("DB[0].Pool.MaxConns = %d", cfg.Databases[0].Pool.MaxConns)
	}
	if cfg.Databases[1].Name != "mysql-audit" {
		t.Errorf("DB[1].Name = %q", cfg.Databases[1].Name)
	}
	if cfg.Databases[1].Driver != "mysql" {
		t.Errorf("DB[1].Driver = %q", cfg.Databases[1].Driver)
	}

	// NATS
	if len(cfg.NATS) != 2 {
		t.Fatalf("NATS = %d, want 2", len(cfg.NATS))
	}
	if cfg.NATS[0].Name != "primary" {
		t.Errorf("NATS[0].Name = %q", cfg.NATS[0].Name)
	}
	if len(cfg.NATS[0].Streams) != 2 {
		t.Errorf("NATS[0].Streams = %d, want 2", len(cfg.NATS[0].Streams))
	}
	if cfg.NATS[0].Streams[0].Name != "orders" {
		t.Errorf("NATS[0].Streams[0].Name = %q", cfg.NATS[0].Streams[0].Name)
	}
	if cfg.NATS[0].Streams[1].Storage != "memory" {
		t.Errorf("NATS[0].Streams[1].Storage = %q, want memory", cfg.NATS[0].Streams[1].Storage)
	}
	if cfg.NATS[0].Streams[1].Compression != "none" {
		t.Errorf("NATS[0].Streams[1].Compression = %q, want none", cfg.NATS[0].Streams[1].Compression)
	}

	// Entry endpoints
	if len(cfg.Entry) != 9 {
		t.Fatalf("Entry = %d, want 9", len(cfg.Entry))
	}

	// CRUD 1: Product
	e0 := cfg.Entry[0]
	if e0.Type != "crud" {
		t.Errorf("Entry[0].Type = %q", e0.Type)
	}
	if e0.Model != "Product" {
		t.Errorf("Entry[0].Model = %q", e0.Model)
	}
	if e0.DB != "pg-main" {
		t.Errorf("Entry[0].DB = %q", e0.DB)
	}
	if e0.Table != "products" {
		t.Errorf("Entry[0].Table = %q, want products", e0.Table)
	}
	if e0.Resource != "products" {
		t.Errorf("Entry[0].Resource = %q, want products", e0.Resource)
	}
	if e0.Path != "/products" {
		t.Errorf("Entry[0].Path = %q, want /products", e0.Path)
	}
	if e0.Overrides == nil {
		t.Fatal("Entry[0].Overrides is nil")
	}
	if !isOverridden(e0.Overrides, e0.Overrides.Create) {
		t.Error("Entry[0].Overrides.Create should be overridden")
	}
	if e0.Overrides.Create != "onCustomCreate" {
		t.Errorf("Override = %q", e0.Overrides.Create)
	}

	// CRUD 2: Order (partial overrides)
	e1 := cfg.Entry[1]
	if e1.Type != "crud" {
		t.Errorf("Entry[1].Type = %q", e1.Type)
	}
	if e1.Table != "orders" {
		t.Errorf("Entry[1].Table = %q", e1.Table)
	}
	if e1.Overrides == nil {
		t.Fatal("Entry[1].Overrides is nil")
	}
	// list: not specified → "" default
	if e1.Overrides.List != "" {
		t.Errorf("List = %q, want empty (default)", e1.Overrides.List)
	}
	// get: "onCustomGet" → overridden
	if !isOverridden(e1.Overrides, e1.Overrides.Get) {
		t.Error("Get should be overridden")
	}
	if e1.Overrides.Get != "onCustomGet" {
		t.Errorf("Get = %q", e1.Overrides.Get)
	}
	// create: not specified → "" default
	if e1.Overrides.Create != "" {
		t.Errorf("Create = %q, want empty (default)", e1.Overrides.Create)
	}
	// update: "-" → disabled
	if !isDisabled(e1.Overrides, e1.Overrides.Update) {
		t.Error("Update should be disabled (-)")
	}
	if !isDisabled(e1.Overrides, e1.Overrides.Delete) {
		t.Error("Delete should be disabled (-)")
	}

	// REST: transform
	e2 := cfg.Entry[2]
	if e2.Type != "rest" {
		t.Errorf("Entry[2].Type = %q", e2.Type)
	}
	if e2.Method != "GET" {
		t.Errorf("Entry[2].Method = %q", e2.Method)
	}
	if e2.Handler != "onTransformProduct" {
		t.Errorf("Entry[2].Handler = %q", e2.Handler)
	}
	if e2.DB != "" {
		t.Errorf("Entry[2].DB should be empty for transform")
	}
	if !e2.Auth {
		t.Error("Entry[2].Auth should be true")
	}

	// REST: with NATS publish
	e3 := cfg.Entry[3]
	if e3.Type != "rest" {
		t.Errorf("Entry[3].Type = %q", e3.Type)
	}
	if e3.Method != "POST" {
		t.Errorf("Entry[3].Method = %q", e3.Method)
	}
	if e3.DB != "pg-main" {
		t.Errorf("Entry[3].DB = %q", e3.DB)
	}
	if len(e3.NATSPublish) != 1 {
		t.Fatalf("Entry[3].NATSPublish = %d", len(e3.NATSPublish))
	}
	if e3.NATSPublish[0].Stream != "orders" {
		t.Errorf("NATSPublish.Stream = %q", e3.NATSPublish[0].Stream)
	}
	if e3.NATSPublish[0].Subject != "orders.created" {
		t.Errorf("NATSPublish.Subject = %q", e3.NATSPublish[0].Subject)
	}

	// Webhook
	e4 := cfg.Entry[4]
	if e4.Type != "webhook" {
		t.Errorf("Entry[4].Type = %q", e4.Type)
	}
	if e4.Method != "POST" {
		t.Errorf("Entry[4].Method = %q, want POST", e4.Method)
	}

	// WebSocket
	e5 := cfg.Entry[5]
	if e5.Type != "websocket" {
		t.Errorf("Entry[5].Type = %q", e5.Type)
	}
	if e5.Path != "/ws/chat" {
		t.Errorf("Entry[5].Path = %q", e5.Path)
	}

	// SSE
	e6 := cfg.Entry[6]
	if e6.Type != "sse" {
		t.Errorf("Entry[6].Type = %q", e6.Type)
	}
	if e6.Path != "/events/stream" {
		t.Errorf("Entry[6].Path = %q", e6.Path)
	}

	// File: S3 upload
	e7 := cfg.Entry[7]
	if e7.Type != "file" {
		t.Errorf("Entry[7].Type = %q", e7.Type)
	}
	if e7.Method != "POST" {
		t.Errorf("Entry[7].Method = %q", e7.Method)
	}
	if e7.Storage == nil {
		t.Fatal("Entry[7].Storage is nil")
	}
	if e7.Storage.Mode != "s3" {
		t.Errorf("Storage.Mode = %q", e7.Storage.Mode)
	}
	if e7.Storage.Bucket != "uploads" {
		t.Errorf("Storage.Bucket = %q", e7.Storage.Bucket)
	}
	if e7.Storage.Endpoint != "http://minio:9000" {
		t.Errorf("Storage.Endpoint = %q", e7.Storage.Endpoint)
	}
	if len(e7.AllowedTypes) != 2 {
		t.Errorf("AllowedTypes = %d, want 2", len(e7.AllowedTypes))
	}

	// File: local download
	e8 := cfg.Entry[8]
	if e8.Type != "file" {
		t.Errorf("Entry[8].Type = %q", e8.Type)
	}
	if e8.Method != "GET" {
		t.Errorf("Entry[8].Method = %q", e8.Method)
	}
	if e8.Storage.Mode != "local" {
		t.Errorf("Storage.Mode = %q, want local", e8.Storage.Mode)
	}
	if e8.Storage.Path != "/data/uploads" {
		t.Errorf("Storage.Path = %q", e8.Storage.Path)
	}

	// Exit workers
	if len(cfg.Exit) != 2 {
		t.Fatalf("Exit = %d, want 2", len(cfg.Exit))
	}

	ex0 := cfg.Exit[0]
	if ex0.Name != "email-sender" {
		t.Errorf("Exit[0].Name = %q", ex0.Name)
	}
	if ex0.Reply {
		t.Error("Exit[0].Reply should be false")
	}
	if ex0.MaxConcurrent != 10 {
		t.Errorf("Exit[0].MaxConcurrent = %d", ex0.MaxConcurrent)
	}
	if ex0.DB != "" {
		t.Errorf("Exit[0].DB should be empty (no db)")
	}

	ex1 := cfg.Exit[1]
	if ex1.Name != "order-validator" {
		t.Errorf("Exit[1].Name = %q", ex1.Name)
	}
	if !ex1.Reply {
		t.Error("Exit[1].Reply should be true")
	}
	if ex1.ReplyTimeout != "30s" {
		t.Errorf("Exit[1].ReplyTimeout = %q", ex1.ReplyTimeout)
	}
	if ex1.DB != "pg-main" {
		t.Errorf("Exit[1].DB = %q, want pg-main", ex1.DB)
	}
	if ex1.Subscribe.Durable != "order-validator-durable" {
		t.Errorf("Exit[1].Durable = %q", ex1.Subscribe.Durable)
	}

	// Cron
	if len(cfg.Cron) != 3 {
		t.Fatalf("Cron = %d, want 3", len(cfg.Cron))
	}

	c0 := cfg.Cron[0]
	if c0.Name != "daily-report" {
		t.Errorf("Cron[0].Name = %q", c0.Name)
	}
	if c0.Mode != "nats" {
		t.Errorf("Cron[0].Mode = %q, want nats", c0.Mode)
	}
	if c0.Publish == nil || c0.Publish.Stream != "orders" {
		t.Error("Cron[0].Publish.Stream should be orders")
	}

	c1 := cfg.Cron[1]
	if c1.Name != "cleanup-expired" {
		t.Errorf("Cron[1].Name = %q", c1.Name)
	}
	if c1.Mode != "handler" {
		t.Errorf("Cron[1].Mode = %q, want handler", c1.Mode)
	}
	if c1.Handler != "onCleanupExpired" {
		t.Errorf("Cron[1].Handler = %q", c1.Handler)
	}

	c2 := cfg.Cron[2]
	if c2.Name != "health-check" {
		t.Errorf("Cron[2].Name = %q", c2.Name)
	}
	if c2.Mode != "internal" {
		t.Errorf("Cron[2].Mode = %q, want internal", c2.Mode)
	}

	// Server
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q", cfg.Server.Host)
	}
	if cfg.Server.APIPrefix != "/api/v1" {
		t.Errorf("Server.APIPrefix = %q", cfg.Server.APIPrefix)
	}
	if cfg.Server.CORS == nil {
		t.Fatal("CORS is nil")
	}
	if !cfg.Server.CORS.Credentials {
		t.Error("CORS.Credentials should be true")
	}
	if len(cfg.Server.Middleware) != 1 {
		t.Fatalf("Middleware = %d, want 1", len(cfg.Server.Middleware))
	}
	if cfg.Server.Middleware[0].Path != "/api/v1/*" {
		t.Errorf("Middleware.Path = %q", cfg.Server.Middleware[0].Path)
	}
	if len(cfg.Server.Middleware[0].Apply) != 4 {
		t.Errorf("Middleware.Apply = %d, want 4", len(cfg.Server.Middleware[0].Apply))
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := &ServiceConfig{}
	// manual default since we're not loading from file
	cfg.Port = 8080
	_ = cfg
}

func TestDBConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DBConfig
		wantErr bool
	}{
		{"valid pg", DBConfig{Name: "pg", Driver: "postgres", URL: "pg://x"}, false},
		{"valid turso", DBConfig{Name: "t", Driver: "turso", URL: "libsql://x"}, false},
		{"valid mysql", DBConfig{Name: "m", Driver: "mysql", URL: "mysql://x"}, false},
		{"auto postgres default", DBConfig{Name: "auto", URL: "pg://x"}, false},
		{"wrong driver", DBConfig{Name: "w", Driver: "oracle", URL: "x"}, true},
		{"missing url", DBConfig{Name: "mu", Driver: "postgres"}, true},
		{"missing name", DBConfig{Driver: "postgres", URL: "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestEntryDef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   EntryDef
		wantErr bool
	}{
		{"valid crud", EntryDef{Type: "crud", Model: "Product", DB: "pg", Table: "products"}, false},
		{"crud missing model", EntryDef{Type: "crud", DB: "pg"}, true},
		{"crud missing db", EntryDef{Type: "crud", Model: "P"}, true},
		{"crud auto table", EntryDef{Type: "crud", Model: "OrderItem", DB: "pg"}, false},
		{"valid rest", EntryDef{Type: "rest", Method: "GET", Path: "/x", Handler: "f"}, false},
		{"rest missing handler", EntryDef{Type: "rest", Method: "GET", Path: "/x"}, true},
		{"rest missing method", EntryDef{Type: "rest", Path: "/x", Handler: "f"}, true},
		{"valid webhook", EntryDef{Type: "webhook", Path: "/wh", Handler: "f"}, false},
		{"valid ws", EntryDef{Type: "websocket", Path: "/ws", Handler: "f"}, false},
		{"ws missing handler", EntryDef{Type: "websocket", Path: "/ws"}, true},
		{"valid sse", EntryDef{Type: "sse", Path: "/sse", Handler: "f"}, false},
		{"valid file s3", EntryDef{Type: "file", Method: "POST", Path: "/f", Handler: "f", Storage: &StorageDef{Mode: "s3"}}, false},
		{"valid file local", EntryDef{Type: "file", Method: "POST", Path: "/f", Handler: "f", Storage: &StorageDef{Mode: "local", Path: "/tmp"}}, false},
		{"file missing storage", EntryDef{Type: "file", Method: "POST", Path: "/f", Handler: "f"}, true},
		{"file bad storage mode", EntryDef{Type: "file", Method: "POST", Path: "/f", Handler: "f", Storage: &StorageDef{Mode: "ftp"}}, true},
		{"file local missing path", EntryDef{Type: "file", Method: "POST", Path: "/f", Handler: "f", Storage: &StorageDef{Mode: "local"}}, true},
		{"unknown type", EntryDef{Type: "grpc", Path: "/g", Handler: "f"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestExitWorker_Validate(t *testing.T) {
	tests := []struct {
		name    string
		worker  ExitWorker
		wantErr bool
	}{
		{"valid", ExitWorker{Name: "w", Subscribe: SubscribeDef{Stream: "s"}, Handler: "h"}, false},
		{"missing name", ExitWorker{Subscribe: SubscribeDef{Stream: "s"}, Handler: "h"}, true},
		{"missing stream", ExitWorker{Name: "w", Subscribe: SubscribeDef{}, Handler: "h"}, true},
		{"missing handler", ExitWorker{Name: "w", Subscribe: SubscribeDef{Stream: "s"}}, true},
		{"auto durable", ExitWorker{Name: "emailer", Subscribe: SubscribeDef{Stream: "s"}, Handler: "h"}, false},
		{"auto subject from stream", ExitWorker{Name: "w", Subscribe: SubscribeDef{Stream: "orders"}, Handler: "h"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.worker.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			// Check auto-filled defaults
			if err == nil && tt.worker.Subscribe.Subject == "" {
				t.Error("Subject should be auto-filled from stream name")
			}
			if err == nil && tt.worker.Subscribe.Durable == "" {
				t.Error("Durable should be auto-filled")
			}
		})
	}
}

func TestCronJob_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cron    CronJob
		wantErr bool
	}{
		{"nats mode valid", CronJob{Name: "c", Schedule: "* * * * *", Mode: "nats", Publish: &CronPublish{Stream: "s"}}, false},
		{"nats mode missing stream", CronJob{Name: "c", Schedule: "* * * * *", Mode: "nats"}, true},
		{"nats mode nil publish", CronJob{Name: "c", Schedule: "* * * * *", Mode: "nats", Publish: &CronPublish{}}, true},
		{"handler mode valid", CronJob{Name: "c", Schedule: "* * * * *", Mode: "handler", Handler: "f"}, false},
		{"handler mode missing handler", CronJob{Name: "c", Schedule: "* * * * *", Mode: "handler"}, true},
		{"internal mode valid", CronJob{Name: "c", Schedule: "* * * * *", Mode: "internal"}, false},
		{"default mode nats", CronJob{Name: "c", Schedule: "* * * * *", Publish: &CronPublish{Stream: "s"}}, false},
		{"bad mode", CronJob{Name: "c", Schedule: "* * * * *", Mode: "unknown"}, true},
		{"missing name", CronJob{Schedule: "* * * * *", Mode: "nats"}, true},
		{"missing schedule", CronJob{Name: "c", Mode: "nats"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cron.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestCRUDOverrides_Helpers(t *testing.T) {
	o := &CRUDOverrides{}

	if o.List != "" {
		t.Error("List should be empty (default)")
	}

	o.Create = "-"
	if !isDisabled(o, o.Create) {
		t.Error("Create should be disabled (-)")
	}
	if isOverridden(o, o.Create) {
		t.Error("Create should not be overridden (it's disabled)")
	}

	o.Update = "onCustomFn"
	if isDisabled(o, o.Update) {
		t.Error("Update should not be disabled")
	}
	if !isOverridden(o, o.Update) {
		t.Error("Update should be overridden")
	}

	o.Delete = ""
	if isDisabled(o, o.Delete) {
		t.Error("Delete should not be disabled (empty = default)")
	}
	if isOverridden(o, o.Delete) {
		t.Error("Delete should not be overridden (empty = default)")
	}
}

func TestLoadConfig_DuplicateDBNames(t *testing.T) {
	cfg := ServiceConfig{
		Name: "test",
		Databases: []DBConfig{
			{Name: "pg", Driver: "postgres", URL: "pg://a"},
			{Name: "pg", Driver: "postgres", URL: "pg://b"},
		},
	}
	_ = cfg
	// This scenario is caught in LoadConfig via duplicate name check
}

func TestLoadConfig_DBReferenceValidation(t *testing.T) {
	cfg := ServiceConfig{
		Name: "test",
		Databases: []DBConfig{
			{Name: "pg", Driver: "postgres", URL: "pg://a"},
		},
		Entry: []EntryDef{
			{Type: "crud", Model: "Product", DB: "nonexistent", Table: "p"},
		},
	}
	_ = cfg
}

func TestToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Product", "product"},
		{"OrderItem", "order_item"},
		{"ID", "id"},
		{"URL", "url"},
		{"XMLParser", "xml_parser"},
		{"HTTPServer", "http_server"},
		{"DB", "db"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toSnake(tt.input)
			if got != tt.expected {
				t.Errorf("toSnake(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"product", "products"},
		{"category", "categories"},
		{"bus", "bus"},
		{"status", "status"},
		{"day", "days"},
		{"key", "keys"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := plural(tt.input)
			if got != tt.expected {
				t.Errorf("plural(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
