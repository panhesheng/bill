package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// recentAccountResponse mirrors the payload of the recent accounts endpoint.
type recentAccountResponse struct {
	Data struct {
		Items []struct {
			ID          uint   `json:"id"`
			Name        string `json:"name"`
			Identifier  string `json:"identifier"`
			Type        string `json:"type"`
			Count       int64  `json:"count"`
			RecentCount int64  `json:"recent_count"`
		} `json:"items"`
	} `json:"data"`
}

// TestRecentAccountsRankByUsage makes sure the quick picks list the accounts
// the user records bills with most often. A transfer counts for both of its
// accounts, archived accounts are left out.
func TestRecentAccountsRankByUsage(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "recent-accounts.db"))
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
	cash := PersonalAccount{UserID: user.ID, Type: AccountTypeCash, Name: "现金钱包", Institution: "现金"}
	card := PersonalAccount{UserID: user.ID, Type: AccountTypeBankCard, Name: "工资卡", Institution: "招商银行", Identifier: "1234"}
	archived := PersonalAccount{UserID: user.ID, Type: AccountTypeCash, Name: "旧钱包", Institution: "现金", Archived: true}
	db.Create(&cash)
	db.Create(&card)
	db.Create(&archived)

	now := time.Now()
	for i := 0; i < 3; i++ {
		db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: cash.ID, Kind: KindExpense, AmountCents: 1200, Category: "餐饮", OccurredAt: now.AddDate(0, 0, -i)})
	}
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: card.ID, CounterpartAccountID: cash.ID, Kind: KindTransfer, AmountCents: 5000, Category: "转账", OccurredAt: now})
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: archived.ID, Kind: KindExpense, AmountCents: 900, Category: "旧账", OccurredAt: now})

	router := auditTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/accounts/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent accounts status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload recentAccountResponse
	decodeBody(t, rec, &payload)
	items := payload.Data.Items
	if len(items) != 2 {
		t.Fatalf("items = %+v, want 2 accounts", items)
	}
	// The cash wallet is used three times and receives one transfer.
	if items[0].Name != "现金钱包" || items[0].Count != 4 || items[0].RecentCount != 4 {
		t.Fatalf("first = %+v, want 现金钱包 with 4 uses", items[0])
	}
	if items[1].Name != "工资卡" || items[1].Identifier != "1234" || items[1].Count != 1 {
		t.Fatalf("second = %+v, want 工资卡 with one transfer", items[1])
	}
}

// TestRecentAccountsSkipOtherUsers makes sure the quick picks never hold the
// accounts of somebody else.
func TestRecentAccountsSkipOtherUsers(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "recent-accounts-owner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	owner := database.User{Username: "owner", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&owner)
	other := database.User{Username: "other", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&other)
	ledger, err := ensurePersonalSetup(db, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := PersonalAccount{UserID: other.ID, Type: AccountTypeCash, Name: "别人的钱包", Institution: "现金"}
	db.Create(&account)
	db.Create(&PersonalTransaction{UserID: other.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: 1200, Category: "餐饮", OccurredAt: time.Now()})

	router := auditTestRouter(db, owner.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/accounts/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent accounts status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload recentAccountResponse
	decodeBody(t, rec, &payload)
	if len(payload.Data.Items) != 0 {
		t.Fatalf("items = %+v, want none", payload.Data.Items)
	}
}
