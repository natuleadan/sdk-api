package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/ory/fosite"
)

type fositeRW struct {
	header http.Header
	buf    bytes.Buffer
	code   int
}

func (w *fositeRW) Header() http.Header         { return w.header }
func (w *fositeRW) Write(b []byte) (int, error)  { return w.buf.Write(b) }
func (w *fositeRW) WriteHeader(code int)          { w.code = code }
func (w *fositeRW) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func handleOAuthAuthorize(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "login required to authorize"})
		}

		fositeReq, err := svcCtx.OAuth.Provider.NewAuthorizeRequest(c.Context(), buildHTTPReq(c))
		if err != nil {
			rw := &fositeRW{header: http.Header{}}
			svcCtx.OAuth.Provider.WriteAuthorizeError(c.Context(), rw, fositeReq, err)
			return writeFositeRW(c, rw)
		}

		session := svcCtx.OAuth.GetSession(a.UserID, a.OrgID)
		response, err := svcCtx.OAuth.Provider.NewAuthorizeResponse(c.Context(), fositeReq, session)
		if err != nil {
			rw := &fositeRW{header: http.Header{}}
			svcCtx.OAuth.Provider.WriteAuthorizeError(c.Context(), rw, fositeReq, err)
			return writeFositeRW(c, rw)
		}

		rw := &fositeRW{header: http.Header{}}
		svcCtx.OAuth.Provider.WriteAuthorizeResponse(c.Context(), rw, fositeReq, response)
		return writeFositeRW(c, rw)
	}
}

func handleOAuthToken(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		httpReq := buildHTTPReq(c)
		accessRequest, err := svcCtx.OAuth.Provider.NewAccessRequest(c.Context(), httpReq, &fosite.DefaultSession{})
		if err != nil {
			log.Printf("[OAUTH] token error: %v", err)
			rw := &fositeRW{header: http.Header{}}
			svcCtx.OAuth.Provider.WriteAccessError(c.Context(), rw, accessRequest, err)
			return writeFositeRW(c, rw)
		}

		if accessRequest.GetGrantTypes().ExactOne("client_credentials") {
			session := accessRequest.GetSession().(*fosite.DefaultSession)
			session.Subject = accessRequest.GetClient().GetID()
		}

		response, err := svcCtx.OAuth.Provider.NewAccessResponse(c.Context(), accessRequest)
		if err != nil {
			rw := &fositeRW{header: http.Header{}}
			svcCtx.OAuth.Provider.WriteAccessError(c.Context(), rw, accessRequest, err)
			return writeFositeRW(c, rw)
		}

		rw := &fositeRW{header: http.Header{}}
		svcCtx.OAuth.Provider.WriteAccessResponse(c.Context(), rw, accessRequest, response)
		return writeFositeRW(c, rw)
	}
}

func handleOAuthIntrospect(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		ir, err := svcCtx.OAuth.Provider.NewIntrospectionRequest(c.Context(), buildHTTPReq(c), &fosite.DefaultSession{})
		if err != nil {
			rw := &fositeRW{header: http.Header{}}
			svcCtx.OAuth.Provider.WriteIntrospectionError(c.Context(), rw, err)
			return writeFositeRW(c, rw)
		}

		rw := &fositeRW{header: http.Header{}}
		svcCtx.OAuth.Provider.WriteIntrospectionResponse(c.Context(), rw, ir)
		return writeFositeRW(c, rw)
	}
}

func handleOAuthRevoke(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		err := svcCtx.OAuth.Provider.NewRevocationRequest(c.Context(), buildHTTPReq(c))
		rw := &fositeRW{header: http.Header{}}
		svcCtx.OAuth.Provider.WriteRevocationResponse(c.Context(), rw, err)
		return writeFositeRW(c, rw)
	}
}

func buildHTTPReq(c *runtime.RestCtx) *http.Request {
	body := c.Body()
	req, _ := http.NewRequest(c.Method(), c.Path(), bytes.NewReader(body))
	ct := c.Get("Content-Type")
	if ct == "" {
		ct = "application/x-www-form-urlencoded"
	}
	// Support JSON body: convert to form-encoded for fosite
	if ct == "application/json" {
		var data map[string]string
		if json.Unmarshal(body, &data) == nil && len(data) > 0 {
			vals := url.Values{}
			for k, v := range data {
				vals.Set(k, v)
			}
			req, _ = http.NewRequest("POST", c.Path(), strings.NewReader(vals.Encode()))
			ct = "application/x-www-form-urlencoded"
		}
	}
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", c.Get("Authorization"))
	req.Host = "localhost:23400"
	return req
}

func writeFositeRW(c *runtime.RestCtx, rw *fositeRW) error {
	for k, vals := range rw.header {
		for _, v := range vals {
			c.Set(k, v)
		}
	}
	if rw.code > 0 {
		c.Status(rw.code)
	}
	if rw.buf.Len() > 0 {
		var result map[string]any
		if json.Unmarshal(rw.buf.Bytes(), &result) == nil && result != nil {
			return c.JSON(result)
		}
		return c.SendString(rw.buf.String())
	}
	return nil
}

func init() {
	_ = time.Second
	_ = fosite.ErrInactiveToken
	_ = json.Marshal
}

func handleOAuthClientsList(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := c.PoolPG("primary")
		rows, err := pool.Query(c.Context(),
			`SELECT id, grant_types, response_types, scopes, is_public FROM oauth_clients ORDER BY id`)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		defer rows.Close()
		var clients []runtime.Map
		for rows.Next() {
			var id, scopes string
			var grantTypes, responseTypes []string
			var isPublic bool
			rows.Scan(&id, &grantTypes, &responseTypes, &scopes, &isPublic)
			clients = append(clients, runtime.Map{
				"id": id, "grant_types": grantTypes,
				"response_types": responseTypes, "scopes": scopes, "is_public": isPublic,
			})
		}
		if clients == nil {
			clients = []runtime.Map{}
		}
		return c.JSON(runtime.Map{"data": clients})
	}
}

func handleOAuthClientsCreate(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		var body struct {
			ID            string   `json:"id"`
			Secret        string   `json:"secret"`
			RedirectURIs  []string `json:"redirect_uris"`
			GrantTypes    []string `json:"grant_types"`
			ResponseTypes []string `json:"response_types"`
			Scopes        string   `json:"scopes"`
			IsPublic      bool     `json:"is_public"`
		}
		if err := c.Bind(&body); err != nil || body.ID == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "client id required"})
		}
		if len(body.GrantTypes) == 0 {
			body.GrantTypes = []string{"authorization_code", "client_credentials", "refresh_token"}
		}
		if len(body.ResponseTypes) == 0 {
			body.ResponseTypes = []string{"code"}
		}
		for _, gt := range body.GrantTypes {
			if gt == "authorization_code" && len(body.RedirectURIs) == 0 {
				return c.Status(400).JSON(runtime.Map{"code": 400, "message": "redirect_uris required for authorization_code grant"})
			}
		}
		if body.Secret == "" && !body.IsPublic {
			body.Secret = body.ID
		}

		pool := c.PoolPG("primary")
		hashed := hashClientSecret(body.Secret)
		_, err := pool.Exec(c.Context(),
			`INSERT INTO oauth_clients (id, hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public)
			 VALUES ($1, $2, $3, $4, $5, $6, '{}', $7)`,
			body.ID, hashed, body.RedirectURIs, body.GrantTypes, body.ResponseTypes, body.Scopes, body.IsPublic)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.Status(201).JSON(runtime.Map{"status": "created", "id": body.ID})
	}
}

func handleOAuthClientsDelete(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		clientID := c.Params("id")
		if clientID == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "client id required"})
		}
		pool := c.PoolPG("primary")
		_, err := pool.Exec(c.Context(), `DELETE FROM oauth_clients WHERE id = $1`, clientID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "deleted"})
	}
}

func hashClientSecret(secret string) []byte {
	h, _ := auth.HashPassword(secret)
	return []byte(h)
}
