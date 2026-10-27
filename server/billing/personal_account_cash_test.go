package billing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateCashAccount makes sure a cash account can be added and that the
// backend normalises it: it has no bank, no card kind and no card number.
func TestCreateCashAccount(t *testing.T) {
	db, userID := accountTestDB(t, "cash.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "cash", "name": "现金钱包", "institution": "现金", "identifier": "随身", "card_kind": "debit", "opening_balance_cents": 2000,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("create cash account status = %d body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &created)
	if created.Data.Type != AccountTypeCash {
		t.Fatalf("type = %q, want %q", created.Data.Type, AccountTypeCash)
	}
	if created.Data.Institution != "现金" {
		t.Fatalf("institution = %q, want 现金", created.Data.Institution)
	}
	if created.Data.CardKind != "" {
		t.Fatalf("card kind = %q, want an empty card kind", created.Data.CardKind)
	}
	if created.Data.CardFingerprint != nil {
		t.Fatalf("card fingerprint = %v, want none", created.Data.CardFingerprint)
	}
	if created.Data.Identifier != "随身" {
		t.Fatalf("identifier = %q, want the note to be kept", created.Data.Identifier)
	}
	if created.Data.OpeningBalanceCents != 2000 {
		t.Fatalf("opening balance = %d, want 2000", created.Data.OpeningBalanceCents)
	}
}

// TestCashAccountIsListed makes sure a cash account shows up in the account
// list with its balance, so the user can pick it while recording a bill.
func TestCashAccountIsListed(t *testing.T) {
	db, userID := accountTestDB(t, "cash-list.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "cash", "name": "现金钱包", "opening_balance_cents": 3000,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("create cash account status = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/accounts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list accounts status = %d body = %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Data []PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &listed)
	if len(listed.Data) != 1 {
		t.Fatalf("accounts = %d, want 1", len(listed.Data))
	}
	account := listed.Data[0]
	if account.Type != AccountTypeCash {
		t.Fatalf("type = %q, want %q", account.Type, AccountTypeCash)
	}
	if account.BalanceCents != 3000 {
		t.Fatalf("balance = %d, want 3000", account.BalanceCents)
	}
	if !account.CanDelete {
		t.Fatalf("a cash account without bills must be deletable")
	}
}

// TestUnknownAccountTypeIsRejected makes sure the account types stay a closed
// list.
func TestUnknownAccountTypeIsRejected(t *testing.T) {
	db, userID := accountTestDB(t, "cash-invalid.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "crypto", "name": "某加密钱包",
	})))
	if rec.Code == http.StatusOK {
		t.Fatalf("an unknown account type must be rejected: %s", rec.Body.String())
	}
}
