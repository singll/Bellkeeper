package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRole(t *testing.T) {
	tests := []struct {
		input    string
		expected Role
	}{
		{"owner", RoleOwner},
		{"Owner", RoleOwner},
		{"OWNER", RoleOwner},
		{"admin", RoleAdmin},
		{"member", RoleMember},
		{"guest", RoleGuest},
		{"invalid", RoleUnknown},
		{"", RoleUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoleString(t *testing.T) {
	assert.Equal(t, "owner", RoleOwner.String())
	assert.Equal(t, "admin", RoleAdmin.String())
	assert.Equal(t, "member", RoleMember.String())
	assert.Equal(t, "guest", RoleGuest.String())
}

func TestRoleHasPermission(t *testing.T) {
	// Owner should have all permissions
	assert.True(t, RoleOwner.HasPermission(PermissionExecuteUserCommand))
	assert.True(t, RoleOwner.HasPermission(PermissionExecuteAdminCommand))
	assert.True(t, RoleOwner.HasPermission(PermissionManageRoom))
	assert.True(t, RoleOwner.HasPermission(PermissionManageChannel))
	assert.True(t, RoleOwner.HasPermission(PermissionManageCommand))
	assert.True(t, RoleOwner.HasPermission(PermissionViewEvents))
	assert.True(t, RoleOwner.HasPermission(PermissionManageUsers))

	// Admin should not have manage users permission
	assert.True(t, RoleAdmin.HasPermission(PermissionExecuteUserCommand))
	assert.True(t, RoleAdmin.HasPermission(PermissionExecuteAdminCommand))
	assert.False(t, RoleAdmin.HasPermission(PermissionManageUsers))

	// Member should only have user command and view permissions
	assert.True(t, RoleMember.HasPermission(PermissionExecuteUserCommand))
	assert.False(t, RoleMember.HasPermission(PermissionExecuteAdminCommand))
	assert.False(t, RoleMember.HasPermission(PermissionManageRoom))
	assert.True(t, RoleMember.HasPermission(PermissionViewEvents))

	// Guest should only have view permission
	assert.False(t, RoleGuest.HasPermission(PermissionExecuteUserCommand))
	assert.False(t, RoleGuest.HasPermission(PermissionExecuteAdminCommand))
	assert.True(t, RoleGuest.HasPermission(PermissionViewEvents))
	assert.False(t, RoleGuest.HasPermission(PermissionManageUsers))

	// Unknown role should have no permissions
	assert.False(t, RoleUnknown.HasPermission(PermissionExecuteUserCommand))
}

func TestRoleCanExecute(t *testing.T) {
	tests := []struct {
		role      Role
		permLevel string
		expected  bool
	}{
		// Guest permission level
		{RoleOwner, "guest", true},
		{RoleAdmin, "guest", true},
		{RoleMember, "guest", true},
		{RoleGuest, "guest", false},

		// User permission level
		{RoleOwner, "user", true},
		{RoleAdmin, "user", true},
		{RoleMember, "user", true},
		{RoleGuest, "user", false},

		// Admin permission level
		{RoleOwner, "admin", true},
		{RoleAdmin, "admin", true},
		{RoleMember, "admin", false},
		{RoleGuest, "admin", false},

		// Owner permission level
		{RoleOwner, "owner", true},
		{RoleAdmin, "owner", false},
		{RoleMember, "owner", false},
		{RoleGuest, "owner", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_"+tt.permLevel, func(t *testing.T) {
			result := tt.role.CanExecute(tt.permLevel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoleIsHigherThan(t *testing.T) {
	// Guest is lowest
	assert.False(t, RoleGuest.IsHigherThan(RoleGuest))
	assert.True(t, RoleMember.IsHigherThan(RoleGuest))
	assert.True(t, RoleAdmin.IsHigherThan(RoleGuest))
	assert.True(t, RoleOwner.IsHigherThan(RoleGuest))

	// Member is above guest
	assert.False(t, RoleMember.IsHigherThan(RoleMember))
	assert.False(t, RoleMember.IsHigherThan(RoleAdmin))
	assert.False(t, RoleMember.IsHigherThan(RoleOwner))
	assert.True(t, RoleAdmin.IsHigherThan(RoleMember))
	assert.True(t, RoleOwner.IsHigherThan(RoleMember))

	// Admin is above member
	assert.False(t, RoleAdmin.IsHigherThan(RoleAdmin))
	assert.False(t, RoleAdmin.IsHigherThan(RoleOwner))
	assert.True(t, RoleOwner.IsHigherThan(RoleAdmin))

	// Owner is highest
	assert.False(t, RoleOwner.IsHigherThan(RoleOwner))

	// Unknown role
	assert.False(t, RoleUnknown.IsHigherThan(RoleGuest))
	assert.False(t, RoleGuest.IsHigherThan(RoleUnknown))
}

func TestRoleIndex(t *testing.T) {
	assert.Equal(t, 0, RoleGuest.index())
	assert.Equal(t, 1, RoleMember.index())
	assert.Equal(t, 2, RoleAdmin.index())
	assert.Equal(t, 3, RoleOwner.index())
	assert.Equal(t, -1, RoleUnknown.index())
}
