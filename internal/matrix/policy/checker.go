package policy

import (
	"context"
	"log"

	"github.com/singll/bellkeeper/internal/repository"
)

// Checker checks user permissions
type Checker struct {
	repos      *repository.Repositories
	adminUsers map[string]bool // Global admin users (from config)
}

// NewChecker creates a new permission checker
func NewChecker(repos *repository.Repositories, adminUsers []string) *Checker {
	adminMap := make(map[string]bool)
	for _, u := range adminUsers {
		adminMap[u] = true
	}
	return &Checker{
		repos:      repos,
		adminUsers: adminMap,
	}
}

// SetAdminUsers updates the list of global admin users
func (c *Checker) SetAdminUsers(users []string) {
	c.adminUsers = make(map[string]bool)
	for _, u := range users {
		c.adminUsers[u] = true
	}
}

// CheckCommandPermission checks if a user can execute a command in a room
func (c *Checker) CheckCommandPermission(ctx context.Context, userID, roomID, commandName, permLevel string) (bool, error) {
	// Get user's role in this room
	role, err := c.GetUserRole(ctx, userID, roomID)
	if err != nil {
		return false, err
	}

	// Global admin check
	if c.adminUsers[userID] {
		log.Printf("[Policy] user %s is global admin, granting access", userID)
		return true, nil
	}

	// Check role permission
	allowed := role.CanExecute(permLevel)
	if !allowed {
		log.Printf("[Policy] denied %s (role=%s) to execute %s (require=%s) in room %s",
			userID, role, commandName, permLevel, roomID)
	}
	return allowed, nil
}

// GetUserRole gets the user's role in a specific room
func (c *Checker) GetUserRole(ctx context.Context, userID, roomID string) (Role, error) {
	// Try to get from database
	userRole, err := c.repos.MatrixUserRole.GetByUserAndRoom(ctx, userID, roomID)
	if err == nil && userRole != nil {
		return ParseRole(userRole.Role), nil
	}

	// Check if user is room owner (creator)
	room, err := c.repos.MatrixRoom.GetByRoomID(roomID)
	if err == nil && room != nil {
		// If room has owner info in config, check it
		// For now, default to member if user has any role record
	}

	// Default to guest (no permissions by default)
	return RoleGuest, nil
}

// GetUserRooms gets all rooms where a user has a specific role or higher
func (c *Checker) GetUserRooms(ctx context.Context, userID string, minRole Role) ([]string, error) {
	roles, err := c.repos.MatrixUserRole.GetByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var rooms []string
	for _, ur := range roles {
		r := ParseRole(ur.Role)
		if r.IsHigherThan(minRole) || r == minRole {
			rooms = append(rooms, ur.RoomID)
		}
	}
	return rooms, nil
}

// SetUserRole sets a user's role in a room
func (c *Checker) SetUserRole(ctx context.Context, userID, roomID string, role Role) error {
	return c.repos.MatrixUserRole.Upsert(ctx, userID, roomID, role.String())
}

// RemoveUserRole removes a user's role from a room
func (c *Checker) RemoveUserRole(ctx context.Context, userID, roomID string) error {
	return c.repos.MatrixUserRole.Delete(ctx, userID, roomID)
}

// IsAdmin checks if user is a global admin
func (c *Checker) IsAdmin(userID string) bool {
	return c.adminUsers[userID]
}
