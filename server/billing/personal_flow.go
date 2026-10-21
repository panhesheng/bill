package billing

import (
	"fmt"
	"time"

	"smallgo/server/response"
	"smallgo/server/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// flowEntry is one period of the cash flow list: a year, a month or a day.
type flowEntry struct {
	Key          string `json:"key"`
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	IncomeCents  int64  `json:"income_cents"`
	ExpenseCents int64  `json:"expense_cents"`
	BalanceCents int64  `json:"balance_cents"`
	Count        int64  `json:"count"`
}

type personalFlowResponse struct {
	Granularity    string      `json:"granularity"`
	Year           int         `json:"year"`
	Month          int         `json:"month"`
	Entries        []flowEntry `json:"entries"`
	AvailableYears []int       `json:"available_years"`
}

// personalFlowGranularity derives the level of detail from the requested
// year and month: no year means a year per row, a year means a month per row
// and a year plus a month means a day per row.
func personalFlowGranularity(year, month int) string {
	if year > 0 && month > 0 {
		return "day"
	}
	if year > 0 {
		return "month"
	}
	return "year"
}

// personalFlowYears lists the years the user has recorded income or expenses
// in, newest first, so the client can offer them in a picker.
func personalFlowYears(db *gorm.DB, userID uint) ([]int, error) {
	var years []int
	err := db.Model(&PersonalTransaction{}).
		Where("user_id = ? AND kind IN (?, ?)", userID, KindIncome, KindExpense).
		Select("DISTINCT CAST(strftime('%Y', occurred_at) AS INTEGER)").
		Order("1 DESC").
		Scan(&years).Error
	return years, err
}

// personalFlowEntries aggregates the income and the expenses of every period
// inside the requested range. Transfers and refunds are neither income nor
// expense, so they never show up here.
func personalFlowEntries(db *gorm.DB, userID uint, granularity string, year, month int, ledgerID uint) ([]flowEntry, error) {
	format := "%Y"
	switch granularity {
	case "month":
		format = "%Y-%m"
	case "day":
		format = "%Y-%m-%d"
	}
	query := db.Model(&PersonalTransaction{}).Where("user_id = ?", userID)
	if ledgerID > 0 {
		query = query.Where("ledger_id = ?", ledgerID)
	}
	if year > 0 {
		start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)
		if month > 0 {
			start = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
			end = start.AddDate(0, 1, 0)
		}
		query = query.Where("occurred_at >= ? AND occurred_at < ?", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"))
	}
	var rows []struct {
		Period string
		Kind   string
		Total  int64
		Count  int64
	}
	if err := query.
		Select("strftime('" + format + "', occurred_at) AS period, kind, SUM(amount_cents) AS total, COUNT(*) AS count").
		Group("period, kind").Order("period DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	type bucket struct {
		income  int64
		expense int64
		count   int64
	}
	buckets := map[string]*bucket{}
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		item, ok := buckets[row.Period]
		if !ok {
			item = &bucket{}
			buckets[row.Period] = item
			order = append(order, row.Period)
		}
		switch row.Kind {
		case KindIncome:
			item.income += row.Total
			item.count += row.Count
		case KindExpense:
			item.expense += row.Total
			item.count += row.Count
		}
	}
	entries := make([]flowEntry, 0, len(order))
	for _, period := range order {
		item := buckets[period]
		if item.count == 0 {
			continue
		}
		entry := flowEntry{
			Key:          period,
			IncomeCents:  item.income,
			ExpenseCents: item.expense,
			BalanceCents: item.income - item.expense,
			Count:        item.count,
		}
		applyFlowLabels(&entry, granularity)
		entries = append(entries, entry)
	}
	return entries, nil
}

// applyFlowLabels fills the title and the subtitle of a period.
func applyFlowLabels(entry *flowEntry, granularity string) {
	switch granularity {
	case "month":
		if len(entry.Key) >= 7 {
			entry.Title = fmt.Sprintf("%d月", flowMonth(entry.Key))
			entry.Subtitle = entry.Key[:4]
		}
	case "day":
		if len(entry.Key) >= 10 {
			entry.Title = fmt.Sprintf("%d日", flowDay(entry.Key))
			entry.Subtitle = fmt.Sprintf("%d月%d日", flowMonth(entry.Key), flowDay(entry.Key))
		}
	default:
		entry.Title = fmt.Sprintf("%s年", entry.Key)
		entry.Subtitle = ""
	}
}

func flowMonth(key string) int {
	if len(key) < 7 {
		return 0
	}
	month := 0
	for _, char := range key[5:7] {
		if char < '0' || char > '9' {
			return 0
		}
		month = month*10 + int(char-'0')
	}
	return month
}

func flowDay(key string) int {
	if len(key) < 10 {
		return 0
	}
	day := 0
	for _, char := range key[8:10] {
		if char < '0' || char > '9' {
			return 0
		}
		day = day*10 + int(char-'0')
	}
	return day
}

// handlePersonalFlow returns the income, expense and balance of every year,
// month or day the user asks for, newest period first.
func handlePersonalFlow(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint("userID")
		year := utils.Atoi(c.Query("year"), 0)
		month := utils.Atoi(c.Query("month"), 0)
		if year < 0 || year > 9999 {
			year = 0
		}
		if month < 0 || month > 12 {
			month = 0
		}
		ledgerID := uint(utils.Atoi(c.Query("ledger_id"), 0))
		granularity := personalFlowGranularity(year, month)
		entries, err := personalFlowEntries(db, userID, granularity, year, month, ledgerID)
		if err != nil {
			response.ErrorInternal(c, "获取收支流水失败")
			return
		}
		years, err := personalFlowYears(db, userID)
		if err != nil {
			response.ErrorInternal(c, "获取收支流水失败")
			return
		}
		response.Success(c, personalFlowResponse{
			Granularity:    granularity,
			Year:           year,
			Month:          month,
			Entries:        entries,
			AvailableYears: years,
		})
	}
}
