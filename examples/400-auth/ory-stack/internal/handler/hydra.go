package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// hydraAdminURL is the Hydra admin API (login/consent accept/reject).
func hydraAdminURL() string {
	if v := os.Getenv("HYDRA_ADMIN_URL"); v != "" {
		return v
	}
	return "http://localhost:4445"
}

// handleHydraLogin is Hydra's login provider. It resumes the login by accepting
// the challenge for the caller's Kratos identity (the subject). A real UI would
// authenticate first; here the session middleware already established identity.
func handleHydraLogin(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		challenge := c.Query("login_challenge")
		if challenge == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "login_challenge required"})
		}
		subject := ""
		if a := getAuth(c); a != nil {
			subject = a.UserID
		}
		if subject == "" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "login required"})
		}
		var out struct {
			RedirectTo string `json:"redirect_to"`
		}
		if err := hydraJSON(c, http.MethodPut, "/admin/oauth2/auth/requests/login/accept?login_challenge="+url.QueryEscape(challenge),
			map[string]any{"subject": subject}, &out); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"redirect_to": out.RedirectTo})
	}
}

// handleHydraConsent accepts the consent challenge, granting the requested
// scopes carried in the request.
func handleHydraConsent(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		challenge := c.Query("consent_challenge")
		if challenge == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "consent_challenge required"})
		}
		subject := ""
		if a := getAuth(c); a != nil {
			subject = a.UserID
		}
		body := map[string]any{
			"grant_scope": []string{"openid", "profile", "email"},
		}
		if subject != "" {
			body["subject"] = subject
		}
		var out struct {
			RedirectTo string `json:"redirect_to"`
		}
		if err := hydraJSON(c, http.MethodPut, "/admin/oauth2/auth/requests/consent/accept?consent_challenge="+url.QueryEscape(challenge),
			body, &out); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"redirect_to": out.RedirectTo})
	}
}

// handleHydraLogout accepts the logout challenge.
func handleHydraLogout(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		challenge := c.Query("logout_challenge")
		if challenge == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "logout_challenge required"})
		}
		var out struct {
			RedirectTo string `json:"redirect_to"`
		}
		if err := hydraJSON(c, http.MethodPut, "/admin/oauth2/auth/requests/logout/accept?logout_challenge="+url.QueryEscape(challenge),
			nil, &out); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"redirect_to": out.RedirectTo})
	}
}

// hydraJSON performs a JSON request against the Hydra admin API.
func hydraJSON(c *runtime.RestCtx, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(c.Context(), method, hydraAdminURL()+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &hydraError{status: resp.StatusCode, body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type hydraError struct {
	status int
	body   string
}

func (e *hydraError) Error() string {
	return "hydra: admin returned " + http.StatusText(e.status) + ": " + e.body
}
