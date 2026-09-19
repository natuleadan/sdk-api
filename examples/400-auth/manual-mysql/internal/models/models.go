package models

import (
	"time"

	"github.com/natuleadan/sdk-api/db"
)

// ============================================================================
// 1. Users
// ============================================================================

type User struct {
	ID           string `db:"id,primary,default=gen_random_uuid()"`
	Username     string `db:"username,unique,required"`
	PasswordHash string `db:"password_hash,required"`
	Role         string `db:"role,required,default=viewer"`
	CreatedAt    string `db:"created_at,default=now()"`
}

// ============================================================================
// 2. API Keys
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
// 3. Products
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
// 4. Tenant Products
// ============================================================================

type TenantProduct struct {
	ID       string  `db:"id,primary,default=gen_random_uuid()" json:"id"`
	Name     string  `db:"name,required" json:"name"`
	Price    float64 `db:"price,type=DECIMAL(10,2),default=0" json:"price"`
	TenantID string  `db:"tenant_id,required" json:"tenant_id"`
}

// ============================================================================
// 5. Audit Log
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

// ============================================================================
// 6. Failed Logins
// ============================================================================

type FailedLogin struct {
	Username    string  `db:"username,primary"`
	Attempts    int     `db:"attempts,default=1"`
	LastAttempt string  `db:"last_attempt"`
	LockedUntil *string `db:"locked_until"`
}

// ============================================================================
// 7. Revoked Tokens
// ============================================================================

type RevokedToken struct {
	TokenHash string `db:"token_hash,primary"`
	RevokedAt string `db:"revoked_at,default=now()"`
}

// ============================================================================
// 8. MFA Secrets
// ============================================================================

type MFASecret struct {
	UserID    string `db:"user_id,primary"`
	Secret    string `db:"secret,required"`
	Enabled   bool   `db:"enabled,default=false"`
	CreatedAt string `db:"created_at,default=now()"`
}

// ============================================================================
// 9. Email Verifications
// ============================================================================

type EmailVerification struct {
	UserID    string    `db:"user_id,primary"`
	Token     string    `db:"token,required"`
	Verified  bool      `db:"verified,default=false"`
	CreatedAt time.Time `db:"created_at,default=now()"`
	ExpiresAt time.Time `db:"expires_at,required"`
}

// ============================================================================
// 10. Password Resets
// ============================================================================

type PasswordReset struct {
	UserID    string    `db:"user_id,primary"`
	Token     string    `db:"token,required"`
	CreatedAt time.Time `db:"created_at,default=now()"`
	ExpiresAt time.Time `db:"expires_at,required"`
}

// ============================================================================
// 11. Auth Codes
// ============================================================================

type AuthCode struct {
	ID             string    `db:"id,primary,default=gen_random_uuid()"`
	UserID         string    `db:"user_id"`
	Code           string    `db:"code,required"`
	Purpose        string    `db:"purpose,required,default=access"`
	DeliveredTo    string    `db:"delivered_to"`
	DeliveryMethod string    `db:"delivery_method,required,default=console"`
	ExpiresAt      time.Time `db:"expires_at,required"`
	Attempts       int       `db:"attempts,default=0"`
	Used           bool      `db:"used,default=false"`
	CreatedAt      time.Time `db:"created_at,default=now()"`
}

// ============================================================================
// 12. Linked Accounts
// ============================================================================

type LinkedAccount struct {
	ID         string `db:"id,primary,default=gen_random_uuid()"`
	UserID     string `db:"user_id,required,fk=users.id"`
	Provider   string `db:"provider,required"`
	ProviderID string `db:"provider_id,required"`
	Email      string `db:"email"`
}

func (LinkedAccount) Constraints() []db.Constraint {
	return []db.Constraint{
		{Type: "UNIQUE", Columns: []string{"provider", "provider_id"}},
	}
}

// ============================================================================
// 13. WebAuthn Users
// ============================================================================

type WebAuthnUser struct {
	ID        string `db:"id,primary,default=gen_random_uuid()"`
	UserID    string `db:"user_id,required,fk=users.id"`
	Handle    []byte `db:"handle,required,unique"`
	CreatedAt string `db:"created_at,default=now()"`
}

// ============================================================================
// 14. WebAuthn Credentials
// ============================================================================

type WebAuthnCredential struct {
	ID                string `db:"id,primary,default=gen_random_uuid()"`
	UserID            string `db:"user_id,required,fk=users.id"`
	KID               []byte `db:"kid,required"`
	PublicKey         []byte `db:"public_key,required"`
	AttestationType   string `db:"attestation_type,required"`
	AttestationFormat string `db:"attestation_format,required"`
	Transport         string `db:"transport,default=''"`
	SignCount         int64  `db:"sign_count,default=0"`
	AAGUID            []byte `db:"aaguid"`
	CloneWarning      bool   `db:"clone_warning,default=false"`
	Attachment        string `db:"attachment,default=''"`
	Flags             []byte `db:"flags"`
	Present           bool   `db:"present,default=false"`
	Verified          bool   `db:"verified,default=false"`
	BackupEligible    bool   `db:"backup_eligible,default=false"`
	BackupState       bool   `db:"backup_state,default=false"`
	CreatedAt         string `db:"created_at,default=now()"`
}

func (WebAuthnCredential) Constraints() []db.Constraint {
	return []db.Constraint{
		{Type: "UNIQUE", Columns: []string{"kid"}},
	}
}

// ============================================================================
// 15. WebAuthn Sessions
// ============================================================================

type WebAuthnSession struct {
	ID           string `db:"id,primary,default=gen_random_uuid()"`
	UserID       string `db:"user_id"`
	CeremonyType string `db:"ceremony_type,required"`
	SessionData  string `db:"session_data,type=JSONB,required"`
	ExpiresAt    string `db:"expires_at,required"`
	CreatedAt    string `db:"created_at,default=now()"`
}

// ============================================================================
// 16. OAuth Clients
// ============================================================================

type OAuthClient struct {
	ID            string   `db:"id,primary"`
	HashedSecret  []byte   `db:"hashed_secret,required"`
	RedirectURIs  []string `db:"redirect_uris,type=JSON"`
	GrantTypes    []string `db:"grant_types,type=JSON"`
	ResponseTypes []string `db:"response_types,type=JSON"`
	Scopes        string   `db:"scopes,default=''"`
	Audience      []string `db:"audience,type=JSON"`
	IsPublic      bool     `db:"is_public,default=false"`
	CreatedAt     string   `db:"created_at,default=now()"`
}

// ============================================================================
// 17. OAuth Sessions
// ============================================================================

type OAuthSession struct {
	ID                string `db:"id,primary,default=gen_random_uuid()"`
	Signature         string `db:"signature,required"`
	Type              string `db:"type,required"`
	RequestID         string `db:"request_id"`
	ClientID          string `db:"client_id,required"`
	RequestedScopes   string `db:"requested_scopes,type=JSON"`
	GrantedScopes     string `db:"granted_scopes,type=JSON"`
	RequestedAudience string `db:"requested_audience,type=JSON"`
	GrantedAudience   string `db:"granted_audience,type=JSON"`
	SessionData       string `db:"session_data,type=JSON"`
	Form              string `db:"form,type=JSON"`
	Lang              string `db:"lang,default=''"`
	Active            bool   `db:"active,default=true"`
	ExpiresAt         string `db:"expires_at"`
	CreatedAt         string `db:"created_at,default=now()"`
}

func (OAuthSession) Constraints() []db.Constraint {
	return []db.Constraint{
		{Type: "UNIQUE", Columns: []string{"signature", "type"}},
	}
}

// ============================================================================
// 18. OAuth JTI Blacklist
// ============================================================================

type OAuthJTIS struct {
	JTI       string `db:"jti,primary"`
	ExpiresAt string `db:"expires_at,required"`
}

// ============================================================================
// Role assignments — dynamic, arbitrary role names, with lifecycle
// ============================================================================

// RoleAssignment records that a user holds a role. Role names are free-form and
// unlimited; status is pending|active|revoked; expires_at bounds its validity.
type RoleAssignment struct {
	ID         string     `db:"id,primary,default=gen_random_uuid()"`
	UserID     string     `db:"user_id,required"`
	Role       string     `db:"role,required"`
	Status     string     `db:"status,required,default=active"`
	GrantedAt  string     `db:"granted_at,default=now()"`
	ExpiresAt  *time.Time `db:"expires_at"`
	AcceptedAt *time.Time `db:"accepted_at"`
}

// Constraints makes (user_id, role) unique so upserts are idempotent.
func (RoleAssignment) Constraints() []db.Constraint {
	return []db.Constraint{{Type: "UNIQUE", Columns: []string{"user_id", "role"}}}
}

// RolePermission maps a role to a permission ("resource:action").
type RolePermission struct {
	ID         string `db:"id,primary,default=gen_random_uuid()"`
	Role       string `db:"role,required"`
	Permission string `db:"permission,required"`
}

// Constraints makes (role, permission) unique so seeding is idempotent.
func (RolePermission) Constraints() []db.Constraint {
	return []db.Constraint{{Type: "UNIQUE", Columns: []string{"role", "permission"}}}
}
