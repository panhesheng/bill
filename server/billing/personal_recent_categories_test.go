package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// recentCategoryResponse mirrors the payload of the recent categories
// endpoint.
type recentCategoryResponse struct {
	Data struct {
		Items []struct {
			Name        string `json:"name"`
			Kind        string `json:"kind"`
			Count       int64  `json:"count"`
			RecentCount int64  `json:"recent_count"`
		} `json:"items"`
	} `json:"data"`
}

// TestRecentCategoriesRankByUsage makes sure the quick picks list the
// categories the user records most often, and that the categories used long
// ago come last.
func TestRecentCategoriesRankByUsage(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "recent.db"))
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
	old := now.AddDate(0, -6, 0)
	// 餐饮 is the most used category of the last months, 购物 was used a lot
	// but a long time ago.
	for i := 0; i < 3; i++ {
		db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: 1200, Category: "餐饮", OccurredAt: now.AddDate(0, 0, -i)})
	}
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: 800, Category: "交通", OccurredAt: now.AddDate(0, 0, -10)})
	for i := 0; i < 5; i++ {
		db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: 500, Category: "购物", OccurredAt: old.AddDate(0, 0, -i)})
	}
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindIncome, AmountCents: 9000, Category: "工资", OccurredAt: now})

	router := auditTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/categories/recent?kind=expense", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent categories status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload recentCategoryResponse
	decodeBody(t, rec, &payload)
	items := payload.Data.Items
	if len(items) != 3 {
		t.Fatalf("items = %+v, want 3 categories", items)
	}
	if items[0].Name != "餐饮" || items[0].Count != 3 || items[0].RecentCount != 3 {
		t.Fatalf("first = %+v, want 餐饮 with 3 uses", items[0])
	}
	if items[1].Name != "交通" || items[1].RecentCount != 1 {
		t.Fatalf("second = %+v, want 交通 with 1 recent use", items[1])
	}
	if items[2].Name != "购物" || items[2].Count != 5 || items[2].RecentCount != 0 {
		t.Fatalf("third = %+v, want 购物 with 5 uses and none of them recent", items[2])
	}

	// The kind filter keeps the other kinds out.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/categories/recent?kind=income", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent categories status = %d body = %s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &payload)
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].Name != "工资" {
		t.Fatalf("income items = %+v, want only 工资", payload.Data.Items)
	}
}

// TestRecentCategoriesSkipOtherUsers makes sure the quick picks never hold
// the categories of somebody else.
func TestRecentCategoriesSkipOtherUsers(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "recent-owner.db"))
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
	account := PersonalAccount{UserID: other.ID, Type: AccountTypeCash, Name: "现金", Institution: "现金"}
	db.Create(&account)
	db.Create(&PersonalTransaction{UserID: other.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense, AmountCents: 1200, Category: "别人的分类", OccurredAt: time.Now()})

	router := auditTestRouter(db, owner.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/categories/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent categories status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload recentCategoryResponse
	decodeBody(t, rec, &payload)
	if len(payload.Data.Items) != 0 {
		t.Fatalf("items = %+v, want none", payload.Data.Items)
	}
}
