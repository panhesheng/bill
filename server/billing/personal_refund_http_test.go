package billing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
)

func jsonBody(t *testing.T, body map[string]interface{}) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
}

// TestRefundHTTPFlow walks the whole refund flow through the HTTP layer:
// create an expense, list refundable bills, refund it partially, then try to
// refund more than what is left.
func TestRefundHTTPFlow(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "refund-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "refund-http", Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	ledger, err := ensurePersonalSetup(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", user.ID); c.Next() })
	group.GET("/personal/refundable-transactions", handleListRefundableTransactions(db))
	group.POST("/personal/transactions", handleCreatePersonal(db))

	now := time.Now().Format(time.RFC3339)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": account.ID, "kind": KindExpense,
		"amount_cents": 10000, "category": "购物", "occurred_at": now,
	})))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create expense status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Data PersonalTransaction `json:"data"`
	}
	decodeBody(t, recorder, &created)
	if created.Data.ID == 0 {
		t.Fatalf("expense was not created: %s", recorder.Body.String())
	}

	var listed struct {
		Data []refundableTransaction `json:"data"`
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/refundable-transactions", nil))
	decodeBody(t, recorder, &listed)
	if len(listed.Data) != 1 || listed.Data[0].RefundableCents != 10000 {
		t.Fatalf("refundable list = %#v", listed.Data)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": account.ID, "kind": KindRefund,
		"amount_cents": 4000, "refund_source_id": created.Data.ID, "occurred_at": now,
	})))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create refund status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/refundable-transactions", nil))
	decodeBody(t, recorder, &listed)
	if len(listed.Data) != 1 || listed.Data[0].RefundableCents != 6000 {
		t.Fatalf("remaining refundable = %#v", listed.Data)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": account.ID, "kind": KindRefund,
		"amount_cents": 7000, "refund_source_id": created.Data.ID, "occurred_at": now,
	})))
	if recorder.Code == http.StatusOK {
		t.Fatalf("refunding above the remaining amount must fail: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/billing/personal/transactions", jsonBody(t, map[string]interface{}{
		"ledger_id": ledger.ID, "account_id": account.ID, "kind": KindRefund,
		"amount_cents": 1000, "occurred_at": now,
	})))
	if recorder.Code == http.StatusOK {
		t.Fatalf("refund without a source bill must fail: %s", recorder.Body.String())
	}
}
