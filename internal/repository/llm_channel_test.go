package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMChannelRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	ch := &model.LLMChannel{
		Name:         "openai-main",
		BaseURL:      "https://api.openai.com/v1",
		APIKeyEnv:    "OPENAI_API_KEY",
		ProviderType: "openai",
		RPM:          500,
		RPD:          50000,
		Priority:     1,
		IsFree:       false,
		IsEnabled:    true,
		Models:       `["gpt-4o","gpt-4o-mini"]`,
		Tier:         "standard",
	}
	assertNoError(t, repo.Create(ch), "Create")
	assertEqual(t, ch.ID > 0, true)

	got, err := repo.Get(ch.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Name, "openai-main")
	assertEqual(t, got.ProviderType, "openai")
}

func TestLLMChannelRepository_GetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	ch := &model.LLMChannel{Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", ProviderType: "deepseek"}
	assertNoError(t, repo.Create(ch), "Create")

	got, err := repo.GetByName("deepseek")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.Name, "deepseek")

	_, err = repo.GetByName("nonexistent")
	assertError(t, err, "GetByName nonexistent")
}

func TestLLMChannelRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	assertNoError(t, repo.Create(&model.LLMChannel{Name: "ch1", BaseURL: "https://a.com", Priority: 1}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMChannel{Name: "ch2", BaseURL: "https://b.com", Priority: 2}), "Create 2")

	channels, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(channels), 2)
}

func TestLLMChannelRepository_ListEnabled(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	assertNoError(t, repo.Create(&model.LLMChannel{Name: "ch1", BaseURL: "https://a.com", IsEnabled: true}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMChannel{Name: "ch2", BaseURL: "https://b.com", IsEnabled: false}), "Create 2")

	channels, err := repo.ListEnabled()
	assertNoError(t, err, "ListEnabled")
	for _, ch := range channels {
		assertEqual(t, ch.IsEnabled, true)
	}
}

func TestLLMChannelRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	ch := &model.LLMChannel{Name: "ch1", BaseURL: "https://a.com", RPM: 100}
	assertNoError(t, repo.Create(ch), "Create")

	ch.RPM = 200
	assertNoError(t, repo.Update(ch), "Update")

	got, err := repo.Get(ch.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.RPM, 200)
}

func TestLLMChannelRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelRepository(db)

	ch := &model.LLMChannel{Name: "ch1", BaseURL: "https://a.com"}
	assertNoError(t, repo.Create(ch), "Create")
	assertNoError(t, repo.Delete(ch.ID), "Delete")

	_, err := repo.Get(ch.ID)
	assertError(t, err, "Get after delete")
}
