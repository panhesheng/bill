package billing

import (
	"errors"
	"strings"

	"smallgo/server/audit"
	"smallgo/server/database"
	"smallgo/server/response"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ErrSharedDataForbidden is returned when a user asks for data that was not
// shared with it.
var ErrSharedDataForbidden = errors.New("无权查看该用户的数据")

type shareViewer struct {
	ID        uint   `json:"id"`
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type shareOwner struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
}

type personalShareRequest struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
}

// canViewSharedData reports whether the viewer may read the data of the owner.
func canViewSharedData(db *gorm.DB, ownerID, viewerID uint) (bool, error) {
	if ownerID == 0 || viewerID == 0 || ownerID == viewerID {
		return false, nil
	}
	var count int64
	if err := db.Model(&DataShare{}).Where("owner_id = ? AND viewer_id = ?", ownerID, viewerID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// resolveDataOwner returns the user whose personal data the request reads. It
// is the caller by default. Another user is only accepted when that user
// shared their data with the caller.
func resolveDataOwner(db *gorm.DB, c *gin.Context) (uint, error) {
	caller := c.GetUint("userID")
	ownerID := uint(utils.Atoi(c.Query("owner_id"), 0))
	if ownerID == 0 || ownerID == caller {
		return caller, nil
	}
	allowed, err := canViewSharedData(db, ownerID, caller)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrSharedDataForbidden
	}
	return ownerID, nil
}

// dataOwner resolves the owner of the data and reports the error to the
// client. It is used by every read only endpoint of the personal data.
func dataOwner(db *gorm.DB, c *gin.Context) (uint, bool) {
	ownerID, err := resolveDataOwner(db, c)
	if err != nil {
		if errors.Is(err, ErrSharedDataForbidden) {
			response.ErrorForbidden(c, ErrSharedDataForbidden.Error())
		} else {
			response.ErrorInternal(c, "获取数据失败")
		}
		return 0, false
	}
	return ownerID, true
}

// handleListPersonalShares returns the users the caller shares the data with
// and the users that share their data with the caller.
func handleListPersonalShares(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")
		var shares []DataShare
		if err := db.Where("owner_id = ?", userID).Order("created_at ASC, id ASC").Find(&shares).Error; err != nil {
			response.ErrorInternal(c, "获取共享列表失败")
			return
		}
		viewers := make([]shareViewer, 0, len(shares))
		usernames := make(map[uint]string, len(shares))
		ids := make([]uint, 0, len(shares))
		for _, share := range shares {
			ids = append(ids, share.ViewerID)
		}
		if len(ids) > 0 {
			var users []database.User
			if err := db.Model(&database.User{}).Where("id IN ?", ids).Find(&users).Error; err != nil {
				response.ErrorInternal(c, "获取共享列表失败")
				return
			}
			for _, user := range users {
				usernames[user.ID] = user.Username
			}
		}
		for _, share := range shares {
			viewers = append(viewers, shareViewer{ID: share.ID, UserID: share.ViewerID, Username: usernames[share.ViewerID], CreatedAt: share.CreatedAt.Format("2006-01-02 15:04:05")})
		}
		var incoming []DataShare
		if err := db.Where("viewer_id = ?", userID).Order("created_at ASC, id ASC").Find(&incoming).Error; err != nil {
			response.ErrorInternal(c, "获取共享列表失败")
			return
		}
		owners := make([]shareOwner, 0, len(incoming))
		if len(incoming) > 0 {
			ownerIDs := make([]uint, 0, len(incoming))
			for _, share := range incoming {
				ownerIDs = append(ownerIDs, share.OwnerID)
			}
			var users []database.User
			if err := db.Model(&database.User{}).Where("id IN ?", ownerIDs).Find(&users).Error; err != nil {
				response.ErrorInternal(c, "获取共享列表失败")
				return
			}
			for _, user := range users {
				owners = append(owners, shareOwner{UserID: user.ID, Username: user.Username})
			}
		}
		response.Success(c, gin.H{"viewers": viewers, "owners": owners})
	}
}

// handleCreatePersonalShare lets the caller share its data with another user.
func handleCreatePersonalShare(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")
		var req personalShareRequest
		if c.ShouldBindJSON(&req) != nil {
			response.ErrorBadRequest(c, "请选择要共享的用户")
			return
		}
		target := req.UserID
		if target == 0 {
			username := strings.TrimSpace(req.Username)
			if username == "" {
				response.ErrorBadRequest(c, "请输入用户名")
				return
			}
			var user database.User
			if err := db.Where("username = ?", username).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					response.ErrorBadRequest(c, "用户不存在")
					return
				}
				response.ErrorInternal(c, "共享失败")
				return
			}
			target = user.ID
		}
		if target == userID {
			response.ErrorBadRequest(c, "不能共享给自己")
			return
		}
		var exists int64
		if err := db.Model(&DataShare{}).Where("owner_id = ? AND viewer_id = ?", userID, target).Count(&exists).Error; err != nil {
			response.ErrorInternal(c, "共享失败")
			return
		}
		if exists > 0 {
			response.ErrorBadRequest(c, "已经共享给该用户")
			return
		}
		var targetUser database.User
		if err := db.Where("id = ? AND status = ?", target, 1).First(&targetUser).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				response.ErrorBadRequest(c, "用户不存在或已被禁用")
				return
			}
			response.ErrorInternal(c, "共享失败")
			return
		}
		share := DataShare{OwnerID: userID, ViewerID: target}
		if err := db.Create(&share).Error; err != nil {
			response.ErrorInternal(c, "共享失败")
			return
		}
		audit.Log(db, c, "personal_share_create", "data_share", share.ID, "共享数据给 "+targetUser.Username)
		response.Success(c, shareViewer{ID: share.ID, UserID: target, Username: targetUser.Username, CreatedAt: share.CreatedAt.Format("2006-01-02 15:04:05")})
	}
}

// handleDeletePersonalShare removes a share the caller owns.
func handleDeletePersonalShare(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uint(utils.Atoi(c.Param("id"), 0))
		if id == 0 {
			response.ErrorBadRequest(c, "无效的共享记录")
			return
		}
		result := db.Where("id = ? AND owner_id = ?", id, c.GetUint("userID")).Delete(&DataShare{})
		if result.Error != nil {
			response.ErrorInternal(c, "取消共享失败")
			return
		}
		if result.RowsAffected == 0 {
			response.ErrorBadRequest(c, "共享记录不存在")
			return
		}
		audit.Log(db, c, "personal_share_delete", "data_share", id, "取消数据共享")
		response.Success(c, nil)
	}
}
