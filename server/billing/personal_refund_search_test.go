package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
)

func TestRefundableListLimitAndKeyword(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "search", Password: "test"}
	db.Create(&user)
	now := time.Now()
	for i := 0; i < 40; i++ {
		category, note := "餐饮", "午餐"
		if i%2 == 0 {
			category, note = "购物", "超市采购"
		}
		item := PersonalTransaction{
			UserID: user.ID, AccountID: 1, Kind: KindExpense, AmountCents: int64(1000 + i),
			Category: category, Note: note, OccurredAt: now.Add(-time.Duration(i) * time.Hour),
		}
		db.Create(&item)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/billing")
	group.Use(func(c *gin.Context) { c.Set("userID", user.ID); c.Next() })
	group.GET("/personal/refundable-transactions", handleListRefundableTransactions(db))

	list := func(query string) []refundableTransaction {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/billing/personal/refundable-transactions"+query, nil))
		var payload struct {
			Data []refundableTransaction `json:"data"`
		}
		decodeBody(t, rec, &payload)
		return payload.Data
	}
	if items := list(""); len(items) != 30 {
		t.Fatalf("default list returned %d items, want 30", len(items))
	}
	if items := list("?limit=5"); len(items) != 5 {
		t.Fatalf("limit=5 returned %d items, want 5", len(items))
	}
	if items := list("?limit=500"); len(items) != 40 {
		t.Fatalf("limit above the cap returned %d items, want 40", len(items))
	}
	searched := list("?keyword=超市")
	if len(searched) != 20 {
		t.Fatalf("keyword search returned %d items, want 20", len(searched))
	}
	for _, item := range searched {
		if item.Category != "购物" {
			t.Fatalf("keyword search returned %q", item.Category)
		}
	}
	if items := list("?keyword=不存在的分类"); len(items) != 0 {
		t.Fatalf("unmatched keyword returned %d items, want 0", len(items))
	}
}
