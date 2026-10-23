package billing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

// statsTag is the shape of one entry of the tags array of the statistics
// response.
type statsTag struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	AmountCents  int64  `json:"amount_cents"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
	Count        int64  `json:"count"`
	IncomeCount  int64  `json:"income_count"`
	ExpenseCount int64  `json:"expense_count"`
}

// TestTagStatsAreSplitByKind makes sure the statistics endpoint reports the
// income and the expense side of every tag separately, so the tag ranking can
// show the tags of one direction at a time.
func TestTagStatsAreSplitByKind(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "stats.db"))
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
	account := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&account)

	food := PersonalTag{UserID: user.ID, Name: "餐饮"}
	salary := PersonalTag{UserID: user.ID, Name: "工资"}
	db.Create(&food)
	db.Create(&salary)

	now := time.Now()
	// One expense carries both tags, so the shared tag proves that the two
	// directions do not bleed into each other.
	expense := PersonalTransaction{
		UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindExpense,
		AmountCents: 1200, Category: "餐饮", Note: "午饭", OccurredAt: now,
	}
	db.Create(&expense)
	income := PersonalTransaction{
		UserID: user.ID, LedgerID: ledger.ID, AccountID: account.ID, Kind: KindIncome,
		AmountCents: 50000, Category: "工资", Note: "月薪", OccurredAt: now,
	}
	db.Create(&income)
	db.Create(&PersonalTransactionTag{TransactionID: expense.ID, TagID: food.ID})
	db.Create(&PersonalTransactionTag{TransactionID: expense.ID, TagID: salary.ID})
	db.Create(&PersonalTransactionTag{TransactionID: income.ID, TagID: salary.ID})

	router := auditTestRouter(db, user.ID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/billing/personal/stats", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stats status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Tags []statsTag `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != 0 {
		t.Fatalf("stats code = %d, body = %s", payload.Code, recorder.Body.String())
	}
	byName := map[string]statsTag{}
	for _, tag := range payload.Data.Tags {
		byName[tag.Name] = tag
	}
	foodStat, ok := byName["餐饮"]
	if !ok {
		t.Fatalf("tags = %s, want the 餐饮 tag", recorder.Body.String())
	}
	if foodStat.ExpenseCents != 1200 || foodStat.ExpenseCount != 1 {
		t.Fatalf("餐饮 expense = %d/%d, want 1200/1", foodStat.ExpenseCents, foodStat.ExpenseCount)
	}
	if foodStat.IncomeCents != 0 || foodStat.IncomeCount != 0 {
		t.Fatalf("餐饮 income = %d/%d, want 0/0", foodStat.IncomeCents, foodStat.IncomeCount)
	}
	salaryStat, ok := byName["工资"]
	if !ok {
		t.Fatalf("tags = %s, want the 工资 tag", recorder.Body.String())
	}
	if salaryStat.IncomeCents != 50000 || salaryStat.IncomeCount != 1 {
		t.Fatalf("工资 income = %d/%d, want 50000/1", salaryStat.IncomeCents, salaryStat.IncomeCount)
	}
	if salaryStat.ExpenseCents != 1200 || salaryStat.ExpenseCount != 1 {
		t.Fatalf("工资 expense = %d/%d, want 1200/1", salaryStat.ExpenseCents, salaryStat.ExpenseCount)
	}
	if salaryStat.AmountCents != salaryStat.IncomeCents+salaryStat.ExpenseCents {
		t.Fatalf("工资 amount = %d, want the sum of both directions", salaryStat.AmountCents)
	}
}
