package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLogSourceRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	src := &model.LogSource{
		Name:        "test-source",
		SourceType:  "internal",
		Description: "A test source",
		APIKey:      "key123",
		IsActive:    true,
	}
	assertNoError(t, repo.Create(src), "Create")
	assertEqual(t, src.ID > 0, true)

	got, err := repo.GetByID(src.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "test-source")
	assertEqual(t, got.SourceType, "internal")
}

func TestLogSourceRepository_GetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	assertNoError(t, repo.Create(&model.LogSource{Name: "src1", SourceType: "internal"}), "Create")

	got, err := repo.GetByName("src1")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.Name, "src1")

	_, err = repo.GetByName("nonexistent")
	assertError(t, err, "GetByName nonexistent")
}

func TestLogSourceRepository_GetByAPIKey(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	assertNoError(t, repo.Create(&model.LogSource{Name: "src1", SourceType: "internal", APIKey: "key1", IsActive: true}), "Create")

	got, err := repo.GetByAPIKey("key1")
	assertNoError(t, err, "GetByAPIKey")
	assertEqual(t, got.Name, "src1")

	_, err = repo.GetByAPIKey("wrongkey")
	assertError(t, err, "GetByAPIKey wrong")
}

func TestLogSourceRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	assertNoError(t, repo.Create(&model.LogSource{Name: "src1", SourceType: "internal"}), "Create 1")
	assertNoError(t, repo.Create(&model.LogSource{Name: "src2", SourceType: "n8n"}), "Create 2")

	sources, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(sources), 2)
}

func TestLogSourceRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	src := &model.LogSource{Name: "src1", SourceType: "internal", Description: "old"}
	assertNoError(t, repo.Create(src), "Create")

	src.Description = "new"
	assertNoError(t, repo.Update(src), "Update")

	got, err := repo.GetByID(src.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Description, "new")
}

func TestLogSourceRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogSourceRepository(db)

	src := &model.LogSource{Name: "src1", SourceType: "internal"}
	assertNoError(t, repo.Create(src), "Create")
	assertNoError(t, repo.Delete(src.ID), "Delete")

	_, err := repo.GetByID(src.ID)
	assertError(t, err, "GetByID after delete")
}
