package billing

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"smallgo/server/audit"
	"smallgo/server/response"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type personalRequest struct {
	LedgerID             uint                   `json:"ledger_id"`
	AccountID            uint                   `json:"account_id"`
	CounterpartAccountID uint                   `json:"counterpart_account_id"`
	RefundSourceID       uint                   `json:"refund_source_id"`
	TagIDs               []uint                 `json:"tag_ids"`
	Kind                 string                 `json:"kind"`
	AmountCents          int64                  `json:"amount_cents"`
	Category             string                 `json:"category"`
	Note                 string                 `json:"note"`
	OccurredAt           string                 `json:"occurred_at"`
	Attachments          *[]billAttachmentInput `json:"attachments"`
}

type statsSummary struct {
	IncomeCents  int64 `json:"income_cents"`
	ExpenseCents int64 `json:"expense_cents"`
	BalanceCents int64 `json:"balance_cents"`
	Count        int64 `json:"count"`
}

type categoryStat struct {
	Category string `json:"category"`
	// Parent is the first level category of the second level category the
	// record was booked on. It is empty for records booked on a first level
	// category directly.
	Parent      string `json:"parent"`
	AmountCents int64  `json:"amount_cents"`
	Count       int64  `json:"count"`
}

type monthlyStat struct {
	Month        string `json:"month"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
}

func personalQuery(db *gorm.DB, userID uint, c *gin.Context) *gorm.DB {
	query := db.Model(&PersonalTransaction{}).Where("user_id = ?", userID)
	if value := utils.Atoi(c.Query("ledger_id"), 0); value > 0 {
		query = query.Where("ledger_id = ?", value)
	}
	if value := utils.Atoi(c.Query("account_id"), 0); value > 0 {
		query = query.Where("(account_id = ? OR counterpart_account_id = ?)", value, value)
	}
	if value := utils.Atoi(c.Query("tag_id"), 0); value > 0 {
		query = query.Where("EXISTS (SELECT 1 FROM personal_transaction_tags ptt WHERE ptt.transaction_id = personal_transactions.id AND ptt.tag_id = ?)", value)
	}
	if value := c.Query("kind"); validTransactionKind(value) {
		query = query.Where("kind = ?", value)
	}
	if value := strings.TrimSpace(c.Query("category")); value != "" {
		query = query.Where("category = ?", value)
	}
	if value := strings.TrimSpace(c.Query("q")); value != "" {
		like := "%" + value + "%"
		query = query.Where("(note LIKE ? OR category LIKE ?)", like, like)
	}
	if value := c.Query("start"); value != "" {
		query = query.Where("occurred_at >= ?", value)
	}
	if value := c.Query("end"); value != "" {
		query = query.Where("occurred_at < datetime(?, '+1 day')", value)
	}
	return query
}

func loadPersonalRelations(db *gorm.DB, items []PersonalTransaction) error {
	transactionIDs := make([]uint, 0, len(items))
	for _, item := range items {
		transactionIDs = append(transactionIDs, item.ID)
	}
	attachments, err := loadBillAttachments(db, attachmentTypePersonal, transactionIDs)
	if err != nil {
		return err
	}
	for i := range items {
		if err := db.Model(&items[i]).Association("Tags").Find(&items[i].Tags); err != nil {
			return err
		}
		var ledger PersonalLedger
		if err := db.Select("id, name").First(&ledger, items[i].LedgerID).Error; err == nil {
			items[i].LedgerName = ledger.Name
		}
		if items[i].AccountID > 0 {
			var account PersonalAccount
			if err := db.Select("id, name, type").First(&account, items[i].AccountID).Error; err == nil {
				items[i].AccountName = account.Name
				items[i].AccountType = account.Type
			}
		}
		if items[i].CounterpartAccountID > 0 {
			var account PersonalAccount
			if err := db.Select("id, name, type").First(&account, items[i].CounterpartAccountID).Error; err == nil {
				items[i].CounterpartName = account.Name
			}
		}
		loadRefundSourceInfo(db, &items[i])
		items[i].Attachments = attachments[items[i].ID]
	}
	return nil
}

func personalTags(db *gorm.DB, userID uint, ids []uint) ([]PersonalTag, error) {
	if len(ids) == 0 {
		return []PersonalTag{}, nil
	}
	seen := map[uint]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			return nil, errors.New("标签无效")
		}
		seen[id] = true
	}
	var tags []PersonalTag
	if err := db.Where("user_id = ? AND id IN ?", userID, ids).Find(&tags).Error; err != nil {
		return nil, err
	}
	if len(tags) != len(ids) {
		return nil, errors.New("标签不存在或无权使用")
	}
	return tags, nil
}

func personalCategoryExists(db *gorm.DB, userID uint, kind, name string) (bool, error) {
	var count int64
	err := db.Model(&PersonalCategory{}).Where("user_id = ? AND kind = ? AND name = ?", userID, kind, name).Count(&count).Error
	return count > 0, err
}

func validatePersonalRequest(req personalRequest) (PersonalTransaction, error) {
	if !validTransactionKind(req.Kind) || req.AmountCents <= 0 || req.AmountCents > 999999999999 {
		return PersonalTransaction{}, errInvalid
	}
	if req.AccountID == 0 {
		return PersonalTransaction{}, errors.New("请选择收支账户")
	}
	occurredAt, err := parseOccurredAt(req.OccurredAt)
	if err != nil {
		return PersonalTransaction{}, errors.New("发生时间格式不正确")
	}
	item := PersonalTransaction{
		LedgerID:    req.LedgerID,
		AccountID:   req.AccountID,
		Kind:        req.Kind,
		AmountCents: req.AmountCents,
		Note:        cleanText(req.Note, 500),
		OccurredAt:  occurredAt,
	}
	if req.Kind == KindTransfer {
		if req.CounterpartAccountID == 0 {
			return PersonalTransaction{}, errors.New("请选择转入账户")
		}
		if req.CounterpartAccountID == req.AccountID {
			return PersonalTransaction{}, errors.New("转出账户与转入账户不能相同")
		}
		item.CounterpartAccountID = req.CounterpartAccountID
		item.Category = TransferCategory
		return item, nil
	}
	if req.Kind == KindRefund {
		if req.RefundSourceID == 0 {
			return PersonalTransaction{}, errors.New("请选择原支出的账单")
		}
		item.RefundSourceID = req.RefundSourceID
		item.Category = RefundCategory
		return item, nil
	}
	category := cleanText(req.Category, 64)
	if category == "" {
		return PersonalTransaction{}, errors.New("请选择分类")
	}
	item.Category = category
	return item, nil
}

// validatePersonalCategory checks that a non-transfer transaction still points
// at a category the user owns. Transfers carry a fixed category and are
// skipped. The boolean result is false when the category must be rejected.
func validatePersonalCategory(db *gorm.DB, userID uint, item PersonalTransaction, existing *PersonalTransaction) (bool, error) {
	if item.Kind == KindTransfer || item.Kind == KindRefund {
		return true, nil
	}
	categoryExists, err := personalCategoryExists(db, userID, item.Kind, item.Category)
	if err != nil {
		return false, err
	}
	if categoryExists {
		return true, nil
	}
	if existing != nil && existing.Kind == item.Kind && existing.Category == item.Category {
		return true, nil
	}
	return false, nil
}

// ownedCounterpartAccount validates the receiving account of a transfer.
func ownedCounterpartAccount(db *gorm.DB, userID, accountID uint) (PersonalAccount, error) {
	if accountID == 0 {
		return PersonalAccount{}, errors.New("请选择转入账户")
	}
	return ownedPersonalAccount(db, userID, accountID, true)
}

func handleListPersonal(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID, ok := dataOwner(db, c)
		if !ok {
			return
		}
		if _, err := ensurePersonalSetup(db, ownerID); err != nil {
			response.ErrorInternal(c, "获取账单失败")
			return
		}
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("pageSize"), 20))
		query := personalQuery(db, ownerID, c)
		var total int64
		if err := query.Count(&total).Error; err != nil {
			response.ErrorInternal(c, "获取账单失败")
			return
		}
		var items []PersonalTransaction
		if err := query.Order("occurred_at DESC, id DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "获取账单失败")
			return
		}
		if err := loadPersonalRelations(db, items); err != nil {
			response.ErrorInternal(c, "获取账单失败")
			return
		}
		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func handleCreatePersonal(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		defaultLedger, err := ensurePersonalSetup(db, c.GetUint("userID"))
		if err != nil {
			response.ErrorInternal(c, "创建账单失败")
			return
		}
		var req personalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入完整账单信息")
			return
		}
		item, err := validatePersonalRequest(req)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		item.UserID = c.GetUint("userID")
		if item.LedgerID == 0 {
			item.LedgerID = defaultLedger.ID
		}
		ledger, err := ownedPersonalLedger(db, item.UserID, item.LedgerID, true)
		if err != nil {
			respondPersonalError(c, err, "账本不存在")
			return
		}
		account, err := ownedPersonalAccount(db, item.UserID, item.AccountID, true)
		if err != nil {
			respondPersonalError(c, err, "请选择有效的收支账户")
			return
		}
		var counterpart PersonalAccount
		if item.Kind == KindTransfer {
			counterpart, err = ownedCounterpartAccount(db, item.UserID, item.CounterpartAccountID)
			if err != nil {
				respondPersonalError(c, err, "请选择有效的转入账户")
				return
			}
		}
		if item.Kind == KindRefund {
			if _, err := validateRefundAmount(db, item.UserID, item.RefundSourceID, item.AmountCents, 0); err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
		}
		categoryValid, err := validatePersonalCategory(db, item.UserID, item, nil)
		if err != nil {
			response.ErrorInternal(c, "校验分类失败")
			return
		}
		if !categoryValid {
			response.ErrorBadRequest(c, "请选择有效的分类")
			return
		}
		tags, err := personalTags(db, item.UserID, req.TagIDs)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		attachments := []BillAttachment{}
		if req.Attachments != nil {
			attachments, err = validateBillAttachments(*req.Attachments)
			if err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			if err := tx.Model(&item).Association("Tags").Replace(tags); err != nil {
				return err
			}
			return replaceBillAttachments(tx, attachmentTypePersonal, item.ID, attachments)
		}); err != nil {
			response.ErrorInternal(c, "创建账单失败")
			return
		}
		item.Tags = tags
		item.Attachments = attachments
		item.LedgerName = ledger.Name
		item.AccountName = account.Name
		item.AccountType = account.Type
		if item.Kind == KindTransfer {
			item.CounterpartName = counterpart.Name
		}
		if item.Kind == KindRefund {
			loadRefundSourceInfo(db, &item)
		}
		audit.Log(db, c, "personal_transaction_create", "personal_transaction", item.ID, "新增个人账单")
		response.Success(c, item)
	}
}

func handleUpdatePersonal(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		defaultLedger, setupErr := ensurePersonalSetup(db, c.GetUint("userID"))
		if setupErr != nil {
			response.ErrorInternal(c, "更新账单失败")
			return
		}
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			response.ErrorBadRequest(c, "无效的账单 ID")
			return
		}
		var existing PersonalTransaction
		if err := db.Where("id = ? AND user_id = ?", uint(id), c.GetUint("userID")).First(&existing).Error; err != nil {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "账单不存在")
			return
		}
		// The state before the edit is kept so the audit log can tell what
		// actually changed.
		before := existing
		var req personalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.ErrorBadRequest(c, "请输入完整账单信息")
			return
		}
		item, err := validatePersonalRequest(req)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		if item.LedgerID == 0 {
			item.LedgerID = defaultLedger.ID
		}
		ledger, err := ownedPersonalLedger(db, c.GetUint("userID"), item.LedgerID, true)
		if err != nil {
			respondPersonalError(c, err, "账本不存在")
			return
		}
		account, err := ownedPersonalAccount(db, c.GetUint("userID"), item.AccountID, true)
		if err != nil {
			respondPersonalError(c, err, "请选择有效的收支账户")
			return
		}
		var counterpart PersonalAccount
		if item.Kind == KindTransfer {
			counterpart, err = ownedCounterpartAccount(db, c.GetUint("userID"), item.CounterpartAccountID)
			if err != nil {
				respondPersonalError(c, err, "请选择有效的转入账户")
				return
			}
		}
		if item.Kind == KindRefund {
			// The bill a refund points at is bound once the refund exists: editing
			// the amount, the account or the date must not move the refund to
			// another expense.
			if existing.Kind == KindRefund && existing.RefundSourceID > 0 {
				item.RefundSourceID = existing.RefundSourceID
			}
			if _, err := validateRefundAmount(db, c.GetUint("userID"), item.RefundSourceID, item.AmountCents, existing.ID); err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
		}
		categoryValid, err := validatePersonalCategory(db, c.GetUint("userID"), item, &existing)
		if err != nil {
			response.ErrorInternal(c, "校验分类失败")
			return
		}
		if !categoryValid {
			response.ErrorBadRequest(c, "请选择有效的分类")
			return
		}
		tags, err := personalTags(db, c.GetUint("userID"), req.TagIDs)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		var attachments []BillAttachment
		if req.Attachments != nil {
			attachments, err = validateBillAttachments(*req.Attachments)
			if err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"ledger_id":              item.LedgerID,
				"account_id":             item.AccountID,
				"counterpart_account_id": item.CounterpartAccountID,
				"refund_source_id":       item.RefundSourceID,
				"kind":                   item.Kind, "amount_cents": item.AmountCents, "category": item.Category,
				"note": item.Note, "occurred_at": item.OccurredAt,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&existing).Association("Tags").Replace(tags); err != nil {
				return err
			}
			if req.Attachments != nil {
				return replaceBillAttachments(tx, attachmentTypePersonal, existing.ID, attachments)
			}
			return nil
		}); err != nil {
			response.ErrorInternal(c, "更新账单失败")
			return
		}
		db.First(&existing, existing.ID)
		existing.Tags = tags
		existing.LedgerName = ledger.Name
		existing.AccountName = account.Name
		existing.AccountType = account.Type
		if item.Kind == KindTransfer {
			existing.CounterpartName = counterpart.Name
		}
		if item.Kind == KindRefund {
			loadRefundSourceInfo(db, &existing)
		}
		attachmentMap, err := loadBillAttachments(db, attachmentTypePersonal, []uint{existing.ID})
		if err != nil {
			response.ErrorInternal(c, "更新账单失败")
			return
		}
		existing.Attachments = attachmentMap[existing.ID]
		audit.Log(db, c, "personal_transaction_update", "personal_transaction", existing.ID, personalTransactionChangeSummary(before, existing))
		response.Success(c, existing)
	}
}

// auditAmount renders an amount in cents for the audit log.
func auditAmount(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

// personalTransactionChangeSummary describes what an edit changed so the
// audit log stays readable instead of just saying "updated".
func personalTransactionChangeSummary(before, after PersonalTransaction) string {
	changes := make([]string, 0, 7)
	if before.Kind != after.Kind {
		changes = append(changes, fmt.Sprintf("类型 %s→%s", before.Kind, after.Kind))
	}
	if before.AmountCents != after.AmountCents {
		changes = append(changes, fmt.Sprintf("金额 %s→%s", auditAmount(before.AmountCents), auditAmount(after.AmountCents)))
	}
	if before.Category != after.Category {
		changes = append(changes, fmt.Sprintf("分类 %s→%s", before.Category, after.Category))
	}
	if before.LedgerID != after.LedgerID {
		changes = append(changes, "账本变更")
	}
	if before.AccountID != after.AccountID {
		changes = append(changes, "账户变更")
	}
	if before.Note != after.Note {
		changes = append(changes, "备注修改")
	}
	if !before.OccurredAt.Equal(after.OccurredAt) {
		changes = append(changes, fmt.Sprintf("日期 %s→%s", before.OccurredAt.Format("2006-01-02"), after.OccurredAt.Format("2006-01-02")))
	}
	if len(changes) == 0 {
		return "修改个人账单"
	}
	return "修改个人账单：" + strings.Join(changes, "，")
}

func handleDeletePersonal(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil {
			response.ErrorBadRequest(c, "无效的账单 ID")
			return
		}
		var existing PersonalTransaction
		if err := db.Where("id = ? AND user_id = ?", uint(id), c.GetUint("userID")).First(&existing).Error; err != nil {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "账单不存在")
			return
		}
		result := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("transaction_id = ?", existing.ID).Delete(&PersonalTransactionTag{}).Error; err != nil {
				return err
			}
			if err := deleteBillAttachments(tx, attachmentTypePersonal, existing.ID); err != nil {
				return err
			}
			return tx.Delete(&existing).Error
		})
		if result != nil {
			response.ErrorInternal(c, "删除账单失败")
			return
		}
		audit.Log(db, c, "personal_transaction_delete", "personal_transaction", uint(id), "删除个人账单")
		response.Success(c, nil)
	}
}

func personalStatsForLedger(db *gorm.DB, userID uint, start, end string, ledgerID uint) (statsSummary, []categoryStat, []monthlyStat, error) {
	base := db.Model(&PersonalTransaction{}).Where("user_id = ?", userID)
	if ledgerID > 0 {
		base = base.Where("ledger_id = ?", ledgerID)
	}
	if start != "" {
		base = base.Where("occurred_at >= ?", start)
	}
	if end != "" {
		base = base.Where("occurred_at < datetime(?, '+1 day')", end)
	}
	var rows []struct {
		Kind  string
		Total int64
		Count int64
	}
	if err := base.Select("kind, COALESCE(SUM(amount_cents), 0) AS total, COUNT(*) AS count").Group("kind").Scan(&rows).Error; err != nil {
		return statsSummary{}, nil, nil, err
	}
	var summary statsSummary
	for _, row := range rows {
		summary.Count += row.Count
		switch row.Kind {
		case KindIncome:
			summary.IncomeCents += row.Total
		case KindExpense:
			summary.ExpenseCents += row.Total
		}
	}
	summary.BalanceCents = summary.IncomeCents - summary.ExpenseCents

	var categories []categoryStat
	// The category of a record is a name, so the first level category is
	// resolved through the category tree. Records booked on a first level
	// category directly keep an empty parent.
	categoryQuery := applyStatsRange(db.Table("personal_transactions").
		Select("personal_transactions.category AS category, COALESCE(parent_category.name, '') AS parent, SUM(personal_transactions.amount_cents) AS amount_cents, COUNT(*) AS count").
		Joins("LEFT JOIN personal_categories child_category ON child_category.user_id = personal_transactions.user_id AND child_category.kind = personal_transactions.kind AND child_category.name = personal_transactions.category").
		Joins("LEFT JOIN personal_categories parent_category ON parent_category.id = child_category.parent_id"), userID, start, end, ledgerID)
	if err := categoryQuery.Where("personal_transactions.kind = ?", KindExpense).
		Group("personal_transactions.category, parent").
		Order("amount_cents DESC").Scan(&categories).Error; err != nil {
		return summary, nil, nil, err
	}

	since := time.Now().AddDate(0, -5, 0)
	since = time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.Local)
	var trendRows []struct {
		Month string
		Kind  string
		Total int64
	}
	trendQuery := db.Model(&PersonalTransaction{}).Where("user_id = ? AND occurred_at >= ?", userID, since)
	if ledgerID > 0 {
		trendQuery = trendQuery.Where("ledger_id = ?", ledgerID)
	}
	if err := trendQuery.
		Select("strftime('%Y-%m', occurred_at) AS month, kind, SUM(amount_cents) AS total").
		Group("month, kind").Order("month ASC").Scan(&trendRows).Error; err != nil {
		return summary, categories, nil, err
	}
	byMonth := map[string]*monthlyStat{}
	for i := 0; i < 6; i++ {
		month := since.AddDate(0, i, 0).Format("2006-01")
		byMonth[month] = &monthlyStat{Month: month}
	}
	for _, row := range trendRows {
		item := byMonth[row.Month]
		if item == nil {
			continue
		}
		switch row.Kind {
		case KindIncome:
			item.IncomeCents += row.Total
		case KindExpense:
			item.ExpenseCents += row.Total
		}
	}
	trend := make([]monthlyStat, 0, 6)
	for i := 0; i < 6; i++ {
		trend = append(trend, *byMonth[since.AddDate(0, i, 0).Format("2006-01")])
	}
	return summary, categories, trend, nil
}

func personalStats(db *gorm.DB, userID uint, start, end string) (statsSummary, []categoryStat, []monthlyStat, error) {
	return personalStatsForLedger(db, userID, start, end, 0)
}

type dimensionStat struct {
	ID           uint   `json:"id,omitempty"`
	Name         string `json:"name"`
	AmountCents  int64  `json:"amount_cents"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
	Count        int64  `json:"count"`
	// The counts per kind let the client rank a dimension by income or by
	// expense without mixing both directions.
	IncomeCount  int64 `json:"income_count"`
	ExpenseCount int64 `json:"expense_count"`
}

type dailyStat struct {
	Date         string `json:"date"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
	Count        int64  `json:"count"`
}
type weekdayStat struct {
	Weekday      int   `json:"weekday"`
	AmountCents  int64 `json:"amount_cents"`
	IncomeCents  int64 `json:"income_cents"`
	ExpenseCents int64 `json:"expense_cents"`
	Count        int64 `json:"count"`
}

func applyStatsRange(query *gorm.DB, userID uint, start, end string, ledgerID uint) *gorm.DB {
	query = query.Where("personal_transactions.user_id = ?", userID)
	if ledgerID > 0 {
		query = query.Where("personal_transactions.ledger_id = ?", ledgerID)
	}
	if start != "" {
		query = query.Where("personal_transactions.occurred_at >= ?", start)
	}
	if end != "" {
		query = query.Where("personal_transactions.occurred_at < datetime(?, '+1 day')", end)
	}
	return query
}

func personalDimensions(db *gorm.DB, userID uint, start, end string, ledgerID uint) ([]dimensionStat, []dimensionStat, []dailyStat, []weekdayStat, error) {
	var ledgerRows, tagRows []struct {
		ID           uint
		Name, Kind   string
		Total, Count int64
	}
	ledgerQuery := applyStatsRange(db.Table("personal_transactions").
		Select("personal_ledgers.id, personal_ledgers.name, personal_transactions.kind, SUM(personal_transactions.amount_cents) AS total, COUNT(*) AS count").
		Joins("JOIN personal_ledgers ON personal_ledgers.id = personal_transactions.ledger_id"), userID, start, end, ledgerID)
	if err := ledgerQuery.Group("personal_ledgers.id, personal_ledgers.name, personal_transactions.kind").Scan(&ledgerRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	tagQuery := applyStatsRange(db.Table("personal_transactions").
		Select("personal_tags.id, personal_tags.name, personal_transactions.kind, SUM(personal_transactions.amount_cents) AS total, COUNT(*) AS count").
		Joins("JOIN personal_transaction_tags ON personal_transaction_tags.transaction_id = personal_transactions.id").
		Joins("JOIN personal_tags ON personal_tags.id = personal_transaction_tags.tag_id"), userID, start, end, ledgerID)
	if err := tagQuery.Group("personal_tags.id, personal_tags.name, personal_transactions.kind").Scan(&tagRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	combine := func(rows []struct {
		ID           uint
		Name, Kind   string
		Total, Count int64
	}) []dimensionStat {
		items := map[uint]*dimensionStat{}
		order := []uint{}
		for _, row := range rows {
			item := items[row.ID]
			if item == nil {
				item = &dimensionStat{ID: row.ID, Name: row.Name}
				items[row.ID] = item
				order = append(order, row.ID)
			}
			item.Count += row.Count
			switch row.Kind {
			case KindIncome:
				item.IncomeCents += row.Total
				item.IncomeCount += row.Count
			case KindExpense:
				item.ExpenseCents += row.Total
				item.ExpenseCount += row.Count
			}
		}
		result := make([]dimensionStat, 0, len(order))
		for _, id := range order {
			items[id].AmountCents = items[id].IncomeCents + items[id].ExpenseCents
			result = append(result, *items[id])
		}
		return result
	}
	var dailyRows []struct {
		Date, Kind   string
		Total, Count int64
	}
	dailyQuery := applyStatsRange(db.Model(&PersonalTransaction{}).Select("strftime('%Y-%m-%d', occurred_at) AS date, kind, SUM(amount_cents) AS total, COUNT(*) AS count"), userID, start, end, ledgerID)
	if err := dailyQuery.Group("date, kind").Order("date ASC").Scan(&dailyRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	dailyMap := map[string]*dailyStat{}
	dates := []string{}
	for _, row := range dailyRows {
		item := dailyMap[row.Date]
		if item == nil {
			item = &dailyStat{Date: row.Date}
			dailyMap[row.Date] = item
			dates = append(dates, row.Date)
		}
		item.Count += row.Count
		switch row.Kind {
		case KindIncome:
			item.IncomeCents += row.Total
		case KindExpense:
			item.ExpenseCents += row.Total
		}
	}
	daily := make([]dailyStat, 0, len(dates))
	for _, date := range dates {
		daily = append(daily, *dailyMap[date])
	}
	var weekdayRows []struct {
		Weekday      int
		Kind         string
		Total, Count int64
	}
	weekdayQuery := applyStatsRange(db.Model(&PersonalTransaction{}).Select("CAST(strftime('%w', occurred_at) AS INTEGER) AS weekday, kind, SUM(amount_cents) AS total, COUNT(*) AS count"), userID, start, end, ledgerID)
	if err := weekdayQuery.Group("weekday, kind").Order("weekday ASC").Scan(&weekdayRows).Error; err != nil {
		return nil, nil, nil, nil, err
	}
	weekdays := make([]weekdayStat, 7)
	for i := range weekdays {
		weekdays[i].Weekday = i
	}
	for _, row := range weekdayRows {
		switch row.Kind {
		case KindIncome:
			weekdays[row.Weekday].IncomeCents += row.Total
		case KindExpense:
			weekdays[row.Weekday].ExpenseCents = row.Total
			weekdays[row.Weekday].AmountCents = row.Total
			weekdays[row.Weekday].Count = row.Count
		}
	}
	return combine(ledgerRows), combine(tagRows), daily, weekdays, nil
}

// monthlyTrendStat is one month of the yearly trend of the statistics page.
type monthlyTrendStat struct {
	Month        string `json:"month"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
	BalanceCents int64  `json:"balance_cents"`
	Count        int64  `json:"count"`
}

// personalMonthlyTrend aggregates the income and the expenses of every month
// of the requested year. The year fully defines the range, so the trend always
// shows all twelve months of it, even the ones without any record.
func personalMonthlyTrend(db *gorm.DB, userID uint, year int, ledgerID uint) ([]monthlyTrendStat, error) {
	if year < 1 || year > 9999 {
		year = time.Now().Year()
	}
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)
	query := db.Model(&PersonalTransaction{}).
		Where("user_id = ? AND occurred_at >= ? AND occurred_at < ?", userID, start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
	if ledgerID > 0 {
		query = query.Where("ledger_id = ?", ledgerID)
	}
	var rows []struct {
		Month string
		Kind  string
		Total int64
		Count int64
	}
	if err := query.
		Select("strftime('%Y-%m', occurred_at) AS month, kind, SUM(amount_cents) AS total, COUNT(*) AS count").
		Group("month, kind").Order("month ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]monthlyTrendStat, 0, 12)
	position := make(map[string]int, 12)
	for i := 0; i < 12; i++ {
		key := fmt.Sprintf("%d-%02d", year, i+1)
		position[key] = len(items)
		items = append(items, monthlyTrendStat{Month: key})
	}
	for _, row := range rows {
		index, ok := position[row.Month]
		if !ok {
			continue
		}
		item := &items[index]
		item.Count += row.Count
		switch row.Kind {
		case KindIncome:
			item.IncomeCents += row.Total
		case KindExpense:
			item.ExpenseCents += row.Total
		}
	}
	for i := range items {
		items[i].BalanceCents = items[i].IncomeCents - items[i].ExpenseCents
	}
	return items, nil
}

func handlePersonalStats(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID, ok := dataOwner(db, c)
		if !ok {
			return
		}
		if _, err := ensurePersonalSetup(db, ownerID); err != nil {
			response.ErrorInternal(c, "统计分析失败")
			return
		}
		ledgerID := uint(utils.Atoi(c.Query("ledger_id"), 0))
		if ledgerID > 0 {
			if _, err := ownedPersonalLedger(db, ownerID, ledgerID, false); err != nil {
				respondPersonalError(c, err, "账本不存在")
				return
			}
		}
		summary, categories, trend, err := personalStatsForLedger(db, ownerID, c.Query("start"), c.Query("end"), ledgerID)
		if err != nil {
			response.ErrorInternal(c, "统计分析失败")
			return
		}
		ledgers, tags, daily, weekdays, err := personalDimensions(db, ownerID, c.Query("start"), c.Query("end"), ledgerID)
		if err != nil {
			response.ErrorInternal(c, "统计分析失败")
			return
		}
		// The years the user recorded something in feed the year picker of the
		// statistics page so the quick range never offers an empty year.
		years, err := personalFlowYears(db, ownerID)
		if err != nil {
			response.ErrorInternal(c, "统计分析失败")
			return
		}
		monthly, err := personalMonthlyTrend(db, ownerID, utils.Atoi(c.Query("trend_year"), 0), ledgerID)
		if err != nil {
			response.ErrorInternal(c, "统计分析失败")
			return
		}
		response.Success(c, gin.H{"summary": summary, "categories": categories, "trend": trend, "ledgers": ledgers, "tags": tags, "daily": daily, "weekdays": weekdays, "available_years": years, "monthly": monthly})
	}
}

func handleExportPersonal(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID, ok := dataOwner(db, c)
		if !ok {
			return
		}
		if _, err := ensurePersonalSetup(db, ownerID); err != nil {
			response.ErrorInternal(c, "导出账单失败")
			return
		}
		var items []PersonalTransaction
		if err := personalQuery(db, ownerID, c).Order("occurred_at DESC, id DESC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "导出账单失败")
			return
		}
		if err := loadPersonalRelations(db, items); err != nil {
			response.ErrorInternal(c, "导出账单失败")
			return
		}
		filename := "personal-bills-" + time.Now().Format("20060102") + ".csv"
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Status(http.StatusOK)
		_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
		writer := csv.NewWriter(c.Writer)
		defer writer.Flush()
		_ = writer.Write([]string{"日期", "账本", "收支账户", "对方账户", "类型", "分类", "标签", "金额", "备注"})
		for _, item := range items {
			kind := "支出"
			switch item.Kind {
			case KindIncome:
				kind = "收入"
			case KindTransfer:
				kind = "转账"
			case KindRefund:
				kind = "退款"
			}
			tagNames := make([]string, 0, len(item.Tags))
			for _, tag := range item.Tags {
				tagNames = append(tagNames, tag.Name)
			}
			related := item.CounterpartName
			if item.Kind == KindRefund && item.RefundSourceCategory != "" {
				related = "退款自：" + item.RefundSourceCategory
			}
			_ = writer.Write([]string{
				item.OccurredAt.Format("2006-01-02 15:04"),
				item.LedgerName,
				item.AccountName,
				related,
				kind,
				item.Category,
				strings.Join(tagNames, "、"),
				fmt.Sprintf("%.2f", float64(item.AmountCents)/100),
				item.Note,
			})
		}
	}
}
