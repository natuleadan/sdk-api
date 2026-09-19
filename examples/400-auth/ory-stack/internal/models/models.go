package models

import (
	"time"

	"github.com/natuleadan/sdk-api/db"
)

// ============================================================================
// 1. API Keys
// ============================================================================

type APIKey struct {
	ID        string `db:"id,primary,default=gen_random_uuid()"`
	KeyHash   string `db:"key_hash,unique,required"`
	Label     string `db:"label"`
	Role      string `db:"role,required"`
	Enabled   bool   `db:"enabled,default=true"`
	CreatedAt string `db:"created_at,default=now()"`
}

// ============================================================================
// 2. Products
// ============================================================================

type Product struct {
	ID          string `db:"id,primary,default=gen_random_uuid()"`
	Name        string `db:"name,required"`
	Description string `db:"description,default=''"`
	Price       string `db:"price,type=DECIMAL(10,2),default=0"`
	Visibility  string `db:"visibility,default=public"`
	CreatedBy   string `db:"created_by"`
	DeletedAt   string `db:"deleted_at"`
	UpdatedAt   string `db:"updated_at,default=now()"`
}

// ============================================================================
// 3. Tenant Products
// ============================================================================

type TenantProduct struct {
	ID       string  `db:"id,primary,default=gen_random_uuid()" json:"id"`
	Name     string  `db:"name,required" json:"name"`
	Price    float64 `db:"price,type=DECIMAL(10,2),default=0" json:"price"`
	TenantID string  `db:"tenant_id,required" json:"tenant_id"`
}

// ============================================================================
// 4. Audit Log
// ============================================================================

type AuditLog struct {
	ID        string    `db:"id,primary,default=gen_random_uuid()"`
	ProductID string    `db:"product_id"`
	Action    string    `db:"action,required"`
	ChangedBy string    `db:"changed_by,required"`
	OldValue  string    `db:"old_value,type=JSONB"`
	NewValue  string    `db:"new_value,type=JSONB"`
	CreatedAt time.Time `db:"created_at,default=now()"`
}

// Ensure the db package stays referenced (constraint helpers live there).
var _ = db.Constraint{}
