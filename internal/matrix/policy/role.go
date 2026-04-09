package policy

import "strings"

// Role represents a user's role in a room
type Role string

const (
	RoleOwner   Role = "owner"   // Room creator - all permissions
	RoleAdmin   Role = "admin"   // Admin - can execute admin commands
	RoleMember  Role = "member"  // Member - can execute user commands
	RoleGuest   Role = "guest"   // Guest - read only, no commands
	RoleUnknown Role = "unknown" // Unknown role
)

// Permission represents a specific permission
type Permission string

const (
	PermissionExecuteUserCommand   Permission = "execute:user_command"
	PermissionExecuteAdminCommand  Permission = "execute:admin_command"
	PermissionManageRoom           Permission = "manage:room"
	PermissionManageChannel        Permission = "manage:channel"
	PermissionManageCommand        Permission = "manage:command"
	PermissionViewEvents           Permission = "view:events"
	PermissionManageUsers         Permission = "manage:users"
)

// RoleHierarchy defines the hierarchy of roles (higher index = more permissions)
var RoleHierarchy = []Role{
	RoleGuest,
	RoleMember,
	RoleAdmin,
	RoleOwner,
}

// RolePermissions maps roles to their permissions
var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		PermissionExecuteUserCommand,
		PermissionExecuteAdminCommand,
		PermissionManageRoom,
		PermissionManageChannel,
		PermissionManageCommand,
		PermissionViewEvents,
		PermissionManageUsers,
	},
	RoleAdmin: {
		PermissionExecuteUserCommand,
		PermissionExecuteAdminCommand,
		PermissionManageChannel,
		PermissionManageCommand,
		PermissionViewEvents,
	},
	RoleMember: {
		PermissionExecuteUserCommand,
		PermissionViewEvents,
	},
	RoleGuest: {
		PermissionViewEvents,
	},
}

// ParseRole parses a role string to Role
func ParseRole(s string) Role {
	switch strings.ToLower(s) {
	case "owner":
		return RoleOwner
	case "admin":
		return RoleAdmin
	case "member":
		return RoleMember
	case "guest":
		return RoleGuest
	default:
		return RoleUnknown
	}
}

// String returns the string representation of a role
func (r Role) String() string {
	return string(r)
}

// HasPermission checks if a role has a specific permission
func (r Role) HasPermission(perm Permission) bool {
	perms, ok := RolePermissions[r]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// CanExecute checks if a role can execute a command based on permission level
func (r Role) CanExecute(permLevel string) bool {
	switch strings.ToLower(permLevel) {
	case "guest":
		return r == RoleOwner || r == RoleAdmin || r == RoleMember
	case "user":
		return r == RoleOwner || r == RoleAdmin || r == RoleMember
	case "admin":
		return r == RoleOwner || r == RoleAdmin
	case "owner":
		return r == RoleOwner
	default:
		return r == RoleOwner
	}
}

// IsHigherThan checks if this role is higher than another
func (r Role) IsHigherThan(other Role) bool {
	rIdx := r.index()
	oIdx := other.index()
	if rIdx < 0 || oIdx < 0 {
		return false
	}
	return rIdx > oIdx
}

func (r Role) index() int {
	for i, role := range RoleHierarchy {
		if role == r {
			return i
		}
	}
	return -1
}
