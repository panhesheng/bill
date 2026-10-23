package billing

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smallgo/server/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// shareTestRouter wires the real billing routes with a fixed user.
func shareTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Set("role", database.RoleUser)
		c.Next()
	})
	setupRoutes(group, db)
	return router
}

func shareRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

// TestSharedDataIsVisibleToTheDesignatedUser makes sure the owner of an
// account can let another user read the data, that the other users stay
// locked out and that the viewer never writes into the shared data.
func TestSharedDataIsVisibleToTheDesignatedUser(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "share.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	owner := database.User{Username: "owner", Password: "test", Role: database.RoleUser, Status: 1}
	viewer := database.User{Username: "viewer", Password: "test", Role: database.RoleUser, Status: 1}
	other := database.User{Username: "other", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&owner)
	db.Create(&viewer)
	db.Create(&other)
	db.Create(&PersonalTransaction{UserID: owner.ID, AccountID: 1, Kind: KindExpense, AmountCents: 4200, Category: "餐饮", Note: "owner bill", OccurredAt: time.Now()})
	db.Create(&PersonalTransaction{UserID: viewer.ID, AccountID: 1, Kind: KindExpense, AmountCents: 1000, Category: "餐饮", Note: "viewer bill", OccurredAt: time.Now()})

	ownerRouter := shareTestRouter(db, owner.ID)
	viewerRouter := shareTestRouter(db, viewer.ID)
	otherRouter := shareTestRouter(db, other.ID)

	// The owner lists its shares: nobody yet.
	recorder := shareRequest(t, ownerRouter, http.MethodGet, "/api/billing/personal/shares", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("list shares status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var shareList struct {
		Data struct {
			Viewers []shareViewer `json:"viewers"`
			Owners  []shareOwner  `json:"owners"`
		} `json:"data"`
	}
	decodeBody(t, recorder, &shareList)
	if len(shareList.Data.Viewers) != 0 || len(shareList.Data.Owners) != 0 {
		t.Fatalf("unexpected share list: %#v", shareList.Data)
	}

	// The owner shares the data with the viewer.
	recorder = shareRequest(t, ownerRouter, http.MethodPost, "/api/billing/personal/shares", `{"username":"viewer"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create share status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Data shareViewer `json:"data"`
	}
	decodeBody(t, recorder, &created)
	if created.Data.UserID != viewer.ID || created.Data.Username != "viewer" {
		t.Fatalf("created share = %#v", created.Data)
	}

	// The viewer sees the data of the owner.
	recorder = shareRequest(t, viewerRouter, http.MethodGet, "/api/billing/personal/transactions?owner_id="+itoa(owner.ID), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var list struct {
		Data struct {
			Items []PersonalTransaction `json:"items"`
		} `json:"data"`
	}
	decodeBody(t, recorder, &list)
	if len(list.Data.Items) != 1 || list.Data.Items[0].Note != "owner bill" {
		t.Fatalf("viewer sees %#v", list.Data.Items)
	}
	// The viewer still sees its own data.
	recorder = shareRequest(t, viewerRouter, http.MethodGet, "/api/billing/personal/transactions", "")
	decodeBody(t, recorder, &list)
	if len(list.Data.Items) != 1 || list.Data.Items[0].Note != "viewer bill" {
		t.Fatalf("viewer own data = %#v", list.Data.Items)
	}

	// Another user is locked out.
	recorder = shareRequest(t, otherRouter, http.MethodGet, "/api/billing/personal/transactions?owner_id="+itoa(owner.ID), "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("other list status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	recorder = shareRequest(t, otherRouter, http.MethodGet, "/api/billing/personal/stats?owner_id="+itoa(owner.ID), "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("other stats status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	// A write of the viewer never lands in the shared data.
	recorder = shareRequest(t, viewerRouter, http.MethodPost, "/api/billing/personal/accounts", `{"type":"wechat","name":"微信"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("viewer create account status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var accounts int64
	db.Model(&PersonalAccount{}).Where("user_id = ?", owner.ID).Count(&accounts)
	if accounts != 0 {
		t.Fatalf("the shared data grew, account count = %d", accounts)
	}
	db.Model(&PersonalAccount{}).Where("user_id = ?", viewer.ID).Count(&accounts)
	if accounts != 1 {
		t.Fatalf("viewer account count = %d, want 1", accounts)
	}

	// The owner revokes the share.
	recorder = shareRequest(t, ownerRouter, http.MethodDelete, "/api/billing/personal/shares/"+itoa(created.Data.ID), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete share status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	recorder = shareRequest(t, viewerRouter, http.MethodGet, "/api/billing/personal/transactions?owner_id="+itoa(owner.ID), "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("revoked list status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

// TestShareRejectsInvalidTargets makes sure the share cannot be created for
// an unknown user, for itself or twice for the same user.
func TestShareRejectsInvalidTargets(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "share-invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	owner := database.User{Username: "owner", Password: "test", Role: database.RoleUser, Status: 1}
	viewer := database.User{Username: "viewer", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&owner)
	db.Create(&viewer)
	router := shareTestRouter(db, owner.ID)

	if recorder := shareRequest(t, router, http.MethodPost, "/api/billing/personal/shares", `{"username":"nobody"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown user status = %d", recorder.Code)
	}
	if recorder := shareRequest(t, router, http.MethodPost, "/api/billing/personal/shares", `{"username":"owner"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("self share status = %d", recorder.Code)
	}
	if recorder := shareRequest(t, router, http.MethodPost, "/api/billing/personal/shares", `{"username":"viewer"}`); recorder.Code != http.StatusOK {
		t.Fatalf("create share status = %d", recorder.Code)
	}
	if recorder := shareRequest(t, router, http.MethodPost, "/api/billing/personal/shares", `{"username":"viewer"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("duplicate share status = %d", recorder.Code)
	}
}
