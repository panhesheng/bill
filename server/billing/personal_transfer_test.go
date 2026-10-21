package billing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
)

func TestValidatePersonalRequestTransfer(t *testing.T) {
	item, err := validatePersonalRequest(personalRequest{
		LedgerID:             3,
		AccountID:            11,
		CounterpartAccountID: 12,
		Kind:                 KindTransfer,
		AmountCents:          1000,
		Category:             "餐饮",
		OccurredAt:           "2026-01-02T15:04",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != KindTransfer || item.CounterpartAccountID != 12 {
		t.Fatalf("unexpected transfer = %#v", item)
	}
	if item.Category != TransferCategory {
		t.Fatalf("transfer category = %q, want %q", item.Category, TransferCategory)
	}
	if item.AccountID != 11 {
		t.Fatalf("transfer account = %d, want 11", item.AccountID)
	}
}

func TestValidatePersonalRequestRejectsInvalidTransfers(t *testing.T) {
	if _, err := validatePersonalRequest(personalRequest{
		AccountID:   11,
		Kind:        KindTransfer,
		AmountCents: 1000,
		OccurredAt:  "2026-01-02T15:04",
	}); err == nil {
		t.Fatal("transfer without a counterpart account must be rejected")
	}
	if _, err := validatePersonalRequest(personalRequest{
		AccountID:            11,
		CounterpartAccountID: 11,
		Kind:                 KindTransfer,
		AmountCents:          1000,
		OccurredAt:           "2026-01-02T15:04",
	}); err == nil {
		t.Fatal("transfer between the same account must be rejected")
	}
	if _, err := validatePersonalRequest(personalRequest{
		AccountID:   11,
		Kind:        KindTransfer,
		AmountCents: 1000,
		OccurredAt:  "2026-01-02T15:04",
	}); err == nil {
		t.Fatal("transfer without an amount must be rejected")
	}
}

func TestValidatePersonalRequestKeepsCounterpartOnlyForTransfers(t *testing.T) {
	item, err := validatePersonalRequest(personalRequest{
		AccountID:            11,
		CounterpartAccountID: 12,
		Kind:                 KindExpense,
		AmountCents:          1000,
		Category:             "餐饮",
		OccurredAt:           "2026-01-02T15:04",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.CounterpartAccountID != 0 {
		t.Fatalf("expense counterpart account = %d, want 0", item.CounterpartAccountID)
	}
	if item.Category != "餐饮" {
		t.Fatalf("expense category = %q, want 餐饮", item.Category)
	}
}

func TestPersonalStatsIgnoreTransferAmounts(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "billing-transfer-stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "transfer-stats", Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	ledger, err := ensurePersonalSetup(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []PersonalTransaction{
		{UserID: user.ID, LedgerID: ledger.ID, AccountID: 1, Kind: KindIncome, AmountCents: 5000, Category: "工资", OccurredAt: now},
		{UserID: user.ID, LedgerID: ledger.ID, AccountID: 1, Kind: KindExpense, AmountCents: 2000, Category: "餐饮", OccurredAt: now},
		{UserID: user.ID, LedgerID: ledger.ID, AccountID: 1, CounterpartAccountID: 2, Kind: KindTransfer, AmountCents: 1000, Category: TransferCategory, OccurredAt: now},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	summary, categories, trend, err := personalStats(db, user.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.IncomeCents != 5000 {
		t.Fatalf("income = %d, want 5000", summary.IncomeCents)
	}
	if summary.ExpenseCents != 2000 {
		t.Fatalf("expense = %d, want 2000", summary.ExpenseCents)
	}
	if summary.BalanceCents != 3000 {
		t.Fatalf("balance = %d, want 3000", summary.BalanceCents)
	}
	if summary.Count != 3 {
		t.Fatalf("count = %d, want 3", summary.Count)
	}
	if len(categories) != 1 || categories[0].Category != "餐饮" || categories[0].AmountCents != 2000 {
		t.Fatalf("categories = %#v", categories)
	}
	if len(trend) != 6 {
		t.Fatalf("trend = %#v", trend)
	}
	current := trend[len(trend)-1]
	if current.IncomeCents != 5000 || current.ExpenseCents != 2000 {
		t.Fatalf("current month trend = %#v", current)
	}
}

func TestPersonalQueryFiltersTransferKind(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "billing-transfer-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "transfer-filter", Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: 1000, Category: "餐饮", OccurredAt: now},
		{UserID: user.ID, AccountID: 1, CounterpartAccountID: 2, Kind: KindTransfer, AmountCents: 500, Category: TransferCategory, OccurredAt: now},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?kind=transfer", nil)
	var items []PersonalTransaction
	if err := personalQuery(db, user.ID, ctx).Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != KindTransfer {
		t.Fatalf("filtered items = %#v", items)
	}
}

func TestPersonalQueryMatchesBothTransferAccounts(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "billing-transfer-account-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "transfer-account-filter", Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "转出账户", Institution: "微信"}).Error; err != nil {
		t.Fatal(err)
	}
	target := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "转入账户", Institution: "微信"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	transfer := PersonalTransaction{
		UserID: user.ID, AccountID: 1, CounterpartAccountID: target.ID,
		Kind: KindTransfer, AmountCents: 500, Category: TransferCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&transfer).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	for _, accountID := range []int{1, int(target.ID)} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?account_id=%d", accountID), nil)
		var items []PersonalTransaction
		if err := personalQuery(db, user.ID, ctx).Find(&items).Error; err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != transfer.ID {
			t.Fatalf("account %d filter returned %#v", accountID, items)
		}
	}
}

func TestTransferColumnMigratesForLegacyTables(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "billing-transfer-upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := db.Exec(`CREATE TABLE personal_transactions (
		id integer PRIMARY KEY AUTOINCREMENT,
		user_id integer NOT NULL,
		kind text NOT NULL,
		amount_cents integer NOT NULL,
		category text NOT NULL,
		note text,
		occurred_at datetime NOT NULL,
		created_at datetime,
		updated_at datetime
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate legacy transfer schema: %v", err)
	}
	transfer := PersonalTransaction{
		UserID: 7, AccountID: 1, CounterpartAccountID: 2,
		Kind: KindTransfer, AmountCents: 100, Category: TransferCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&transfer).Error; err != nil {
		t.Fatal(err)
	}
	var stored PersonalTransaction
	if err := db.First(&stored, transfer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CounterpartAccountID != 2 {
		t.Fatalf("counterpart_account_id = %d, want 2", stored.CounterpartAccountID)
	}
}
