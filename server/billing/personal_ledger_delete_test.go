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

func ledgerTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	group.GET("/personal/ledgers", handleListPersonalLedgers(db))
	group.DELETE("/personal/ledgers/:id/permanent", handleDeletePersonalLedger(db))
	return router
}

func TestDeletePersonalLedgerWithoutBills(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "ledger-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "ledger-delete", Password: "test"}
	db.Create(&user)
	ledger := PersonalLedger{UserID: user.ID, Name: "旅行账本", Currency: "CNY", Cover: "coral"}
	db.Create(&ledger)
	router := ledgerTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/billing/personal/ledgers/"+itoa(ledger.ID)+"/permanent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete ledger status = %d body = %s", rec.Code, rec.Body.String())
	}
	var check PersonalLedger
	if err := db.First(&check, ledger.ID).Error; err == nil {
		t.Fatal("ledger was not deleted")
	}
}

func TestDeletePersonalLedgerBlockedWithBills(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "ledger-delete-blocked.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "ledger-blocked", Password: "test"}
	db.Create(&user)
	ledger := PersonalLedger{UserID: user.ID, Name: "旅行账本", Currency: "CNY", Cover: "coral"}
	db.Create(&ledger)
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: ledger.ID, AccountID: 1, Kind: KindExpense, AmountCents: 100, Category: "餐饮", OccurredAt: time.Now()})
	router := ledgerTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/billing/personal/ledgers/"+itoa(ledger.ID)+"/permanent", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("deleting a ledger with bills must fail: %s", rec.Body.String())
	}
	var check PersonalLedger
	if err := db.First(&check, ledger.ID).Error; err != nil {
		t.Fatalf("ledger was deleted anyway: %v", err)
	}
}

func TestDeleteDefaultPersonalLedgerBlocked(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "ledger-delete-default.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "ledger-default", Password: "test"}
	db.Create(&user)
	ledger, err := ensurePersonalSetup(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := ledgerTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/billing/personal/ledgers/"+itoa(ledger.ID)+"/permanent", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("deleting the default ledger must fail: %s", rec.Body.String())
	}
}

func TestListPersonalLedgersMarksDeletable(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "ledger-can-delete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "ledger-can-delete", Password: "test"}
	db.Create(&user)
	empty := PersonalLedger{UserID: user.ID, Name: "空账本", Currency: "CNY", Cover: "coral"}
	used := PersonalLedger{UserID: user.ID, Name: "有账单", Currency: "CNY", Cover: "coral"}
	db.Create(&empty)
	db.Create(&used)
	db.Create(&PersonalTransaction{UserID: user.ID, LedgerID: used.ID, AccountID: 1, Kind: KindExpense, AmountCents: 100, Category: "餐饮", OccurredAt: time.Now()})
	router := ledgerTestRouter(db, user.ID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/ledgers?include_archived=true", nil))
	var payload struct {
		Data []PersonalLedger `json:"data"`
	}
	decodeBody(t, rec, &payload)
	for _, item := range payload.Data {
		switch item.Name {
		case "空账本":
			if !item.CanDelete {
				t.Fatal("a ledger without bills must be deletable")
			}
		case "有账单":
			if item.CanDelete {
				t.Fatal("a ledger with bills must not be deletable")
			}
		}
	}
}
