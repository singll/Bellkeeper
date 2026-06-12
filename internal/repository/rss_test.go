package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestRSSRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	feed := &model.RSSFeed{Name: "testfeed", URL: "https://example.com/rss", Category: "tech"}
	assertNoError(t, repo.Create(feed), "Create")
	assertEqual(t, feed.ID > 0, true)

	got, err := repo.GetByID(feed.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "testfeed")
	assertEqual(t, got.URL, "https://example.com/rss")
}

func TestRSSRepository_GetByURL(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	feed := &model.RSSFeed{Name: "feed", URL: "https://a.com/rss"}
	assertNoError(t, repo.Create(feed), "Create")

	got, err := repo.GetByURL("https://a.com/rss")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Name, "feed")

	_, err = repo.GetByURL("https://nonexistent.com/rss")
	assertError(t, err, "GetByURL nonexistent")
}

func TestRSSRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	feed := &model.RSSFeed{Name: "old", URL: "https://old.com/rss"}
	assertNoError(t, repo.Create(feed), "Create")

	feed.Name = "new"
	assertNoError(t, repo.Update(feed), "Update")

	got, err := repo.GetByID(feed.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "new")
}

func TestRSSRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	feed := &model.RSSFeed{Name: "del", URL: "https://del.com/rss"}
	assertNoError(t, repo.Create(feed), "Create")
	assertNoError(t, repo.Delete(feed.ID), "Delete")

	_, err := repo.GetByID(feed.ID)
	assertError(t, err, "GetByID after delete")
}

func TestRSSRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	for i := 0; i < 5; i++ {
		feed := &model.RSSFeed{Name: "feed" + string(rune('A'+i)), URL: "https://f" + string(rune('A'+i)) + ".com/rss"}
		assertNoError(t, repo.Create(feed), "Create")
	}

	feeds, total, err := repo.List(1, 3, "", "", nil)
	assertNoError(t, err, "List")
	assertEqual(t, len(feeds), 3, "page size")
	assertEqual(t, total, int64(5), "total count")
}

func TestRSSRepository_ListByCategory(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", Category: "tech"}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", Category: "news"}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")

	feeds, total, err := repo.List(1, 10, "tech", "", nil)
	assertNoError(t, err, "List tech")
	assertEqual(t, total, int64(1))
	assertEqual(t, len(feeds), 1)
}

func TestRSSRepository_ListByActive(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", IsActive: true}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", IsActive: true}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")
	db.Model(f2).Update("is_active", false)

	active := true
	_, total, err := repo.List(1, 10, "", "", &active)
	assertNoError(t, err, "List active")
	assertEqual(t, total, int64(1))
}

func TestRSSRepository_GetActive(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", IsActive: true, IsPaused: false}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", IsActive: true}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")
	db.Model(f2).Update("is_active", false)

	feeds, err := repo.GetActive()
	assertNoError(t, err, "GetActive")
	assertEqual(t, len(feeds), 1)
}

func TestRSSRepository_GetPaused(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", IsActive: true, IsPaused: true}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", IsActive: true, IsPaused: false}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")

	feeds, err := repo.GetPaused()
	assertNoError(t, err, "GetPaused")
	assertEqual(t, len(feeds), 1)
}

func TestRSSRepository_Counts(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", IsActive: true, IsPaused: false}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", IsActive: true, IsPaused: true}
	f3 := &model.RSSFeed{Name: "f3", URL: "https://f3.com/rss", IsActive: true}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")
	assertNoError(t, repo.Create(f3), "Create f3")
	db.Model(f3).Update("is_active", false)

	counts, err := repo.Counts()
	assertNoError(t, err, "Counts")
	assertEqual(t, counts.Total, int64(3))
	assertEqual(t, counts.Active, int64(1))
	assertEqual(t, counts.Paused, int64(1))
}

func TestRSSRepository_BatchUpdatePaused(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	f1 := &model.RSSFeed{Name: "f1", URL: "https://f1.com/rss", IsActive: true}
	f2 := &model.RSSFeed{Name: "f2", URL: "https://f2.com/rss", IsActive: true}
	assertNoError(t, repo.Create(f1), "Create f1")
	assertNoError(t, repo.Create(f2), "Create f2")

	affected, err := repo.BatchUpdatePaused([]uint{f1.ID, f2.ID}, true)
	assertNoError(t, err, "BatchUpdatePaused")
	assertEqual(t, affected, int64(2))

	paused, err := repo.GetPaused()
	assertNoError(t, err, "GetPaused")
	assertEqual(t, len(paused), 2)
}

func TestRSSRepository_BatchUpdatePausedEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	affected, err := repo.BatchUpdatePaused([]uint{}, true)
	assertNoError(t, err, "BatchUpdatePaused empty")
	assertEqual(t, affected, int64(0))
}

func TestRSSRepository_UpdateTags(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)
	tagRepo := NewTagRepository(db)

	feed := &model.RSSFeed{Name: "feed", URL: "https://f.com/rss"}
	assertNoError(t, repo.Create(feed), "Create feed")

	t1 := &model.Tag{Name: "t1", Color: "#1"}
	t2 := &model.Tag{Name: "t2", Color: "#2"}
	assertNoError(t, tagRepo.Create(t1), "Create t1")
	assertNoError(t, tagRepo.Create(t2), "Create t2")

	assertNoError(t, repo.UpdateTags(feed, []model.Tag{*t1, *t2}), "UpdateTags")

	got, err := repo.GetByID(feed.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, len(got.Tags), 2)
}

func TestRSSRepository_Search(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRSSRepository(db)

	assertNoError(t, repo.Create(&model.RSSFeed{Name: "TechCrunch", URL: "https://techcrunch.com/rss"}), "Create 1")
	assertNoError(t, repo.Create(&model.RSSFeed{Name: "HackerNews", URL: "https://news.ycombinator.com/rss"}), "Create 2")
	assertNoError(t, repo.Create(&model.RSSFeed{Name: "AI Weekly", URL: "https://aiweekly.com/rss"}), "Create 3")

	feeds, err := repo.Search("%tech%", 10)
	assertNoError(t, err, "Search")
	assertEqual(t, len(feeds), 1)
	assertEqual(t, feeds[0].Name, "TechCrunch")
}