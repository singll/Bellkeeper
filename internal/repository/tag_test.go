package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestTagRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	tag := &model.Tag{Name: "security", Color: "#F56C6C"}
	assertNoError(t, repo.Create(tag), "Create")
	assertEqual(t, tag.ID > 0, true)

	got, err := repo.GetByID(tag.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "security")
}

func TestTagRepository_GetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	assertNoError(t, repo.Create(&model.Tag{Name: "ai", Color: "#409EFF"}), "Create")

	got, err := repo.GetByName("ai")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.Name, "ai")

	_, err = repo.GetByName("nonexistent")
	assertError(t, err, "GetByName nonexistent")
}

func TestTagRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	tag := &model.Tag{Name: "old", Color: "#111"}
	assertNoError(t, repo.Create(tag), "Create")

	tag.Color = "#222"
	assertNoError(t, repo.Update(tag), "Update")

	got, err := repo.GetByID(tag.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Color, "#222")
}

func TestTagRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	tag := &model.Tag{Name: "del", Color: "#000"}
	assertNoError(t, repo.Create(tag), "Create")
	assertNoError(t, repo.Delete(tag.ID), "Delete")

	_, err := repo.GetByID(tag.ID)
	assertError(t, err, "GetByID after delete")
}

func TestTagRepository_GetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	assertNoError(t, repo.Create(&model.Tag{Name: "a", Color: "#1"}), "Create a")
	assertNoError(t, repo.Create(&model.Tag{Name: "b", Color: "#2"}), "Create b")

	tags, err := repo.GetAll()
	assertNoError(t, err, "GetAll")
	assertEqual(t, len(tags), 3)
}

func TestTagRepository_GetByIDs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	t1 := &model.Tag{Name: "x", Color: "#1"}
	t2 := &model.Tag{Name: "y", Color: "#2"}
	assertNoError(t, repo.Create(t1), "Create t1")
	assertNoError(t, repo.Create(t2), "Create t2")

	tags, err := repo.GetByIDs([]uint{t1.ID, t2.ID})
	assertNoError(t, err, "GetByIDs")
	assertEqual(t, len(tags), 2)
}

func TestTagRepository_GetByNames(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	assertNoError(t, repo.Create(&model.Tag{Name: "alpha", Color: "#1"}), "Create")
	assertNoError(t, repo.Create(&model.Tag{Name: "beta", Color: "#2"}), "Create")

	tags, err := repo.GetByNames([]string{"alpha", "beta"})
	assertNoError(t, err, "GetByNames")
	assertEqual(t, len(tags), 2)
}

func TestTagRepository_FindOrCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	tag, err := repo.FindOrCreate("newtag", "#ABC")
	assertNoError(t, err, "FindOrCreate new")
	assertEqual(t, tag.Name, "newtag")
	assertEqual(t, tag.Color, "#ABC")

	tag2, err := repo.FindOrCreate("newtag", "#DEF")
	assertNoError(t, err, "FindOrCreate existing")
	assertEqual(t, tag2.ID, tag.ID, "should return same tag")
}

func TestTagRepository_FindOrCreateRestoresSoftDeleted(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	tag := &model.Tag{Name: "recycle", Color: "#111"}
	assertNoError(t, repo.Create(tag), "Create")
	assertNoError(t, repo.Delete(tag.ID), "Delete")

	var softDeleted model.Tag
	err := db.Unscoped().Where("name = ?", "recycle").First(&softDeleted).Error
	assertNoError(t, err, "find soft-deleted")

	restored, err := repo.FindOrCreate("recycle", "#222")
	assertNoError(t, err, "FindOrCreate after soft delete")
	assertEqual(t, restored.Name, "recycle")
}

func TestTagRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	for i := 0; i < 5; i++ {
		assertNoError(t, repo.Create(&model.Tag{Name: "tag" + string(rune('A'+i)), Color: "#1"}), "Create")
	}

	tags, total, err := repo.List(1, 3, "")
	assertNoError(t, err, "List")
	assertEqual(t, len(tags), 3, "page size")
	assertEqual(t, total, int64(6), "total count")
}

func TestTagRepository_Search(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	assertNoError(t, repo.Create(&model.Tag{Name: "security", Color: "#1"}), "Create 1")
	assertNoError(t, repo.Create(&model.Tag{Name: "cyber-sec", Color: "#2"}), "Create 2")
	assertNoError(t, repo.Create(&model.Tag{Name: "ai", Color: "#3"}), "Create 3")

	tags, err := repo.Search("%sec%", 10)
	assertNoError(t, err, "Search")
	assertEqual(t, len(tags), 2)
}

func TestTagRepository_MatchByKeyword(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTagRepository(db)

	assertNoError(t, repo.Create(&model.Tag{Name: "golang", Color: "#1"}), "Create 1")
	assertNoError(t, repo.Create(&model.Tag{Name: "gophers", Color: "#2"}), "Create 2")
	assertNoError(t, repo.Create(&model.Tag{Name: "rust", Color: "#3"}), "Create 3")

	tags, err := repo.MatchByKeyword("gol")
	assertNoError(t, err, "MatchByKeyword")
	assertEqual(t, len(tags), 1)
	assertEqual(t, tags[0].Name, "golang")
}
