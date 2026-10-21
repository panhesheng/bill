package billing

import (
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"
)

func TestPersonalAccountBalances(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "balance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "balance", Password: "test"}
	db.Create(&user)
	now := time.Now()
	bank := PersonalAccount{UserID: user.ID, Type: AccountTypeBankCard, Name: "工资卡", Institution: "中国银行", CardKind: CardKindDebit}
	wallet := PersonalAccount{UserID: user.ID, Type: AccountTypeWechat, Name: "微信", Institution: "微信"}
	db.Create(&bank)
	db.Create(&wallet)

	rows := []PersonalTransaction{
		{UserID: user.ID, AccountID: bank.ID, Kind: KindIncome, AmountCents: 100000, Category: "工资", OccurredAt: now},
		{UserID: user.ID, AccountID: bank.ID, Kind: KindExpense, AmountCents: 25000, Category: "购物", OccurredAt: now},
		{UserID: user.ID, AccountID: bank.ID, CounterpartAccountID: wallet.ID, Kind: KindTransfer, AmountCents: 10000, Category: TransferCategory, OccurredAt: now},
		{UserID: user.ID, AccountID: wallet.ID, Kind: KindExpense, AmountCents: 3000, Category: "餐饮", OccurredAt: now},
	}
	for i := range rows {
		db.Create(&rows[i])
	}
	refund := PersonalTransaction{
		UserID: user.ID, AccountID: wallet.ID, RefundSourceID: rows[3].ID, Kind: KindRefund,
		AmountCents: 1000, Category: RefundCategory, OccurredAt: now,
	}
	db.Create(&refund)

	balances, err := personalAccountBalances(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	// bank: +100000 income -25000 expense -10000 outgoing transfer = 65000
	if balances[bank.ID] != 65000 {
		t.Fatalf("bank balance = %d, want 65000", balances[bank.ID])
	}
	// wallet: +10000 incoming transfer -3000 expense +1000 refund = 8000
	if balances[wallet.ID] != 8000 {
		t.Fatalf("wallet balance = %d, want 8000", balances[wallet.ID])
	}
}

func TestPersonalAccountBalancesWithoutTransactions(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "balance-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "balance-empty", Password: "test"}
	db.Create(&user)
	balances, err := personalAccountBalances(db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 0 {
		t.Fatalf("balances = %#v, want empty", balances)
	}
}
