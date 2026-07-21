package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"auth-roles/internal/svc"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func newWebAuthn() *webauthn.WebAuthn {
	w, _ := webauthn.New(&webauthn.Config{
		RPDisplayName: "400-auth",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost:23400"},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationPreferred,
		},
	})
	return w
}

type webAuthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                       { return u.id }
func (u *webAuthnUser) WebAuthnName() string                     { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string              { return u.displayName }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func loadWebAuthnUser(c *runtime.RestCtx, userID string) (*webAuthnUser, error) {
	pool := c.PoolPG("primary")
	ctx := c.Context()

	var handle []byte
	err := pool.QueryRow(ctx, `SELECT handle FROM webauthn_users WHERE user_id = $1`, userID).Scan(&handle)
	if err != nil {
		handle = randomBytes(32)
		_, err = pool.Exec(ctx,
			`INSERT INTO webauthn_users (user_id, handle) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			userID, handle)
		if err != nil {
			return nil, fmt.Errorf("create webauthn user: %w", err)
		}
	}

	var username string
	pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)

	creds, err := loadCredentials(c, userID)
	if err != nil {
		return nil, err
	}

	return &webAuthnUser{
		id:          handle,
		name:        userID,
		displayName: username,
		credentials: creds,
	}, nil
}

func loadWebAuthnUserByHandle(c *runtime.RestCtx, handle []byte) (*webAuthnUser, error) {
	pool := c.PoolPG("primary")
	ctx := c.Context()

	var userID string
	err := pool.QueryRow(ctx, `SELECT user_id FROM webauthn_users WHERE handle = $1`, handle).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("user not found by handle")
	}
	return loadWebAuthnUser(c, userID)
}

func loadCredentials(c *runtime.RestCtx, userID string) ([]webauthn.Credential, error) {
	pool := c.PoolPG("primary")
	rows, err := pool.Query(c.Context(),
		`SELECT kid, public_key, attestation_type, attestation_format, transport, sign_count,
		        aaguid, clone_warning, attachment, flags, present, verified, backup_eligible, backup_state
		 FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var c webauthn.Credential
		var transport, attachment string
		var flags []byte
		err := rows.Scan(&c.ID, &c.PublicKey, &c.AttestationType, &c.AttestationFormat,
			&transport, &c.Authenticator.SignCount, &c.Authenticator.AAGUID,
			&c.Authenticator.CloneWarning, &attachment, &flags,
			&c.Flags.UserPresent, &c.Flags.UserVerified,
			&c.Flags.BackupEligible, &c.Flags.BackupState)
		if err != nil {
			return nil, err
		}
		if transport != "" {
			c.Transport = []protocol.AuthenticatorTransport{protocol.AuthenticatorTransport(transport)}
		}
		c.Authenticator.Attachment = protocol.AuthenticatorAttachment(attachment)
		if len(flags) > 0 {
			c.Flags = webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(flags[0]))
		}
		creds = append(creds, c)
	}
	return creds, nil
}

func saveCredential(c *runtime.RestCtx, userID string, cred *webauthn.Credential) error {
	pool := c.PoolPG("primary")
	transport := ""
	if len(cred.Transport) > 0 {
		transport = string(cred.Transport[0])
	}
	flags := []byte{byte(cred.Flags.ProtocolValue())}
	_, err := pool.Exec(c.Context(),
		`INSERT INTO webauthn_credentials
		 (user_id, kid, public_key, attestation_type, attestation_format, transport,
		  sign_count, aaguid, clone_warning, attachment, flags,
		  present, verified, backup_eligible, backup_state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT (kid) DO UPDATE SET
		 sign_count = EXCLUDED.sign_count, clone_warning = EXCLUDED.clone_warning,
		 backup_state = EXCLUDED.backup_state, flags = EXCLUDED.flags`,
		userID, cred.ID, cred.PublicKey, cred.AttestationType, cred.AttestationFormat,
		transport, cred.Authenticator.SignCount, cred.Authenticator.AAGUID,
		cred.Authenticator.CloneWarning, string(cred.Authenticator.Attachment), flags,
		cred.Flags.UserPresent, cred.Flags.UserVerified,
		cred.Flags.BackupEligible, cred.Flags.BackupState)
	return err
}

func deleteCredential(c *runtime.RestCtx, userID, credentialID string) error {
	pool := c.PoolPG("primary")
	_, err := pool.Exec(c.Context(),
		`DELETE FROM webauthn_credentials WHERE user_id = $1 AND encode(kid, 'hex') = $2`,
		userID, credentialID)
	return err
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() & 0xff)
	}
	return b
}

func storeSession(c *runtime.RestCtx, userID, ceremony string, session *webauthn.SessionData) (string, error) {
	pool := c.PoolPG("primary")
	data, _ := json.Marshal(session)
	var id string
	err := pool.QueryRow(c.Context(),
		`INSERT INTO webauthn_sessions (user_id, ceremony_type, session_data, expires_at)
		 VALUES ($1, $2, $3, now() + interval '5 minutes') RETURNING id`,
		userID, ceremony, data).Scan(&id)
	return id, err
}

func loadSession(c *runtime.RestCtx, sessionID string) (string, string, *webauthn.SessionData, error) {
	pool := c.PoolPG("primary")
	var userID, ceremony string
	var data []byte
	err := pool.QueryRow(c.Context(),
		`SELECT user_id, ceremony_type, session_data FROM webauthn_sessions WHERE id = $1 AND expires_at > now()`,
		sessionID).Scan(&userID, &ceremony, &data)
	if err != nil {
		return "", "", nil, fmt.Errorf("session not found or expired")
	}
	var session webauthn.SessionData
	json.Unmarshal(data, &session)
	return userID, ceremony, &session, nil
}

func deleteSession(c *runtime.RestCtx, sessionID string) {
	pool := c.PoolPG("primary")
	pool.Exec(c.Context(), `DELETE FROM webauthn_sessions WHERE id = $1`, sessionID)
}

func handleWebAuthnRegisterBegin(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}

		var body struct {
			Type string `json:"type"`
		}
		c.Bind(&body)

		web, user, err := prepareWebAuthnUser(c, a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		opts := []webauthn.RegistrationOption{}
		if body.Type == "passkey" {
			opts = append(opts, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
		} else {
			opts = append(opts, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementDiscouraged))
		}

		creation, session, err := web.BeginRegistration(user, opts...)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": fmt.Sprintf("begin registration: %v", err)})
		}

		sessionID, err := storeSession(c, a.UserID, "register", session)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session storage failed"})
		}

		return c.JSON(runtime.Map{
			"session_id": sessionID,
			"creation":   creation,
		})
	}
}

func handleWebAuthnRegisterFinish(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}

		var body struct {
			SessionID string          `json:"session_id"`
			Response  json.RawMessage `json:"response"`
		}
		if err := c.Bind(&body); err != nil || body.SessionID == "" || body.Response == nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "session_id and response required"})
		}

		userID, ceremony, session, err := loadSession(c, body.SessionID)
		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": err.Error()})
		}
		defer deleteSession(c, body.SessionID)

		if ceremony != "register" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid session ceremony"})
		}
		if userID != a.UserID {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "session user mismatch"})
		}

		web, user, err := prepareWebAuthnUser(c, a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		parsed, err := protocol.ParseCredentialCreationResponseBytes([]byte(body.Response))
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": fmt.Sprintf("parse response: %v", err)})
		}

		cred, err := web.CreateCredential(user, *session, parsed)
		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": fmt.Sprintf("create credential: %v", err)})
		}

		if err := saveCredential(c, a.UserID, cred); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "credential storage failed"})
		}

		return c.JSON(runtime.Map{"status": "registered", "credential_id": base64.RawURLEncoding.EncodeToString(cred.ID)})
	}
}

func handleWebAuthnLoginBegin(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		web, err := webauthn.New(&webauthn.Config{
			RPDisplayName: "400-auth",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:23400"},
		})
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		assertion, session, err := web.BeginDiscoverableLogin()
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": fmt.Sprintf("begin login: %v", err)})
		}

		sessionID, err := storeSession(c, "", "login_passkey", session)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session storage failed"})
		}

		return c.JSON(runtime.Map{
			"session_id": sessionID,
			"assertion":  assertion,
		})
	}
}

func handleWebAuthnLoginFinish(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			SessionID string          `json:"session_id"`
			Response  json.RawMessage `json:"response"`
		}
		if err := c.Bind(&body); err != nil || body.SessionID == "" || body.Response == nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "session_id and response required"})
		}

		_, ceremony, session, err := loadSession(c, body.SessionID)
		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": err.Error()})
		}
		defer deleteSession(c, body.SessionID)

		if ceremony != "login_passkey" && ceremony != "login_manual" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid session ceremony"})
		}

		web, err := webauthn.New(&webauthn.Config{
			RPDisplayName: "400-auth",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:23400"},
		})
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		parsed, err := protocol.ParseCredentialRequestResponseBytes([]byte(body.Response))
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": fmt.Sprintf("parse response: %v", err)})
		}

		if ceremony == "login_passkey" {
			wu, cred, err := web.ValidatePasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
				if len(userHandle) == 0 {
					return nil, fmt.Errorf("no user handle in passkey response")
				}
				return loadWebAuthnUserByHandle(c, userHandle)
			}, *session, parsed)
			if err != nil {
				return c.Status(401).JSON(runtime.Map{"code": 401, "message": fmt.Sprintf("passkey login: %v", err)})
			}
			return loginWithCredential(c, svcCtx, wu.(*webAuthnUser), cred)
		}

		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "unexpected ceremony state"})
	}
}

func handleWebAuthnManualLoginBegin(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
		}
		if err := c.Bind(&body); err != nil || body.Username == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "username required"})
		}

		pool := c.PoolPG("primary")
		var userID string
		err := pool.QueryRow(c.Context(), `SELECT id FROM users WHERE username = $1`, body.Username).Scan(&userID)
		if err != nil {
			return c.Status(200).JSON(runtime.Map{"status": "ok"})
		}

		web, user, err := prepareWebAuthnUser(c, userID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		if len(user.credentials) == 0 {
			return c.Status(200).JSON(runtime.Map{"status": "ok"})
		}

		assertion, session, err := web.BeginLogin(user)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": fmt.Sprintf("begin login: %v", err)})
		}

		sessionID, err := storeSession(c, userID, "login_manual", session)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session storage failed"})
		}

		return c.JSON(runtime.Map{
			"session_id": sessionID,
			"assertion":  assertion,
		})
	}
}

func handleWebAuthnManualLoginFinish(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			SessionID string          `json:"session_id"`
			Response  json.RawMessage `json:"response"`
		}
		if err := c.Bind(&body); err != nil || body.SessionID == "" || body.Response == nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "session_id and response required"})
		}

		userID, ceremony, session, err := loadSession(c, body.SessionID)
		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": err.Error()})
		}
		defer deleteSession(c, body.SessionID)

		if ceremony != "login_manual" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid session ceremony"})
		}

		web, user, err := prepareWebAuthnUser(c, userID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		parsed, err := protocol.ParseCredentialRequestResponseBytes([]byte(body.Response))
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": fmt.Sprintf("parse response: %v", err)})
		}

		cred, err := web.ValidateLogin(user, *session, parsed)
		if err != nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": fmt.Sprintf("login: %v", err)})
		}

		return loginWithCredential(c, svcCtx, user, cred)
	}
}

func loginWithCredential(c *runtime.RestCtx, svcCtx *svc.ServiceContext, user *webAuthnUser, cred *webauthn.Credential) error {
	if err := saveCredential(c, user.name, cred); err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "credential update failed"})
	}

	var role string
	pool := c.PoolPG("primary")
	pool.QueryRow(c.Context(), `SELECT role FROM users WHERE id = $1`, user.name).Scan(&role)
	if role == "" {
		role = "viewer"
	}

	sessionToken, err := middleware.SignToken(svcCtx.JWTSecret, "HS256",
		middleware.DefaultClaims(user.name, "", []string{role}, nil, svcCtx.AuthExpiry))
	if err != nil {
		return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session creation failed"})
	}

	c.SetCookie(runtime.NewCookie("token", sessionToken, svcCtx.AuthExpiry))
	log.Printf("[WEBAUTHN] user %s logged in via passkey", user.name)
	return c.JSON(runtime.Map{"token": sessionToken, "role": role, "user_id": user.name})
}

func prepareWebAuthnUser(c *runtime.RestCtx, userID string) (*webauthn.WebAuthn, *webAuthnUser, error) {
	web := newWebAuthn()
	user, err := loadWebAuthnUser(c, userID)
	if err != nil {
		return nil, nil, err
	}
	return web, user, nil
}

func handleWebAuthnCredentials(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}

		creds, err := loadCredentials(c, a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		var list []runtime.Map
		for _, cred := range creds {
			list = append(list, runtime.Map{
				"id":         base64.RawURLEncoding.EncodeToString(cred.ID),
				"attachment": cred.Authenticator.Attachment,
			})
		}
		if list == nil {
			list = []runtime.Map{}
		}
		return c.JSON(runtime.Map{"data": list})
	}
}

func handleWebAuthnDeleteCredential(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		credID := c.Params("id")
		if credID == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "credential id required"})
		}
		deleteCredential(c, a.UserID, credID)
		return c.JSON(runtime.Map{"status": "deleted"})
	}
}
