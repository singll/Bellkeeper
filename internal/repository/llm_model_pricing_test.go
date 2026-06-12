package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMModelPricingRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	p := &model.LLMModelPricing{
		ChannelName:           "openai-main",
		Model:                 "gpt-4o",
		InputPricePer1MCents:  250,
		OutputPricePer1MCents: 1000,
		Currency:              "USD",
		EffectiveFrom:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	assertNoError(t, repo.Create(p), "Create")
	assertEqual(t, p.ID > 0, true)

	got, err := repo.Get(p.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.ChannelName, "openai-main")
	assertEqual(t, got.Model, "gpt-4o")
	assertEqual(t, got.InputPricePer1MCents, 250)
}

func TestLLMModelPricingRepository_GetByChannelAndModel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	p := &model.LLMModelPricing{
		ChannelName:   "openai-main",
		Model:         "gpt-4o",
		Currency:      "USD",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	assertNoError(t, repo.Create(p), "Create")

	got, err := repo.GetByChannelAndModel("openai-main", "gpt-4o")
	assertNoError(t, err, "GetByChannelAndModel")
	assertEqual(t, got.Model, "gpt-4o")

	got, err = repo.GetByChannelAndModel("nonexistent", "no-model")
	assertNoError(t, err, "GetByChannelAndModel not found")
	assertEqual(t, got, nil)
}

func TestLLMModelPricingRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	assertNoError(t, repo.Create(&model.LLMModelPricing{ChannelName: "ch-a", Model: "m1", EffectiveFrom: time.Now()}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMModelPricing{ChannelName: "ch-b", Model: "m2", EffectiveFrom: time.Now()}), "Create 2")

	pricings, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(pricings), 2)
}

func TestLLMModelPricingRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	p := &model.LLMModelPricing{
		ChannelName:           "ch1",
		Model:                 "m1",
		InputPricePer1MCents:  100,
		EffectiveFrom:         time.Now(),
	}
	assertNoError(t, repo.Create(p), "Create")

	p.InputPricePer1MCents = 200
	assertNoError(t, repo.Update(p), "Update")

	got, err := repo.Get(p.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.InputPricePer1MCents, 200)
}

func TestLLMModelPricingRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	p := &model.LLMModelPricing{ChannelName: "ch1", Model: "m1", EffectiveFrom: time.Now()}
	assertNoError(t, repo.Create(p), "Create")
	assertNoError(t, repo.Delete(p.ID), "Delete")

	_, err := repo.Get(p.ID)
	assertError(t, err, "Get after delete")
}

func TestLLMModelPricingRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelPricingRepository(db)

	assertNoError(t, repo.Create(&model.LLMModelPricing{ChannelName: "ch1", Model: "m1", EffectiveFrom: time.Now()}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMModelPricing{ChannelName: "ch2", Model: "m2", EffectiveFrom: time.Now()}), "Create 2")

	count, err := repo.Count()
	assertNoError(t, err, "Count")
	assertEqual(t, count, int64(2))
}
