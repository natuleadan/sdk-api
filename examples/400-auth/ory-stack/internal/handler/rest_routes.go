package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// RegisterRestRoutes wires the handlers referenced by service.yaml. Identity
// flows (login, MFA, WebAuthn, social, OAuth2/OIDC) are provided by Ory Kratos;
// authorization is Ory Keto. Only resource and admin handlers live here.
func RegisterRestRoutes(s *runtime.Service, svcCtx *svc.ServiceContext) {
	s.WithRest("listProducts", handleListProducts(svcCtx))
	s.WithRest("createProduct", handleCreateProduct(svcCtx))
	s.WithRest("getProduct", handleGetProduct(svcCtx))
	s.WithRest("updateProduct", handleUpdateProduct(svcCtx))
	s.WithRest("deleteProduct", handleDeleteProduct(svcCtx))
	s.WithRest("hardDeleteProduct", handleHardDeleteProduct(svcCtx))
	s.WithRest("setVisibility", handleSetVisibility(svcCtx))
	s.WithRest("getAuditLog", handleGetAuditLog(svcCtx))
	s.WithRest("listUsers", handleListUsers(svcCtx))
	s.WithRest("deleteUser", handleDeleteUser(svcCtx))
	s.WithRest("setUserRole", handleSetUserRole(svcCtx))
	s.WithRest("rateLimitedHandler", handleRateLimited(svcCtx))
	s.WithRest("perUserLimited", handlePerUserLimited(svcCtx))
	s.WithRest("perKeyLimited", handlePerKeyLimited(svcCtx))
	s.WithRest("perRoleLimited", handlePerRoleLimited(svcCtx))
	s.WithRest("maxFuncLimited", handleMaxFuncLimited(svcCtx))
	s.WithRest("noCSRFHandler", handleNoCSRF(svcCtx))
	s.WithRest("cookieProfile", handleCookieProfile(svcCtx))
	s.WithRest("viewerDataHandler", handleViewerData(svcCtx))
	s.WithRest("grantRole", handleGrantRole(svcCtx))
	s.WithRest("revokeRole", handleRevokeRole(svcCtx))
	s.WithRest("myRoles", handleMyRoles(svcCtx))
	s.WithRest("createTeam", handleCreateTeam(svcCtx))
	s.WithRest("listTeams", handleListTeams(svcCtx))
	s.WithRest("hydraLogin", handleHydraLogin(svcCtx))
	s.WithRest("hydraConsent", handleHydraConsent(svcCtx))
	s.WithRest("hydraLogout", handleHydraLogout(svcCtx))
}
