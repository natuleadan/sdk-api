package auth

// RoleHierarchy defines a role inheritance tree where each role maps to
// a list of roles it inherits from. Use Inherits to check whether a
// user's role satisfies a required role.
//
// Example:
//
//	h := RoleHierarchy{
//	    "viewer": {},
//	    "editor": {"viewer"},
//	    "admin":  {"editor", "viewer"},
//	}
//	h.Inherits("admin", "viewer")  // true
//	h.Inherits("viewer", "admin")  // false
type RoleHierarchy map[string][]string

// Inherits reports whether userRole (or any role it transitively inherits
// from) equals requiredRole.
func (h RoleHierarchy) Inherits(userRole, requiredRole string) bool {
	if userRole == requiredRole {
		return true
	}
	inherited, ok := h[userRole]
	if !ok {
		return false
	}
	for _, r := range inherited {
		if h.Inherits(r, requiredRole) {
			return true
		}
	}
	return false
}
