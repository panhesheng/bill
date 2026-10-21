package billing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
)

func TestRefundSourceLockedOnUpdate(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "lock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "lock", Password: "test"}
	db.Create(&user)
	ledger, _ := ensurePersonalSetup(db, user.ID)
	acc := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&acc)
	now := time.Now().Format(time.RFC3339)
	first := PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: acc.ID, Kind: KindExpense, AmountCents: 10000, Category: "购物", OccurredAt: time.Now()}
	second := PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: acc.ID, Kind: KindExpense, AmountCents: 5000, Category: "餐饮", OccurredAt: time.Now()}
	db.Create(&first)
	db.Create(&second)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", user.ID); c.Next() })
	group.POST("/personal/transactions", handleCreatePersonal(db))
	group.PUT("/personal/transactions/:id", handleUpdatePersonal(db))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": acc.ID, "kind": KindRefund,
		"amount_cents": 4000, "refund_source_id": first.ID, "occurred_at": now,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("create refund status = %d body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data PersonalTransaction `json:"data"`
	}
	decodeBody(t, rec, &created)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/billing/personal/transactions/%d", created.Data.ID), jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": acc.ID, "kind": KindRefund,
		"amount_cents": 4000, "refund_source_id": second.ID, "occurred_at": now,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("update refund status = %d body = %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Data PersonalTransaction `json:"data"`
	}
	decodeBody(t, rec, &updated)
	if updated.Data.RefundSourceID != first.ID {
		t.Fatalf("refund source changed to %d, want %d", updated.Data.RefundSourceID, first.ID)
	}
}
