package billing

import (
	"errors"
	"strings"
	"time"

	"smallgo/server/response"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// refundableTransaction describes an expense that can still be refunded.
// RefundableCents is what is left after the previous refunds of the same bill.
type refundableTransaction struct {
	ID              uint      `json:"id"`
	Category        string    `json:"category"`
	Note            string    `json:"note"`
	AmountCents     int64     `json:"amount_cents"`
	OccurredAt      time.Time `json:"occurred_at"`
	AccountID       uint      `json:"account_id"`
	AccountName     string    `json:"account_name"`
	RefundedCents   int64     `json:"refunded_cents"`
	RefundableCents int64     `json:"refundable_cents"`
}

// refundedCentsOf sums the refunds already recorded for the given source bill.
// excludeID drops one refund from the sum so that editing a refund does not
// count its own amount against itself.
func refundedCentsOf(db *gorm.DB, userID, sourceID, excludeID uint) (int64, error) {
	query := db.Model(&PersonalTransaction{}).
		Where("user_id = ? AND kind = ? AND refund_source_id = ?", userID, KindRefund, sourceID)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var total int64
	err := query.Select("COALESCE(SUM(amount_cents), 0)").Scan(&total).Error
	return total, err
}

// refundSource loads the expense a refund refers to and verifies ownership.
func refundSource(db *gorm.DB, userID, sourceID uint) (PersonalTransaction, error) {
	if sourceID == 0 {
		return PersonalTransaction{}, errors.New("请选择原支出的账单")
	}
	var source PersonalTransaction
	if err := db.Where("id = ? AND user_id = ?", sourceID, userID).First(&source).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return source, errors.New("原支出的账单不存在")
		}
		return source, err
	}
	if source.Kind != KindExpense {
		return source, errors.New("只能对支出账单发起退款")
	}
	return source, nil
}

// validateRefundAmount makes sure the refund does not exceed what is left of
// the original bill and returns the remaining refundable amount.
func validateRefundAmount(db *gorm.DB, userID, sourceID uint, amountCents int64, excludeID uint) (int64, error) {
	source, err := refundSource(db, userID, sourceID)
	if err != nil {
		return 0, err
	}
	refunded, err := refundedCentsOf(db, userID, sourceID, excludeID)
	if err != nil {
		return 0, err
	}
	refundable := source.AmountCents - refunded
	if refundable <= 0 {
		return 0, errors.New("该笔支出的金额已全部退回")
	}
	if amountCents > refundable {
		return 0, errors.New("退款金额不能超过剩余可退金额")
	}
	return refundable, nil
}

// defaultRefundableLimit is how many refund sources are returned when the
// client does not ask for a specific amount.
const defaultRefundableLimit = 30

// maxRefundableLimit caps how many refund sources a single request may load.
const maxRefundableLimit = 100

// handleListRefundableTransactions lists the user's expenses that still have
// money left to refund, so the client can offer them as refund sources.
// The list is limited to the most recent bills and can be narrowed by keyword.
func handleListRefundableTransactions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")
		if _, err := ensurePersonalSetup(db, userID); err != nil {
			response.ErrorInternal(c, "获取可退款账单失败")
			return
		}
		limit := utils.Atoi(c.Query("limit"), defaultRefundableLimit)
		if limit <= 0 {
			limit = defaultRefundableLimit
		}
		if limit > maxRefundableLimit {
			limit = maxRefundableLimit
		}
		conditions := []string{"t.user_id = ?", "t.kind = ?", "t.refunded_cents < t.amount_cents"}
		args := []interface{}{userID, KindExpense}
		if value := uint(utils.Atoi(c.Query("ledger_id"), 0)); value > 0 {
			conditions = append(conditions, "t.ledger_id = ?")
			args = append(args, value)
		}
		if keyword := cleanText(c.Query("keyword"), 64); keyword != "" {
			conditions = append(conditions, "(t.category LIKE ? OR t.note LIKE ?)")
			like := "%" + keyword + "%"
			args = append(args, like, like)
		}
		args = append(args, limit)
		// The refunded amount is aggregated in a sub query so the limit only
		// counts bills that still have money left to refund.
		sql := `SELECT t.id, t.category, t.note, t.amount_cents, t.occurred_at, t.account_id, t.refunded_cents
			FROM (
				SELECT p.*, COALESCE((
					SELECT SUM(r.amount_cents) FROM personal_transactions r
					WHERE r.user_id = p.user_id AND r.kind = ? AND r.refund_source_id = p.id
				), 0) AS refunded_cents
				FROM personal_transactions p
				WHERE p.user_id = ? AND p.kind = ?
			) t
			WHERE ` + strings.Join(conditions, " AND ") + `
			ORDER BY t.occurred_at DESC, t.id DESC
			LIMIT ?`
		// The sub query binds the refund kind plus the owner and kind of the
		// source bills, the outer query binds the filter conditions.
		queryArgs := append([]interface{}{KindRefund, userID, KindExpense}, args...)
		var sources []refundableTransaction
		if err := db.Raw(sql, queryArgs...).Scan(&sources).Error; err != nil {
			response.ErrorInternal(c, "获取可退款账单失败")
			return
		}
		items := make([]refundableTransaction, 0, len(sources))
		for _, source := range sources {
			source.RefundableCents = source.AmountCents - source.RefundedCents
			if source.AccountID > 0 {
				var account PersonalAccount
				if err := db.Select("id, name").First(&account, source.AccountID).Error; err == nil {
					source.AccountName = account.Name
				}
			}
			items = append(items, source)
		}
		response.Success(c, items)
	}
}

// loadRefundSourceInfo fills the display fields of a refund with information
// about the bill it refunds. The source may have been deleted, in which case
// the refund simply shows without the original bill.
func loadRefundSourceInfo(db *gorm.DB, item *PersonalTransaction) {
	if item.Kind != KindRefund || item.RefundSourceID == 0 {
		return
	}
	var source PersonalTransaction
	if err := db.Select("id, category, amount_cents, occurred_at").First(&source, item.RefundSourceID).Error; err != nil {
		return
	}
	item.RefundSourceCategory = source.Category
	item.RefundSourceAmount = source.AmountCents
	item.RefundSourceDate = source.OccurredAt.Format(time.RFC3339)
}
