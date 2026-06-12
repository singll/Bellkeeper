package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestLogAlertRuleRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogAlertRuleRepository(db)

	rule := &model.LogAlertRule{
		Name:          "test-rule",
		Condition:     datatypes.JSON(`{"module":"rss_fetch","level":"error"}`),
		NotifyChannel: "alerts",
		IsActive:      true,
	}
	assertNoError(t, repo.Create(rule), "Create")
	assertEqual(t, rule.ID > 0, true)

	got, err := repo.GetByID(rule.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.Name, "test-rule")
	assertEqual(t, got.NotifyChannel, "alerts")
}

func TestLogAlertRuleRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogAlertRuleRepository(db)

	assertNoError(t, repo.Create(&model.LogAlertRule{Name: "r1", Condition: datatypes.JSON(`{}`), NotifyChannel: "a"}), "Create 1")
	assertNoError(t, repo.Create(&model.LogAlertRule{Name: "r2", Condition: datatypes.JSON(`{}`), NotifyChannel: "b"}), "Create 2")

	rules, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(rules), 2)
}

func TestLogAlertRuleRepository_ListActive(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogAlertRuleRepository(db)

	assertNoError(t, repo.Create(&model.LogAlertRule{Name: "r1", Condition: datatypes.JSON(`{}`), IsActive: true}), "Create 1")
	assertNoError(t, repo.Create(&model.LogAlertRule{Name: "r2", Condition: datatypes.JSON(`{}`), IsActive: false}), "Create 2")

	rules, err := repo.ListActive()
	assertNoError(t, err, "ListActive")
	for _, r := range rules {
		assertEqual(t, r.IsActive, true)
	}
}

func TestLogAlertRuleRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogAlertRuleRepository(db)

	rule := &model.LogAlertRule{Name: "r1", Condition: datatypes.JSON(`{}`), NotifyChannel: "old"}
	assertNoError(t, repo.Create(rule), "Create")

	rule.NotifyChannel = "new"
	assertNoError(t, repo.Update(rule), "Update")

	got, err := repo.GetByID(rule.ID)
	assertNoError(t, err, "GetByID")
	assertEqual(t, got.NotifyChannel, "new")
}

func TestLogAlertRuleRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLogAlertRuleRepository(db)

	rule := &model.LogAlertRule{Name: "r1", Condition: datatypes.JSON(`{}`)}
	assertNoError(t, repo.Create(rule), "Create")
	assertNoError(t, repo.Delete(rule.ID), "Delete")

	_, err := repo.GetByID(rule.ID)
	assertError(t, err, "GetByID after delete")
}
