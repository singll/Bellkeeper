package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestDatasetMappingRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{
		Name:        "test-dataset",
		DisplayName: "Test Dataset",
		DatasetID:   "ds_001",
		Description: "A test dataset",
		IsActive:    true,
		Metadata:    datatypes.JSON(`{"key":"value"}`),
	}
	assertNoError(t, repo.Create(dm), "Create")
	assertEqual(t, dm.ID > 0, true)

	got, err := repo.GetByID(dm.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "test-dataset")
	assertEqual(t, got.DatasetID, "ds_001")
}

func TestDatasetMappingRepository_GetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "my-ds", DatasetID: "ds1"}
	assertNoError(t, repo.Create(dm), "Create")

	got, err := repo.GetByName("my-ds")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.Name, "my-ds")

	_, err = repo.GetByName("nonexistent")
	assertError(t, err, "GetByName nonexistent")
}

func TestDatasetMappingRepository_GetByDisplayName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "ds1", DisplayName: "My Dataset", DatasetID: "ds1"}
	assertNoError(t, repo.Create(dm), "Create")

	got, err := repo.GetByDisplayName("My Dataset")
	assertNoError(t, err, "GetByDisplayName")
	assertEqual(t, got.DisplayName, "My Dataset")
}

func TestDatasetMappingRepository_GetDefault(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "default-ds", DatasetID: "ds1", IsDefault: true, IsActive: true}
	assertNoError(t, repo.Create(dm), "Create")

	got, err := repo.GetDefault()
	assertNoError(t, err, "GetDefault")
	assertEqual(t, got.IsDefault, true)
}

func TestDatasetMappingRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "ds1", DatasetID: "ds1", Description: "old"}
	assertNoError(t, repo.Create(dm), "Create")

	dm.Description = "new"
	assertNoError(t, repo.Update(dm), "Update")

	got, err := repo.GetByID(dm.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Description, "new")
}

func TestDatasetMappingRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "ds1", DatasetID: "ds1"}
	assertNoError(t, repo.Create(dm), "Create")
	assertNoError(t, repo.Delete(dm.ID), "Delete")

	_, err := repo.GetByID(dm.ID)
	assertError(t, err, "GetByID after delete")
}

func TestDatasetMappingRepository_UpdateTags(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)
	tagRepo := NewTagRepository(db)

	dm := &model.DatasetMapping{Name: "ds1", DatasetID: "ds1"}
	assertNoError(t, repo.Create(dm), "Create dm")

	t1 := &model.Tag{Name: "tag1", Color: "#1"}
	t2 := &model.Tag{Name: "tag2", Color: "#2"}
	assertNoError(t, tagRepo.Create(t1), "Create t1")
	assertNoError(t, tagRepo.Create(t2), "Create t2")

	assertNoError(t, repo.UpdateTags(dm, []model.Tag{*t1, *t2}), "UpdateTags")

	got, err := repo.GetByID(dm.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, len(got.Tags), 2)
}

func TestDatasetMappingRepository_CreateArticleTag(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	at := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ArticleTitle: "Test"}
	assertNoError(t, repo.CreateArticleTag(at), "CreateArticleTag")
	assertEqual(t, at.ID > 0, true)
}

func TestDatasetMappingRepository_GetArticleTagByURL(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	at := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ArticleURL: "https://example.com/a"}
	assertNoError(t, repo.CreateArticleTag(at), "CreateArticleTag")

	got, err := repo.GetArticleTagByURL("https://example.com/a")
	assertNoError(t, err, "GetArticleTagByURL")
	assertEqual(t, got.ArticleURL, "https://example.com/a")

	_, err = repo.GetArticleTagByURL("https://nonexistent.com")
	assertError(t, err, "GetArticleTagByURL nonexistent")
}

func TestDatasetMappingRepository_ArticleURLExists(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	at := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ArticleURL: "https://example.com/a"}
	assertNoError(t, repo.CreateArticleTag(at), "CreateArticleTag")

	exists, err := repo.ArticleURLExists("https://example.com/a")
	assertNoError(t, err, "ArticleURLExists")
	assertEqual(t, exists, true)

	exists, err = repo.ArticleURLExists("https://nonexistent.com")
	assertNoError(t, err, "ArticleURLExists nonexistent")
	assertEqual(t, exists, false)
}

func TestDatasetMappingRepository_GetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	assertNoError(t, repo.Create(&model.DatasetMapping{Name: "ds1", DatasetID: "ds1", IsActive: true}), "Create 1")
	assertNoError(t, repo.Create(&model.DatasetMapping{Name: "ds2", DatasetID: "ds2", IsActive: false}), "Create 2")

	mappings, err := repo.GetAll()
	assertNoError(t, err, "GetAll")
	for _, m := range mappings {
		assertEqual(t, m.IsActive, true)
	}
}

func TestDatasetMappingRepository_UpdateDatasetID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	dm := &model.DatasetMapping{Name: "ds1", DatasetID: "old-id"}
	assertNoError(t, repo.Create(dm), "Create")
	assertNoError(t, repo.UpdateDatasetID(dm.ID, "new-id"), "UpdateDatasetID")

	got, err := repo.GetByID(dm.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.DatasetID, "new-id")
}

func TestDatasetMappingRepository_GetAllArticleURLs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	assertNoError(t, repo.CreateArticleTag(&model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ArticleURL: "https://a.com", ArticleTitle: "A"}), "Create 1")
	assertNoError(t, repo.CreateArticleTag(&model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", ArticleURL: "https://b.com", ArticleTitle: "B"}), "Create 2")

	ats, err := repo.GetAllArticleURLs()
	assertNoError(t, err, "GetAllArticleURLs")
	assertEqual(t, len(ats), 2)
}

func TestDatasetMappingRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDatasetMappingRepository(db)

	assertNoError(t, repo.Create(&model.DatasetMapping{Name: "ds1", DatasetID: "ds1"}), "Create 1")
	assertNoError(t, repo.Create(&model.DatasetMapping{Name: "ds2", DatasetID: "ds2"}), "Create 2")

	mappings, total, err := repo.List(1, 10)
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(mappings), 2)
}
