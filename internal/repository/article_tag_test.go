package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestArticleTagRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)
	tagRepo := NewTagRepository(db)

	tag := &model.Tag{Name: "test-tag", Color: "#1"}
	assertNoError(t, tagRepo.Create(tag), "Create tag")

	article := &model.ArticleTag{
		DocumentID:   "doc1",
		DatasetID:    "ds1",
		TagID:        tag.ID,
		ArticleTitle: "Test Article",
		ArticleURL:   "https://example.com/article",
		ContentHash:  "abc123",
		SourceDomain: "example.com",
		FilePath:     "/path/to/file.md",
		Layer:        "raw",
		IngestStatus: "ingested",
		IndexStatus:  "pending",
	}
	assertNoError(t, repo.Create(article), "Create")
	assertEqual(t, article.ID > 0, true)

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.ArticleTitle, "Test Article")
	assertEqual(t, got.ArticleURL, "https://example.com/article")
}

func TestArticleTagRepository_GetByURL(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1",
		ArticleURL: "https://example.com/a",
	}
	assertNoError(t, repo.Create(article), "Create")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.ArticleURL, "https://example.com/a")

	_, err = repo.GetByURL("https://nonexistent.com")
	assertError(t, err, "GetByURL nonexistent")
}

func TestArticleTagRepository_GetByContentHash(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID:   "doc1",
		DatasetID:    "ds1",
		ContentHash:  "hash123",
		IngestStatus: "ingested",
	}
	assertNoError(t, repo.Create(article), "Create")

	got, err := repo.GetByContentHash("hash123")
	assertNoError(t, err, "GetByContentHash")
	assertEqual(t, got.ContentHash, "hash123")

	_, err = repo.GetByContentHash("nothash")
	assertError(t, err, "GetByContentHash nonexistent")
}

func TestArticleTagRepository_GetByFilePath(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1",
		FilePath: "/data/articles/test.md",
	}
	assertNoError(t, repo.Create(article), "Create")

	got, err := repo.GetByFilePath("/data/articles/test.md")
	assertNoError(t, err, "GetByFilePath")
	assertEqual(t, got.FilePath, "/data/articles/test.md")

	_, err = repo.GetByFilePath("/nonexistent")
	assertError(t, err, "GetByFilePath nonexistent")
}

func TestArticleTagRepository_UpdateIndexStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID:  "doc1",
		DatasetID:   "ds1",
		IndexStatus: "pending",
	}
	assertNoError(t, repo.Create(article), "Create")
	assertNoError(t, repo.UpdateIndexStatus(article.ID, "indexed"), "UpdateIndexStatus")

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.IndexStatus, "indexed")
}

func TestArticleTagRepository_UpdateIngestStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID:   "doc1",
		DatasetID:    "ds1",
		IngestStatus: "ingested",
	}
	assertNoError(t, repo.Create(article), "Create")
	assertNoError(t, repo.UpdateIngestStatus(article.ID, "tag_association"), "UpdateIngestStatus")

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.IngestStatus, "tag_association")
}

func TestArticleTagRepository_ListPendingIndex(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	for i := 0; i < 3; i++ {
		a := &model.ArticleTag{
			DocumentID:  "doc1",
			DatasetID:   "ds1",
			IndexStatus: "pending",
		}
		assertNoError(t, repo.Create(a), "Create pending")
	}
	a := &model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", IndexStatus: "indexed"}
	assertNoError(t, repo.Create(a), "Create indexed")

	articles, err := repo.ListPendingIndex(10)
	assertNoError(t, err, "ListPendingIndex")
	assertEqual(t, len(articles), 3)
}

func TestArticleTagRepository_ListByLayer(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	a1 := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", Layer: "raw"}
	a2 := &model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", Layer: "pkb"}
	assertNoError(t, repo.Create(a1), "Create a1")
	assertNoError(t, repo.Create(a2), "Create a2")

	articles, err := repo.ListByLayer("raw", 10, 0)
	assertNoError(t, err, "ListByLayer")
	assertEqual(t, len(articles), 1)
	assertEqual(t, articles[0].Layer, "raw")
}

func TestArticleTagRepository_ListByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	a1 := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", IngestStatus: "ingested"}
	a2 := &model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", IngestStatus: "pending"}
	assertNoError(t, repo.Create(a1), "Create a1")
	assertNoError(t, repo.Create(a2), "Create a2")

	articles, err := repo.ListByStatus("ingested", 10, 0)
	assertNoError(t, err, "ListByStatus")
	assertEqual(t, len(articles), 1)
}

func TestArticleTagRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	assertNoError(t, repo.Create(&model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1"}), "Create 1")
	assertNoError(t, repo.Create(&model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1"}), "Create 2")

	count, err := repo.Count()
	assertNoError(t, err, "Count")
	assertEqual(t, count, int64(2))
}

func TestArticleTagRepository_CountByLayer(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	assertNoError(t, repo.Create(&model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", Layer: "raw"}), "Create")
	assertNoError(t, repo.Create(&model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", Layer: "pkb"}), "Create")

	count, err := repo.CountByLayer("raw")
	assertNoError(t, err, "CountByLayer")
	assertEqual(t, count, int64(1))
}

func TestArticleTagRepository_CountByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	assertNoError(t, repo.Create(&model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", IngestStatus: "ingested"}), "Create")

	count, err := repo.CountByStatus("ingested")
	assertNoError(t, err, "CountByStatus")
	assertEqual(t, count, int64(1))
}

func TestArticleTagRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1"}
	assertNoError(t, repo.Create(article), "Create")
	assertNoError(t, repo.Delete(article.ID), "Delete")

	_, err := repo.GetByID(article.ID)
	assertError(t, err, "GetByID after delete")
}

func TestArticleTagRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ArticleTitle: "old"}
	assertNoError(t, repo.Create(article), "Create")

	article.ArticleTitle = "new"
	assertNoError(t, repo.Update(article), "Update")

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.ArticleTitle, "new")
}

func TestArticleTagRepository_ListWithFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	assertNoError(t, repo.Create(&model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1", Layer: "raw",
		IngestStatus: "ingested", ArticleTitle: "Go Programming",
	}), "Create 1")
	assertNoError(t, repo.Create(&model.ArticleTag{
		DocumentID: "doc2", DatasetID: "ds1", Layer: "pkb",
		IngestStatus: "pending", ArticleTitle: "Rust Guide",
	}), "Create 2")

	articles, total, err := repo.ListWithFilter(ListArticleTagOpts{Layer: "raw", Page: 1, PerPage: 10})
	assertNoError(t, err, "ListWithFilter layer=raw")
	assertEqual(t, total, int64(1))
	assertEqual(t, len(articles), 1)

	_, total, err = repo.ListWithFilter(ListArticleTagOpts{Keyword: "Go", Page: 1, PerPage: 10})
	assertNoError(t, err, "ListWithFilter keyword=Go")
	assertEqual(t, total, int64(1))
}

func TestArticleTagRepository_MarkPkbProcessed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1"}
	assertNoError(t, repo.Create(article), "Create")
	assertNoError(t, repo.MarkPkbProcessed(article.ID, "vault", 0.85, "pkb", "/pkb/file.md"), "MarkPkbProcessed")

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.PkbStatus, "processed")
	assertEqual(t, got.PkbDecision, "vault")
	assertEqual(t, got.Layer, "pkb")
	assertEqual(t, got.FilePath, "/pkb/file.md")
}

func TestArticleTagRepository_GetByIDWithPreload(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)
	tagRepo := NewTagRepository(db)

	tag := &model.Tag{Name: "test", Color: "#111"}
	assertNoError(t, tagRepo.Create(tag), "Create tag")

	article := &model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1", TagID: tag.ID,
	}
	assertNoError(t, repo.Create(article), "Create")

	got, err := repo.GetByIDWithPreload(article.ID)
	assertNoError(t, err, "GetByIDWithPreload")
	assertEqual(t, got.Tag.ID, tag.ID)
	assertEqual(t, got.Tag.Name, "test")
}

func TestArticleTagRepository_ListWithFilterExcludeProcessed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	assertNoError(t, repo.Create(&model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1", IngestStatus: "ingested",
	}), "Create unprocessed")
	assertNoError(t, repo.Create(&model.ArticleTag{
		DocumentID: "doc2", DatasetID: "ds1", IngestStatus: "ingested",
		PkbStatus: "processed",
	}), "Create processed")

	articles, total, err := repo.ListWithFilter(ListArticleTagOpts{ExcludeProcessed: true, Page: 1, PerPage: 10})
	assertNoError(t, err, "ListWithFilter exclude processed")
	assertEqual(t, total, int64(1))
	assertEqual(t, len(articles), 1)
}

func TestArticleTagRepository_MarkPkbProcessedNoMove(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", Layer: "raw", FilePath: "/raw/f.md"}
	assertNoError(t, repo.Create(article), "Create")
	assertNoError(t, repo.MarkPkbProcessed(article.ID, "discard", 0.1, "", ""), "MarkPkbProcessed no move")

	got, err := repo.GetByID(article.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.PkbDecision, "discard")
	assertEqual(t, got.Layer, "raw")
	assertEqual(t, got.FilePath, "/raw/f.md")
}

func TestArticleTagRepository_GetByContentHashSkipsTagAssociation(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	a1 := &model.ArticleTag{DocumentID: "doc1", DatasetID: "ds1", ContentHash: "h1", IngestStatus: "tag_association"}
	a2 := &model.ArticleTag{DocumentID: "doc2", DatasetID: "ds1", ContentHash: "h1", IngestStatus: "ingested"}
	assertNoError(t, repo.Create(a1), "Create a1")
	assertNoError(t, repo.Create(a2), "Create a2")

	got, err := repo.GetByContentHash("h1")
	assertNoError(t, err, "GetByContentHash")
	assertEqual(t, got.IngestStatus, "ingested")
}

func TestArticleTagRepository_CreateWithMetadata(t *testing.T) {
	db := setupTestDB(t)
	repo := NewArticleTagRepository(db)

	article := &model.ArticleTag{
		DocumentID: "doc1", DatasetID: "ds1",
		Metadata: datatypes.JSON(`{"key":"value"}`),
	}
	assertNoError(t, repo.Create(article), "Create with metadata")
	assertEqual(t, article.ID > 0, true)
}
