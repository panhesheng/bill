package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// TestPersonalListOffsetReadsTheScrollingList makes sure the bills can be
// read with an offset, which is how the scrolling list continues where it
// ended, even after a bill was deleted.
func TestPersonalListOffsetReadsTheScrollingList(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "personal-offset.db"))
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
	account := PersonalAccount{UserID: user.ID, Type: AccountTypeCash, Name: "现金", Institution: "现金"}
	db.Create(&account)
	now := time.Now()
	for i := 0; i < 5; i++ {
		db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: int64(100 + i), Category: "餐饮", OccurredAt: now.AddDate(0, 0, -i)})
	}

	router := auditTestRouter(db, user.ID)
	type listResponse struct {
		Data struct {
			Items []PersonalTransaction `json:"items"`
			Total int64                 `json:"total"`
		} `json:"data"`
	}
	read := func(query string) listResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/transactions"+query, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("list status = %d body = %s", rec.Code, rec.Body.String())
		}
		var payload listResponse
		decodeBody(t, rec, &payload)
		return payload
	}

	// The first two bills, the page size decides how many come back.
	first := read("?pageSize=2")
	if len(first.Data.Items) != 2 || first.Data.Total != 5 {
		t.Fatalf("first page = %+v, want 2 of 5 bills", first.Data)
	}
	// The offset continues where the list ended.
	second := read("?offset=2&pageSize=2")
	if len(second.Data.Items) != 2 || second.Data.Items[0].ID == first.Data.Items[1].ID {
		t.Fatalf("second page = %+v, want the bills after the first page", second.Data.Items)
	}
	third := read("?offset=4&pageSize=2")
	if len(third.Data.Items) != 1 {
		t.Fatalf("third page = %+v, want the last bill", third.Data.Items)
	}
	// An offset behind the end of the list is empty.
	if empty := read("?offset=9&pageSize=2"); len(empty.Data.Items) != 0 {
		t.Fatalf("offset behind the end = %+v, want none", empty.Data.Items)
	}
}
