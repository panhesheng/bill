package billing

import (
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"gorm.io/gorm"
)

func refundTestDB(t *testing.T, name string) (*gorm.DB, uint) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.CloseDB(db) })
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "refund-" + name, Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ensurePersonalSetup(db, user.ID); err != nil {
		t.Fatal(err)
	}
	return db, user.ID
}

func TestValidatePersonalRequestRefund(t *testing.T) {
	item, err := validatePersonalRequest(personalRequest{
		AccountID:      11,
		RefundSourceID: 22,
		Kind:           KindRefund,
		AmountCents:    1000,
		Category:       "餐饮",
		OccurredAt:     "2026-01-02T15:04",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != KindRefund || item.RefundSourceID != 22 {
		t.Fatalf("unexpected refund = %#v", item)
	}
	if item.Category != RefundCategory {
		t.Fatalf("refund category = %q, want %q", item.Category, RefundCategory)
	}
	if _, err := validatePersonalRequest(personalRequest{
		AccountID:   11,
		Kind:        KindRefund,
		AmountCents: 1000,
		OccurredAt:  "2026-01-02T15:04",
	}); err == nil {
		t.Fatal("refund without a source bill must be rejected")
	}
}

func TestValidateRefundAmountLimitsPartialRefunds(t *testing.T) {
	db, userID := refundTestDB(t, "refund-amount.db")
	source := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 10000,
		Category: "购物", OccurredAt: time.Now(),
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	refundable, err := validateRefundAmount(db, userID, source.ID, 4000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if refundable != 10000 {
		t.Fatalf("refundable = %d, want 10000", refundable)
	}
	partial := PersonalTransaction{
		UserID: userID, AccountID: 1, RefundSourceID: source.ID, Kind: KindRefund,
		AmountCents: 4000, Category: RefundCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&partial).Error; err != nil {
		t.Fatal(err)
	}
	refundable, err = validateRefundAmount(db, userID, source.ID, 6000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if refundable != 6000 {
		t.Fatalf("remaining refundable = %d, want 6000", refundable)
	}
	if _, err := validateRefundAmount(db, userID, source.ID, 7000, 0); err == nil {
		t.Fatal("refund above the remaining amount must be rejected")
	}
	full := PersonalTransaction{
		UserID: userID, AccountID: 1, RefundSourceID: source.ID, Kind: KindRefund,
		AmountCents: 6000, Category: RefundCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&full).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := validateRefundAmount(db, userID, source.ID, 1, 0); err == nil {
		t.Fatal("refunding a fully refunded bill must be rejected")
	}
}

func TestValidateRefundAmountIgnoresEditedRefundItself(t *testing.T) {
	db, userID := refundTestDB(t, "refund-edit.db")
	source := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 10000,
		Category: "购物", OccurredAt: time.Now(),
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	refund := PersonalTransaction{
		UserID: userID, AccountID: 1, RefundSourceID: source.ID, Kind: KindRefund,
		AmountCents: 10000, Category: RefundCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&refund).Error; err != nil {
		t.Fatal(err)
	}
	// Editing the refund must not count its own amount against itself.
	refundable, err := validateRefundAmount(db, userID, source.ID, 10000, refund.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refundable != 10000 {
		t.Fatalf("refundable = %d, want 10000", refundable)
	}
}

func TestRefundSourceMustBeAnOwnedExpense(t *testing.T) {
	db, userID := refundTestDB(t, "refund-source.db")
	income := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindIncome, AmountCents: 1000,
		Category: "工资", OccurredAt: time.Now(),
	}
	if err := db.Create(&income).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := validateRefundAmount(db, userID, income.ID, 100, 0); err == nil {
		t.Fatal("only expenses can be refunded")
	}
	if _, err := validateRefundAmount(db, userID, 999999, 100, 0); err == nil {
		t.Fatal("unknown source bill must be rejected")
	}
}

// TestPersonalStatsIgnoreRefundAmounts makes sure a refund is not treated as
// income: the original expense already left the account, so counting the
// refund as an inflow would inflate the balance.
func TestPersonalStatsIgnoreRefundAmounts(t *testing.T) {
	db, userID := refundTestDB(t, "refund-stats.db")
	now := time.Now()
	rows := []PersonalTransaction{
		{UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 10000, Category: "购物", OccurredAt: now},
		{UserID: userID, AccountID: 1, RefundSourceID: 1, Kind: KindRefund, AmountCents: 2500, Category: RefundCategory, OccurredAt: now},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	summary, categories, trend, err := personalStats(db, userID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.IncomeCents != 0 {
		t.Fatalf("income = %d, want 0 (refunds are not income)", summary.IncomeCents)
	}
	if summary.ExpenseCents != 10000 {
		t.Fatalf("expense = %d, want 10000", summary.ExpenseCents)
	}
	if summary.BalanceCents != -10000 {
		t.Fatalf("balance = %d, want -10000", summary.BalanceCents)
	}
	if summary.Count != 2 {
		t.Fatalf("count = %d, want 2", summary.Count)
	}
	if len(categories) != 1 || categories[0].Category != "购物" {
		t.Fatalf("categories = %#v", categories)
	}
	current := trend[len(trend)-1]
	if current.IncomeCents != 0 || current.ExpenseCents != 10000 {
		t.Fatalf("current month trend = %#v", current)
	}
}

func TestRefundedCentsOfTracksPartialRefunds(t *testing.T) {
	db, userID := refundTestDB(t, "refund-list.db")
	now := time.Now()
	full := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 1000,
		Category: "餐饮", OccurredAt: now,
	}
	partial := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 2000,
		Category: "购物", OccurredAt: now,
	}
	for _, item := range []*PersonalTransaction{&full, &partial} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	refund := PersonalTransaction{
		UserID: userID, AccountID: 1, RefundSourceID: full.ID, Kind: KindRefund,
		AmountCents: 1000, Category: RefundCategory, OccurredAt: now,
	}
	if err := db.Create(&refund).Error; err != nil {
		t.Fatal(err)
	}
	refunded, err := refundedCentsOf(db, userID, full.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if refunded != 1000 {
		t.Fatalf("refunded = %d, want 1000", refunded)
	}
	untouched, err := refundedCentsOf(db, userID, partial.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if untouched != 0 {
		t.Fatalf("untouched = %d, want 0", untouched)
	}
}

func TestRefundSourceIsLockedWhenEditing(t *testing.T) {
	db, userID := refundTestDB(t, "refund-lock.db")
	now := time.Now()
	first := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 10000,
		Category: "购物", OccurredAt: now,
	}
	second := PersonalTransaction{
		UserID: userID, AccountID: 1, Kind: KindExpense, AmountCents: 5000,
		Category: "餐饮", OccurredAt: now,
	}
	for _, item := range []*PersonalTransaction{&first, &second} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	refund := PersonalTransaction{
		UserID: userID, AccountID: 1, RefundSourceID: first.ID, Kind: KindRefund,
		AmountCents: 4000, Category: RefundCategory, OccurredAt: now,
	}
	if err := db.Create(&refund).Error; err != nil {
		t.Fatal(err)
	}
	// The client asks to move the refund to another bill: the source must stay.
	item, err := validatePersonalRequest(personalRequest{
		AccountID: 1, RefundSourceID: second.ID, Kind: KindRefund,
		AmountCents: 4000, OccurredAt: now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.RefundSourceID != second.ID {
		t.Fatalf("refund source = %d, want %d", item.RefundSourceID, second.ID)
	}
}

func TestRefundColumnMigratesForLegacyTables(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "refund-upgrade.db"))
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
		t.Fatalf("auto migrate legacy refund schema: %v", err)
	}
	refund := PersonalTransaction{
		UserID: 7, AccountID: 1, RefundSourceID: 5, Kind: KindRefund,
		AmountCents: 100, Category: RefundCategory, OccurredAt: time.Now(),
	}
	if err := db.Create(&refund).Error; err != nil {
		t.Fatal(err)
	}
	var stored PersonalTransaction
	if err := db.First(&stored, refund.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RefundSourceID != 5 {
		t.Fatalf("refund_source_id = %d, want 5", stored.RefundSourceID)
	}
}
