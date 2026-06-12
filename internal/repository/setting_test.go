package repository

import (
	"testing"
)

func TestSettingRepository_SetAndGetByKey(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingRepository(db)

	err := repo.Set("test_key", "test_value", "string", "test_cat", "desc", false)
	assertNoError(t, err, "Set")

	got, err := repo.GetByKey("test_key")
	assertNoError(t, err, "GetByKey")
	assertEqual(t, got.Value, "test_value")
	assertEqual(t, got.Category, "test_cat")
	assertEqual(t, got.IsSecret, false)
}

func TestSettingRepository_SetUpdatesExisting(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingRepository(db)

	assertNoError(t, repo.Set("k", "v1", "string", "c", "", false), "Set v1")
	assertNoError(t, repo.Set("k", "v2", "string", "c2", "", true), "Set v2")

	got, err := repo.GetByKey("k")
	assertNoError(t, err, "GetByKey")
	assertEqual(t, got.Value, "v2")
	assertEqual(t, got.Category, "c2")
	assertEqual(t, got.IsSecret, true)
}

func TestSettingRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingRepository(db)

	assertNoError(t, repo.Set("a", "1", "string", "cat1", "", false), "Set a")
	assertNoError(t, repo.Set("b", "2", "string", "cat2", "", false), "Set b")
	assertNoError(t, repo.Set("c", "3", "string", "cat1", "", false), "Set c")

	all, err := repo.List("")
	assertNoError(t, err, "List all")
	assertEqual(t, len(all), 3)

	cat1, err := repo.List("cat1")
	assertNoError(t, err, "List cat1")
	assertEqual(t, len(cat1), 2)
}

func TestSettingRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingRepository(db)

	assertNoError(t, repo.Set("k", "v", "string", "c", "", false), "Set")
	assertNoError(t, repo.Delete("k"), "Delete")

	_, err := repo.GetByKey("k")
	assertError(t, err, "GetByKey after delete")
}

func TestSettingRepository_GetByKeyNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingRepository(db)

	_, err := repo.GetByKey("nonexistent")
	assertError(t, err, "GetByKey nonexistent")
}
