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
// from) equals requiredRole. It is cycle-safe: a malformed hierarchy cannot
// loop forever.
func (h RoleHierarchy) Inherits(userRole, requiredRole string) bool {
	return h.inherits(userRole, requiredRole, map[string]bool{})
}

func (h RoleHierarchy) inherits(userRole, requiredRole string, seen map[string]bool) bool {
	if userRole == requiredRole {
		return true
	}
	if seen[userRole] {
		return false
	}
	seen[userRole] = true
	for _, r := range h[userRole] {
		if h.inherits(r, requiredRole, seen) {
			return true
		}
	}
	return false
}

// Satisfies reports whether any of the user's roles equals or inherits the
// required role.
func (h RoleHierarchy) Satisfies(userRoles []string, requiredRole string) bool {
	for _, r := range userRoles {
		if h.Inherits(r, requiredRole) {
			return true
		}
	}
	return false
}

// MatchPermission reports whether a granted permission covers a required one,
// supporting trailing wildcards: "products:*" covers "products:edit:price",
// "*" covers everything, and an exact string matches itself.
func MatchPermission(granted, required string) bool {
	if granted == required {
		return true
	}
	if i := indexByte(granted, '*'); i >= 0 {
		prefix := granted[:i]
		return len(required) >= len(prefix) && required[:len(prefix)] == prefix
	}
	return false
}

// MatchAnyPermission reports whether any granted permission covers required.
func MatchAnyPermission(granted []string, required string) bool {
	for _, g := range granted {
		if MatchPermission(g, required) {
			return true
		}
	}
	return false
}

// indexByte is strings.IndexByte without importing strings for one call.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
