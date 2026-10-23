package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// TestPersonalStatsSummarisesMonthsOfTheSelectedYear makes sure the trend
// chart receives every month of the requested year, even the empty ones, and
// that records of other years are left out.
func TestPersonalStatsSummarisesMonthsOfTheSelectedYear(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "stats-monthly.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "stats-monthly", Password: "test"}
	db.Create(&user)
	base := time.Date(2026, time.January, 15, 9, 0, 0, 0, time.UTC)
	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: 1, Kind: KindIncome, AmountCents: 10000, Category: "工资", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 2500, Category: "餐饮", OccurredAt: base.AddDate(0, 2, 0)},
		{UserID: user.ID, AccountID: 1, Kind: KindIncome, AmountCents: 7000, Category: "奖金", OccurredAt: base.AddDate(-1, 0, 0)},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	router := statsTestRouter(db, user.ID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/stats?trend_year=2026", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stats status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Monthly []struct {
				Month        string `json:"month"`
				IncomeCents  int64  `json:"income_cents"`
				ExpenseCents int64  `json:"expense_cents"`
				BalanceCents int64  `json:"balance_cents"`
				Count        int64  `json:"count"`
			} `json:"monthly"`
		} `json:"data"`
	}
	decodeBody(t, recorder, &payload)
	monthly := payload.Data.Monthly
	if len(monthly) != 12 {
		t.Fatalf("monthly entries = %d, want 12", len(monthly))
	}
	if monthly[0].Month != "2026-01" || monthly[11].Month != "2026-12" {
		t.Fatalf("monthly range = %#v", monthly)
	}
	if monthly[0].IncomeCents != 10000 || monthly[0].Count != 1 || monthly[0].BalanceCents != 10000 {
		t.Fatalf("january entry = %#v", monthly[0])
	}
	if monthly[2].Month != "2026-03" || monthly[2].ExpenseCents != 2500 || monthly[2].Count != 1 {
		t.Fatalf("march entry = %#v", monthly[2])
	}
	if monthly[1].Count != 0 || monthly[1].IncomeCents != 0 || monthly[1].ExpenseCents != 0 {
		t.Fatalf("february entry = %#v", monthly[1])
	}
	total := int64(0)
	for _, item := range monthly {
		total += item.IncomeCents
	}
	if total != 10000 {
		t.Fatalf("income of the year = %d, want 10000", total)
	}
}
