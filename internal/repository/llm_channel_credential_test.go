package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMChannelCredentialRepository_Get(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelCredentialRepository(db)

	cred := &model.LLMChannelCredential{
		ChannelID:      1,
		Purpose:        "api",
		Source:         "direct",
		EnvVarName:     "OPENAI_API_KEY",
		IsPreset:       false,
		Label:          "main-key",
		Priority:       0,
		ProviderType:   "openai",
		CredentialJSON: "ciphertext-here",
		Status:         "active",
	}
	assertNoError(t, db.Create(cred).Error, "Create credential")

	got, err := repo.Get(cred.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.ChannelID, uint(1))
	assertEqual(t, got.Label, "main-key")
	assertEqual(t, got.Status, "active")
}

func TestLLMChannelCredentialRepository_ListByChannel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelCredentialRepository(db)

	assertNoError(t, db.Create(&model.LLMChannelCredential{
		ChannelID: 1, Purpose: "api", CredentialJSON: "c1", Priority: 0,
	}).Error, "Create 1")
	assertNoError(t, db.Create(&model.LLMChannelCredential{
		ChannelID: 1, Purpose: "api", CredentialJSON: "c2", Priority: 1,
	}).Error, "Create 2")
	assertNoError(t, db.Create(&model.LLMChannelCredential{
		ChannelID: 2, Purpose: "api", CredentialJSON: "c3", Priority: 0,
	}).Error, "Create 3")

	creds, err := repo.ListByChannel(1)
	assertNoError(t, err, "ListByChannel")
	assertEqual(t, len(creds), 2)
}

func TestLLMChannelCredentialRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelCredentialRepository(db)

	cred := &model.LLMChannelCredential{
		ChannelID: 1, CredentialJSON: "c1",
	}
	assertNoError(t, db.Create(cred).Error, "Create")
	assertNoError(t, repo.Delete(cred.ID), "Delete")

	_, err := repo.Get(cred.ID)
	assertError(t, err, "Get after delete")
}

func TestLLMChannelCredentialRepository_Create(t *testing.T) {
	t.Skip("depends on crypto package")
}

func TestLLMChannelCredentialRepository_Update(t *testing.T) {
	t.Skip("depends on crypto package")
}

func TestLLMChannelCredentialRepository_GetDecrypted(t *testing.T) {
	t.Skip("depends on crypto package")
}
