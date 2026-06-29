package runtime

import (
	"testing"

	"github.com/natuleadan/sdk-api/db"
)

func TestNewMySQLCRUDProvider_ImplementsInterface(t *testing.T) {
	// Verify the provider implements CRUDProvider interface at compile time
	var _ CRUDProvider = (*mysqlCRUD[testModel])(nil)
}

func TestNewTursoCRUDProvider_ImplementsInterface(t *testing.T) {
	var _ CRUDProvider = (*tursoCRUD[testModel])(nil)
}

func TestMySQLCRUDProvider_SetHooks(t *testing.T) {
	// Create with nil table just for structure test
	provider := &mysqlCRUD[testModel]{}
	hooks := &trackingHooks{}
	provider.SetHooks(hooks)
	if provider.hooks != hooks {
		t.Error("SetHooks should set hooks")
	}
	// Wrong type should not change
	provider.SetHooks("not hooks")
	if provider.hooks != hooks {
		t.Error("SetHooks with wrong type should not change hooks")
	}
}

func TestTursoCRUDProvider_SetHooks(t *testing.T) {
	provider := &tursoCRUD[testModel]{}
	hooks := &trackingHooks{}
	provider.SetHooks(hooks)
	if provider.hooks != hooks {
		t.Error("SetHooks should set hooks")
	}
	provider.SetHooks("bad type")
	if provider.hooks != hooks {
		t.Error("SetHooks should ignore wrong type")
	}
}

func TestNewMySQLCRUDProvider_NilHooks(t *testing.T) {
	// Should default to DefaultHooks when nil is passed
	var table db.MySQLTable[testModel]
	provider := NewMySQLCRUDProvider(&table, nil)
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
	// DefaultHooks should have been assigned
	mysqlP := provider.(*mysqlCRUD[testModel])
	if mysqlP.hooks == nil {
		t.Error("hooks should default to DefaultHooks when nil passed")
	}
}

func TestNewTursoCRUDProvider_NilHooks(t *testing.T) {
	var table db.TursoTable[testModel]
	provider := NewTursoCRUDProvider(&table, nil)
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
	tursoP := provider.(*tursoCRUD[testModel])
	if tursoP.hooks == nil {
		t.Error("hooks should default to DefaultHooks when nil passed")
	}
}

func TestNewCRUDProvider_NilHooks(t *testing.T) {
	var table db.Table[testModel]
	provider := NewCRUDProvider(&table, nil)
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
	pgP := provider.(*tableCRUD[testModel])
	if pgP.hooks == nil {
		t.Error("hooks should default to DefaultHooks when nil passed")
	}
}

func TestMySQLCRUDProvider_AllMethodsExist(t *testing.T) {
	var p CRUDProvider = &mysqlCRUD[testModel]{}
	_ = p.List
	_ = p.Get
	_ = p.Create
	_ = p.Update
	_ = p.Delete
}

func TestTursoCRUDProvider_AllMethodsExist(t *testing.T) {
	var p CRUDProvider = &tursoCRUD[testModel]{}
	_ = p.List
	_ = p.Get
	_ = p.Create
	_ = p.Update
	_ = p.Delete
}
