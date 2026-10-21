package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func accountTestDB(t *testing.T, name string) (*gorm.DB, uint) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.CloseDB(db) })
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "account-" + name, Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return db, user.ID
}

func accountRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	group.GET("/personal/accounts", handleListPersonalAccounts(db))
	group.POST("/personal/accounts", handleCreatePersonalAccount(db))
	group.PUT("/personal/accounts/:id", handleUpdatePersonalAccount(db))
	group.DELETE("/personal/accounts/:id/permanent", handleDeletePersonalAccount(db))
	return router
}

func TestCreateAccountStoresOpeningBalance(t *testing.T) {
	db, userID := accountTestDB(t, "opening.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "wechat", "name": "日常微信", "institution": "微信", "opening_balance_cents": 12345,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("create account status = %d body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &created)
	if created.Data.OpeningBalanceCents != 12345 {
		t.Fatalf("opening balance = %d, want 12345", created.Data.OpeningBalanceCents)
	}
	if created.Data.BalanceCents != 12345 {
		t.Fatalf("balance = %d, want 12345", created.Data.BalanceCents)
	}
}

func TestUpdateAccountKeepsOpeningBalance(t *testing.T) {
	db, userID := accountTestDB(t, "opening-keep.db")
	router := accountRouter(db, userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/billing/personal/accounts", jsonBody(t, map[string]interface{}{
		"type": "wechat", "name": "日常微信", "institution": "微信", "opening_balance_cents": 5000,
	})))
	var created struct {
		Data PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &created)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/billing/personal/accounts/"+itoa(created.Data.ID), jsonBody(t, map[string]interface{}{
		"type": "wechat", "name": "改名后的微信", "institution": "微信", "opening_balance_cents": 999999,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("update account status = %d body = %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Data PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &updated)
	if updated.Data.OpeningBalanceCents != 5000 {
		t.Fatalf("opening balance = %d, want 5000 (it must not change)", updated.Data.OpeningBalanceCents)
	}
}

func TestDeleteAccountBlockedWhenBillsExist(t *testing.T) {
	db, userID := accountTestDB(t, "delete-blocked.db")
	router := accountRouter(db, userID)
	account := PersonalAccount{UserID: userID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&account)
	db.Create(&PersonalTransaction{UserID: userID, AccountID: account.ID, Kind: KindExpense, AmountCents: 1000, Category: "餐饮", OccurredAt: time.Now()})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/billing/personal/accounts/"+itoa(account.ID)+"/permanent", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("deleting an account with bills must fail: %s", rec.Body.String())
	}
	var check PersonalAccount
	if err := db.First(&check, account.ID).Error; err != nil {
		t.Fatalf("account was deleted anyway: %v", err)
	}
}

func TestDeleteAccountWithoutBills(t *testing.T) {
	db, userID := accountTestDB(t, "delete-ok.db")
	router := accountRouter(db, userID)
	account := PersonalAccount{UserID: userID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&account)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/billing/personal/accounts/"+itoa(account.ID)+"/permanent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete account status = %d body = %s", rec.Code, rec.Body.String())
	}
	var check PersonalAccount
	if err := db.First(&check, account.ID).Error; err == nil {
		t.Fatal("account was not deleted")
	}
}

func itoa(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func TestListAccountsMarksDeletable(t *testing.T) {
	db, userID := accountTestDB(t, "can-delete.db")
	router := accountRouter(db, userID)
	free := PersonalAccount{UserID: userID, Type: AccountTypeWechat, Name: "空账户", Institution: "微信"}
	used := PersonalAccount{UserID: userID, Type: AccountTypeAlipay, Name: "有账单", Institution: "支付宝"}
	db.Create(&free)
	db.Create(&used)
	db.Create(&PersonalTransaction{UserID: userID, AccountID: used.ID, Kind: KindExpense, AmountCents: 100, Category: "餐饮", OccurredAt: time.Now()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/accounts", nil))
	var payload struct {
		Data []PersonalAccount `json:"data"`
	}
	decodeBody(t, rec, &payload)
	for _, item := range payload.Data {
		switch item.Name {
		case "空账户":
			if !item.CanDelete {
				t.Fatal("an account without bills must be deletable")
			}
		case "有账单":
			if item.CanDelete {
				t.Fatal("an account with bills must not be deletable")
			}
		}
	}
}
