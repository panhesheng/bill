package user

import (
	"path/filepath"
	"testing"

	"smallgo/server/database"
)

// TestUpdateUserRoleWhitelist makes sure only the known roles are accepted.
func TestUpdateUserRoleWhitelist(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "role.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB(db)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	admin := database.User{Username: "admin", Password: "test", Role: database.RoleAdmin, Status: 1}
	db.Create(&admin)
	target := database.User{Username: "member", Password: "test", Role: database.RoleUser, Status: 1}
	db.Create(&target)

	normal := database.RoleUser
	if err := UpdateUser(db, target.ID, &normal, nil); err != nil {
		t.Fatalf("set the user role failed: %v", err)
	}
	var updated database.User
	db.First(&updated, target.ID)
	if updated.Role != database.RoleUser {
		t.Fatalf("role = %q, want %q", updated.Role, database.RoleUser)
	}

	unknown := "root"
	if err := UpdateUser(db, target.ID, &unknown, nil); err == nil {
		t.Fatal("expected an error for an unknown role")
	}

	privileged := database.RoleAdmin
	if err := UpdateUser(db, target.ID, &privileged, nil); err != nil {
		t.Fatalf("set the admin role failed: %v", err)
	}
	db.First(&updated, target.ID)
	if updated.Role != database.RoleAdmin {
		t.Fatalf("role = %q, want %q", updated.Role, database.RoleAdmin)
	}
}
