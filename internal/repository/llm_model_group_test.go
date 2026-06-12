package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMModelGroupRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelGroupRepository(db)

	group := &model.LLMModelGroup{
		Name:             "gpt4-group",
		Description:      "GPT-4 model group",
		Strategy:         "priority-health",
		StickyTTLSeconds: 600,
		Members: []model.LLMModelGroupMember{
			{ChannelName: "openai-main", Model: "gpt-4o", Weight: 10},
			{ChannelName: "deepseek", Model: "deepseek-chat", Weight: 5},
		},
	}
	assertNoError(t, repo.Create(group), "Create")
	assertEqual(t, group.ID > 0, true)

	got, err := repo.Get(group.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Name, "gpt4-group")
	assertEqual(t, len(got.Members), 2)
}

func TestLLMModelGroupRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelGroupRepository(db)

	assertNoError(t, repo.Create(&model.LLMModelGroup{Name: "g1", Strategy: "priority-health"}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMModelGroup{Name: "g2", Strategy: "round-robin"}), "Create 2")

	groups, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(groups), 2)
}

func TestLLMModelGroupRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelGroupRepository(db)

	group := &model.LLMModelGroup{
		Name:     "g1",
		Strategy: "priority-health",
		Members: []model.LLMModelGroupMember{
			{ChannelName: "ch1", Model: "m1", Weight: 1},
		},
	}
	assertNoError(t, repo.Create(group), "Create")

	group.Description = "updated"
	group.Members = []model.LLMModelGroupMember{
		{ChannelName: "ch2", Model: "m2", Weight: 2},
		{ChannelName: "ch3", Model: "m3", Weight: 3},
	}
	assertNoError(t, repo.Update(group), "Update")

	got, err := repo.Get(group.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Description, "updated")
	assertEqual(t, len(got.Members), 2)
	assertEqual(t, got.Members[0].ChannelName, "ch2")
}

func TestLLMModelGroupRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelGroupRepository(db)

	group := &model.LLMModelGroup{
		Name:     "g1",
		Strategy: "priority-health",
		Members: []model.LLMModelGroupMember{
			{ChannelName: "ch1", Model: "m1", Weight: 1},
		},
	}
	assertNoError(t, repo.Create(group), "Create")
	assertNoError(t, repo.Delete(group.ID), "Delete")

	_, err := repo.Get(group.ID)
	assertError(t, err, "Get after delete")
}

func TestLLMModelGroupRepository_UpdateReplacesMembers(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMModelGroupRepository(db)

	group := &model.LLMModelGroup{
		Name:     "g1",
		Strategy: "priority-health",
		Members: []model.LLMModelGroupMember{
			{ChannelName: "old-ch", Model: "old-m", Weight: 1},
		},
	}
	assertNoError(t, repo.Create(group), "Create")

	group.Members = []model.LLMModelGroupMember{
		{ChannelName: "new-ch", Model: "new-m", Weight: 5},
	}
	assertNoError(t, repo.Update(group), "Update")

	got, err := repo.Get(group.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, len(got.Members), 1)
	assertEqual(t, got.Members[0].ChannelName, "new-ch")
}
