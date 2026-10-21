package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func flowTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	group.GET("/personal/flow", handlePersonalFlow(db))
	return router
}

func flowRequest(t *testing.T, router *gin.Engine, query string) personalFlowResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/flow"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("flow status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data personalFlowResponse `json:"data"`
	}
	decodeBody(t, recorder, &payload)
	return payload.Data
}

func TestPersonalFlowAggregatesByYearMonthAndDay(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "flow", Password: "test"}
	db.Create(&user)
	base := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: 1, Kind: KindIncome, AmountCents: 100000, Category: "工资", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 30000, Category: "购物", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 2000, Category: "餐饮", OccurredAt: base.AddDate(0, 0, 1)},
		{UserID: user.ID, AccountID: 1, Kind: KindIncome, AmountCents: 5000, Category: "奖金", OccurredAt: base.AddDate(1, 0, 0)},
		{UserID: user.ID, AccountID: 1, Kind: KindTransfer, AmountCents: 7000, Category: TransferCategory, OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindRefund, AmountCents: 900, Category: RefundCategory, OccurredAt: base},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	router := flowTestRouter(db, user.ID)

	years := flowRequest(t, router, "")
	if years.Granularity != "year" {
		t.Fatalf("granularity = %q, want year", years.Granularity)
	}
	if len(years.Entries) != 2 {
		t.Fatalf("year entries = %#v", years.Entries)
	}
	if years.Entries[0].Key != "2027" || years.Entries[1].Key != "2026" {
		t.Fatalf("year entries are not descending: %#v", years.Entries)
	}
	if years.Entries[1].IncomeCents != 100000 || years.Entries[1].ExpenseCents != 32000 || years.Entries[1].BalanceCents != 68000 {
		t.Fatalf("2026 year entry = %#v", years.Entries[1])
	}
	if years.Entries[1].Title != "2026年" {
		t.Fatalf("year title = %q", years.Entries[1].Title)
	}
	if len(years.AvailableYears) != 2 || years.AvailableYears[0] != 2027 {
		t.Fatalf("available years = %#v", years.AvailableYears)
	}

	months := flowRequest(t, router, "?year=2026")
	if months.Granularity != "month" {
		t.Fatalf("granularity = %q, want month", months.Granularity)
	}
	if len(months.Entries) != 1 || months.Entries[0].Key != "2026-03" {
		t.Fatalf("month entries = %#v", months.Entries)
	}
	if months.Entries[0].Title != "3月" || months.Entries[0].Subtitle != "2026" {
		t.Fatalf("month labels = %#v", months.Entries[0])
	}
	if months.Entries[0].BalanceCents != 68000 {
		t.Fatalf("month balance = %d", months.Entries[0].BalanceCents)
	}

	days := flowRequest(t, router, "?year=2026&month=3")
	if days.Granularity != "day" {
		t.Fatalf("granularity = %q, want day", days.Granularity)
	}
	if len(days.Entries) != 2 {
		t.Fatalf("day entries = %#v", days.Entries)
	}
	if days.Entries[0].Key != "2026-03-11" || days.Entries[1].Key != "2026-03-10" {
		t.Fatalf("day entries are not descending: %#v", days.Entries)
	}
	if days.Entries[1].ExpenseCents != 30000 || days.Entries[1].IncomeCents != 100000 {
		t.Fatalf("first day entry = %#v", days.Entries[1])
	}
	if days.Entries[0].ExpenseCents != 2000 {
		t.Fatalf("second day entry = %#v", days.Entries[0])
	}
	if days.Entries[1].Title != "10日" || days.Entries[1].Subtitle != "3月10日" {
		t.Fatalf("day labels = %#v", days.Entries[1])
	}
}

func TestPersonalFlowIgnoresTransfersAndRefunds(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "flow-kinds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "flow-kinds", Password: "test"}
	db.Create(&user)
	now := time.Now()
	db.Create(&PersonalTransaction{UserID: user.ID, AccountID: 1, Kind: KindTransfer, AmountCents: 5000, Category: TransferCategory, OccurredAt: now})
	db.Create(&PersonalTransaction{UserID: user.ID, AccountID: 1, Kind: KindRefund, AmountCents: 3000, Category: RefundCategory, OccurredAt: now})
	router := flowTestRouter(db, user.ID)
	result := flowRequest(t, router, "")
	if len(result.Entries) != 0 {
		t.Fatalf("transfers and refunds must not create periods: %#v", result.Entries)
	}
}
