package repository

import (
	"context"
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixUserRoleRepository_GetByUserAndRoom(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	role := &model.MatrixUserRole{
		UserID: "@user:matrix.org",
		RoomID: "!room1:matrix.org",
		Role:   "admin",
	}
	assertNoError(t, db.Create(role).Error, "Create")

	got, err := repo.GetByUserAndRoom(ctx, "@user:matrix.org", "!room1:matrix.org")
	assertNoError(t, err, "GetByUserAndRoom")
	assertEqual(t, got.Role, "admin")

	got, err = repo.GetByUserAndRoom(ctx, "@user:matrix.org", "!nonexistent:matrix.org")
	assertNoError(t, err, "GetByUserAndRoom nonexistent")
	assertEqual(t, got, nil)
}

func TestMatrixUserRoleRepository_GetByUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@u:m.org", RoomID: "!r1:m.org", Role: "admin"}).Error, "Create 1")
	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@u:m.org", RoomID: "!r2:m.org", Role: "member"}).Error, "Create 2")
	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@other:m.org", RoomID: "!r1:m.org", Role: "guest"}).Error, "Create 3")

	roles, err := repo.GetByUser(ctx, "@u:m.org")
	assertNoError(t, err, "GetByUser")
	assertEqual(t, len(roles), 2)
}

func TestMatrixUserRoleRepository_GetByRoom(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@u1:m.org", RoomID: "!r1:m.org", Role: "admin"}).Error, "Create 1")
	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@u2:m.org", RoomID: "!r1:m.org", Role: "member"}).Error, "Create 2")
	assertNoError(t, db.Create(&model.MatrixUserRole{UserID: "@u1:m.org", RoomID: "!r2:m.org", Role: "guest"}).Error, "Create 3")

	roles, err := repo.GetByRoom(ctx, "!r1:m.org")
	assertNoError(t, err, "GetByRoom")
	assertEqual(t, len(roles), 2)
}

func TestMatrixUserRoleRepository_Upsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Upsert(ctx, "@u:m.org", "!r1:m.org", "member"), "Upsert create")

	got, err := repo.GetByUserAndRoom(ctx, "@u:m.org", "!r1:m.org")
	assertNoError(t, err, "GetByUserAndRoom")
	assertEqual(t, got.Role, "member")

	assertNoError(t, repo.Upsert(ctx, "@u:m.org", "!r1:m.org", "admin"), "Upsert update")

	_, err = repo.GetByUserAndRoom(ctx, "@u:m.org", "!r1:m.org")
	assertNoError(t, err, "GetByUserAndRoom after update")
}

func TestMatrixUserRoleRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Upsert(ctx, "@u:m.org", "!r1:m.org", "member"), "Upsert")
	assertNoError(t, repo.Delete(ctx, "@u:m.org", "!r1:m.org"), "Delete")

	got, err := repo.GetByUserAndRoom(ctx, "@u:m.org", "!r1:m.org")
	assertNoError(t, err, "GetByUserAndRoom after delete")
	assertEqual(t, got, nil)
}

func TestMatrixUserRoleRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Upsert(ctx, "@u1:m.org", "!r1:m.org", "admin"), "Upsert 1")
	assertNoError(t, repo.Upsert(ctx, "@u2:m.org", "!r1:m.org", "member"), "Upsert 2")

	roles, err := repo.List(ctx, 10, 0)
	assertNoError(t, err, "List")
	assertEqual(t, len(roles), 2)
}

func TestMatrixUserRoleRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixUserRoleRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Upsert(ctx, "@u1:m.org", "!r1:m.org", "admin"), "Upsert 1")
	assertNoError(t, repo.Upsert(ctx, "@u2:m.org", "!r1:m.org", "member"), "Upsert 2")

	count, err := repo.Count(ctx)
	assertNoError(t, err, "Count")
	assertEqual(t, count, int64(2))
}
