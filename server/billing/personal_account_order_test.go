package billing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// accountIDsInOrder returns the ids of the accounts in the order the list
// endpoint returns them.
func accountIDsInOrder(t *testing.T, router http.Handler, path string) []uint {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list accounts status = %d body = %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Data []PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &listed)
	ids := make([]uint, 0, len(listed.Data))
	for _, account := range listed.Data {
		ids = append(ids, account.ID)
	}
	return ids
}

// TestAccountsKeepTheOrderTheUserDraggedThemInto makes sure the account list
// follows the order the user saved.
func TestAccountsKeepTheOrderTheUserDraggedThemInto(t *testing.T) {
	db, userID := accountTestDB(t, "order.db")
	router := accountRouter(db, userID)
	names := []string{"工资卡", "日常微信", "现金钱包"}
	for _, name := range names {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
			"type": "wechat", "name": name,
		})))
		if rec.Code != http.StatusOK {
			t.Fatalf("create account %s status = %d body = %s", name, rec.Code, rec.Body.String())
		}
	}
	ids := accountIDsInOrder(t, router, "/api/billing/personal/accounts")
	if len(ids) != 3 {
		t.Fatalf("accounts = %v, want 3", ids)
	}
	if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("accounts = %v, want the creation order", ids)
	}

	reversed := []uint{ids[2], ids[1], ids[0]}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/billing/personal/accounts/order", jsonBody(t, map[string]interface{}{
		"account_ids": reversed,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("order status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := accountIDsInOrder(t, router, "/api/billing/personal/accounts"); got[0] != reversed[0] || got[1] != reversed[1] || got[2] != reversed[2] {
		t.Fatalf("accounts = %v, want %v", got, reversed)
	}
}

// TestAccountOrderRejectsInvalidIds makes sure the order endpoint only
// accepts the accounts of the user, and only once each.
func TestAccountOrderRejectsInvalidIds(t *testing.T) {
	db, userID := accountTestDB(t, "order-invalid.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "wechat", "name": "日常微信",
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("create account status = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/billing/personal/accounts/order", jsonBody(t, map[string]interface{}{
		"account_ids": []uint{1, 9999},
	})))
	if rec.Code == http.StatusOK {
		t.Fatalf("an order with a foreign account must be rejected: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/billing/personal/accounts/order", jsonBody(t, map[string]interface{}{
		"account_ids": []uint{1, 1},
	})))
	if rec.Code == http.StatusOK {
		t.Fatalf("an order with a duplicated account must be rejected: %s", rec.Body.String())
	}
}
