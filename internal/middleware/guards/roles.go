// internal/middleware/guards/roles.go
package guards

// Role constants — single source of truth for every role string used in
// middleware, guards, services and repositories.
//
// Hierarchy (highest → lowest privilege):
//
//	SuperAdmin → Admin → Staff → Resident
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleStaff      = "staff"
	RoleResident   = "resident"
)
