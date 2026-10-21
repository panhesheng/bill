package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// TestPersonalStatsResolvesCategoryParent makes sure the statistics response
// tells the client which first level category every second level category
// belongs to, so the client can aggregate and drill down.
func TestPersonalStatsResolvesCategoryParent(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "stats-categories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "stats-categories", Password: "test"}
	db.Create(&user)
	base := time.Date(2026, time.April, 8, 9, 0, 0, 0, time.UTC)
	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 3000, Category: "交通", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 2000, Category: "午餐", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 1000, Category: "早餐", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 500, Category: "餐饮", OccurredAt: base},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	router := statsTestRouter(db, user.ID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/stats", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stats status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Categories []struct {
				Category    string `json:"category"`
				Parent      string `json:"parent"`
				AmountCents int64  `json:"amount_cents"`
				Count       int64  `json:"count"`
			} `json:"categories"`
		} `json:"data"`
	}
	decodeBody(t, recorder, &payload)
	parents := map[string]string{}
	amounts := map[string]int64{}
	for _, item := range payload.Data.Categories {
		parents[item.Category] = item.Parent
		amounts[item.Category] = item.AmountCents
	}
	if parents["早餐"] != "餐饮" || parents["午餐"] != "餐饮" {
		t.Fatalf("second level parents = %#v", parents)
	}
	if parents["餐饮"] != "" || parents["交通"] != "" {
		t.Fatalf("first level parents = %#v", parents)
	}
	if amounts["午餐"] != 2000 || amounts["早餐"] != 1000 {
		t.Fatalf("category amounts = %#v", amounts)
	}
}
