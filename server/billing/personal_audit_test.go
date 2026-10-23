package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// auditTestRouter wires the billing routes with a fixed user.
func auditTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Set("username", "owner")
		c.Set("role", database.RoleUser)
		c.Next()
	})
	setupRoutes(group, db)
	return router
}

// TestEditingABillWritesAnAuditEntry makes sure an edited bill leaves a trace
// in the operation log with the fields that changed.
func TestEditingABillWritesAnAuditEntry(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "owner", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&user)
	ledger, err := ensurePersonalSetup(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&account)
	bill := PersonalTransaction{
		UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense,
		AmountCents: 1200, Category: "餐饮", Note: "午饭", OccurredAt: time.Now(),
	}
	db.Create(&bill)

	router := auditTestRouter(db, user.ID)
	body := `{"ledger_id":` + itoa(ledger.ID) + `,"account_id":` + itoa(account.ID) + `,"kind":"expense","amount_cents":4500,"category":"餐饮","note":"晚饭加咖啡","occurred_at":"` + time.Now().Format(time.RFC3339) + `"}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/billing/personal/transactions/"+itoa(bill.ID), strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	var entries []database.AuditLog
	if err := db.Where("user_id = ? AND action = ?", user.ID, "personal_transaction_update").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.TargetType != "personal_transaction" || entry.TargetID != bill.ID {
		t.Fatalf("audit target = %s/%d", entry.TargetType, entry.TargetID)
	}
	if !strings.Contains(entry.Detail, "金额 12.00→45.00") {
		t.Fatalf("audit detail = %q, want the amount change", entry.Detail)
	}
	if !strings.Contains(entry.Detail, "备注修改") {
		t.Fatalf("audit detail = %q, want the note change", entry.Detail)
	}
}

// TestPersonalWritesAreLogged makes sure the personal write handlers leave an
// entry in the operation log.
func TestPersonalWritesAreLogged(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-all.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "owner", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&user)
	ledger, err := ensurePersonalSetup(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&account)
	router := auditTestRouter(db, user.ID)

	steps := []struct {
		action string
		method string
		path   string
		body   string
	}{
		{"personal_account_create", http.MethodPost, "/api/billing/personal/accounts", `{"type":"wechat","name":"微信钱包"}`},
		{"personal_ledger_create", http.MethodPost, "/api/billing/personal/ledgers", `{"name":"旅行账本"}`},
		{"personal_category_create", http.MethodPost, "/api/billing/personal/categories", `{"kind":"expense","name":"咖啡","parent_id":0}`},
		{"personal_tag_create", http.MethodPost, "/api/billing/personal/tags", `{"name":"工作日"}`},
	}
	for _, step := range steps {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(step.method, step.path, strings.NewReader(step.body)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", step.action, recorder.Code, recorder.Body.String())
		}
		var count int64
		db.Model(&database.AuditLog{}).Where("user_id = ? AND action = ?", user.ID, step.action).Count(&count)
		if count != 1 {
			t.Fatalf("audit entries for %s = %d, want 1", step.action, count)
		}
	}

	recorder := httptest.NewRecorder()
	body := `{"ledger_id":` + itoa(ledger.ID) + `,"account_id":` + itoa(account.ID) + `,"kind":"expense","amount_cents":880,"category":"餐饮","note":"早餐","occurred_at":"` + time.Now().Format(time.RFC3339) + `"}`
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("transaction create status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var count int64
	db.Model(&database.AuditLog{}).Where("user_id = ? AND action = ?", user.ID, "personal_transaction_create").Count(&count)
	if count != 1 {
		t.Fatalf("audit entries for personal_transaction_create = %d, want 1", count)
	}
}
