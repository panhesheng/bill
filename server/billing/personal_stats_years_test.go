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

func statsTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	group.GET("/personal/stats", handlePersonalStats(db))
	return router
}

// TestPersonalStatsReturnsAvailableYears makes sure the statistics page can
// offer the years the user recorded income or expenses in. Transfers never
// count as a year with data.
func TestPersonalStatsReturnsAvailableYears(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "stats-years.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "stats-years", Password: "test"}
	db.Create(&user)
	base := time.Date(2025, time.June, 5, 10, 0, 0, 0, time.UTC)
	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: 1, Kind: KindIncome, AmountCents: 10000, Category: "工资", OccurredAt: base},
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 2500, Category: "餐饮", OccurredAt: base.AddDate(1, 0, 0)},
		{UserID: user.ID, AccountID: 1, Kind: KindTransfer, AmountCents: 900, Category: TransferCategory, OccurredAt: base.AddDate(-1, 0, 0)},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	router := statsTestRouter(db, user.ID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/stats?start=2025-01-01&end=2025-12-31", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stats status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			AvailableYears []int `json:"available_years"`
		} `json:"data"`
	}
	decodeBody(t, recorder, &payload)
	years := payload.Data.AvailableYears
	if len(years) != 2 || years[0] != 2026 || years[1] != 2025 {
		t.Fatalf("available years = %#v", years)
	}
}
