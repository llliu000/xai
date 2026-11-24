package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UsageRecord stores one immutable revision in the main database.
type UsageRecord struct {
	Sequence          int64     `gorm:"primaryKey;index:idx_usage_record_scan,priority:3" json:"-"`
	AccountID         int       `gorm:"not null;uniqueIndex:uk_usage_record_version,priority:1;index:idx_usage_record_scan,priority:1" json:"account_id,string"`
	RecordID          string    `gorm:"size:192;not null;uniqueIndex:uk_usage_record_version,priority:2" json:"id"`
	Version           int       `gorm:"not null;uniqueIndex:uk_usage_record_version,priority:3" json:"version"`
	TokenID           int       `gorm:"not null" json:"token_id,string"`
	ExternalAccountID string    `gorm:"size:192" json:"external_account_id,omitempty"`
	RequestID         string    `gorm:"size:192;not null" json:"request_id"`
	TaskID            *string   `gorm:"size:192" json:"task_id"`
	Model             string    `gorm:"size:192;not null" json:"model"`
	Capability        string    `gorm:"size:32;not null" json:"capability"`
	Status            string    `gorm:"size:20;not null" json:"status"`
	RequestStatus     string    `gorm:"size:16;not null" json:"request_status"`
	Currency          string    `gorm:"size:3;not null" json:"currency"`
	GrossAmount       *string   `gorm:"size:32" json:"gross_amount"`
	RefundAmount      *string   `gorm:"size:32" json:"refund_amount"`
	NetAmount         *string   `gorm:"size:32" json:"net_amount"`
	Quantities        JSONValue `gorm:"type:text;not null" json:"quantities"`
	PriceVersion      string    `gorm:"size:192;not null" json:"price_version"`
	CreatedAt         int64     `gorm:"autoCreateTime:false;not null" json:"created_at"`
	UpdatedAt         int64     `gorm:"autoUpdateTime:false;not null;index:idx_usage_record_scan,priority:2" json:"updated_at"`
	SettledAt         *int64    `json:"settled_at"`
	ErrorCode         *string   `gorm:"size:128" json:"error_code"`
	Metadata          JSONValue `gorm:"type:text;not null" json:"metadata"`
	TaskCharge        JSONValue `gorm:"type:text" json:"-"`
	TaskOutcome       JSONValue `gorm:"type:text" json:"-"`
}

type TaskUsageCharge struct {
	Quota        int     `json:"quota"`
	QuotaPerUSD  float64 `json:"quota_per_usd"`
	PriceVersion string  `json:"price_version"`
}

type TaskUsageOutcome struct {
	Quota         int    `json:"quota"`
	RequestStatus string `json:"request_status"`
}

type UsageQuantity struct {
	Metric       string `json:"metric"`
	Quantity     string `json:"quantity"`
	UnitPrice    string `json:"unit_price"`
	UnitQuantity int    `json:"unit_quantity"`
	Amount       string `json:"amount"`
}

type UsageRecordFilter struct {
	UpdatedFrom       int64
	UpdatedTo         int64
	RequestID         string
	TaskID            string
	TokenID           int
	ExternalAccountID string
	Model             string
	Status            string
}

func ListUsageRecords(ctx context.Context, accountID int, filter UsageRecordFilter, afterTime, afterSequence int64, limit int) ([]UsageRecord, error) {
	rows := make([]UsageRecord, 0)
	query := DB.WithContext(ctx).Where("account_id = ? AND updated_at >= ? AND updated_at < ?", accountID, filter.UpdatedFrom, filter.UpdatedTo)
	if afterSequence > 0 {
		query = query.Where("updated_at > ? OR (updated_at = ? AND sequence > ?)", afterTime, afterTime, afterSequence)
	}
	if filter.TokenID > 0 {
		query = query.Where("token_id = ?", filter.TokenID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	for _, field := range []struct{ column, value string }{
		{"request_id", filter.RequestID}, {"task_id", filter.TaskID}, {"external_account_id", filter.ExternalAccountID}, {"model", filter.Model},
	} {
		if field.value != "" {
			query = whereExactUsageIdentifier(query, field.column, field.value)
		}
	}
	err := query.Order("updated_at, sequence").Limit(limit).Find(&rows).Error
	return rows, err
}

func GetUsageRecord(ctx context.Context, accountID int, recordID string) (UsageRecord, error) {
	var record UsageRecord
	query := DB.WithContext(ctx).Where("account_id = ?", accountID)
	err := whereExactUsageIdentifier(query, "record_id", recordID).Order("version DESC").First(&record).Error
	return record, err
}

// RecordInitialUsageSettlement records the synchronous result at most once.
// A later correction must preserve this revision and append its own version.
func RecordInitialUsageSettlement(ctx context.Context, pending UsageRecord, status, requestStatus, amount, priceVersion string, settledAt int64, quantities []UsageQuantity) error {
	pending.Sequence = 0
	pending.Version = 2
	pending.Status = status
	pending.RequestStatus = requestStatus
	pending.GrossAmount = &amount
	pending.RefundAmount = new("0.00000000")
	pending.NetAmount = &amount
	pending.PriceVersion = priceVersion
	pending.UpdatedAt = max(pending.UpdatedAt, settledAt)
	pending.SettledAt = &settledAt
	data, err := common.Marshal(quantities)
	if err != nil {
		return err
	}
	pending.Quantities = JSONValue(data)
	return insertUsageRevision(DB.WithContext(ctx), &pending)
}

func RecordTaskUsageCharge(ctx context.Context, task *Task, charge TaskUsageCharge) error {
	return recordTaskUsageEvent(ctx, task, "task_charge", charge)
}

func RecordTaskUsageOutcome(ctx context.Context, task *Task, outcome TaskUsageOutcome) error {
	return recordTaskUsageEvent(ctx, task, "task_outcome", outcome)
}

// The first revision holds private confirmations while its public fields stay
// pending. Either confirmation can arrive first. The guarded write locks the
// row before reading it, including on SQLite, where writes are serialized.
func recordTaskUsageEvent(ctx context.Context, task *Task, column string, event any) error {
	data, err := common.Marshal(event)
	if err != nil {
		return err
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := func() *gorm.DB {
			query := tx.Model(&UsageRecord{}).Where("account_id = ? AND version = ?", task.UserId, 1)
			query = whereExactUsageIdentifier(query, "record_id", "bill_"+task.PrivateData.Execution.RequestID)
			return whereExactUsageIdentifier(query, "task_id", task.TaskID)
		}
		write := query().Where(column+" IS NULL").UpdateColumn(column, JSONValue(data))
		if write.Error != nil || write.RowsAffected == 0 {
			return write.Error
		}
		var record UsageRecord
		if err := query().First(&record).Error; err != nil {
			return err
		}
		if record.TaskCharge == nil || record.TaskOutcome == nil {
			return nil
		}
		var charge TaskUsageCharge
		var outcome TaskUsageOutcome
		if err := common.Unmarshal(record.TaskCharge, &charge); err != nil {
			return err
		}
		if err := common.Unmarshal(record.TaskOutcome, &outcome); err != nil {
			return err
		}
		grossQuota := max(charge.Quota, outcome.Quota)
		unit := decimal.NewFromFloat(charge.QuotaPerUSD)
		gross := decimal.NewFromInt(int64(grossQuota)).DivRound(unit, 8)
		net := decimal.NewFromInt(int64(outcome.Quota)).DivRound(unit, 8)
		record.Sequence, record.Version = 0, 2
		record.TaskCharge, record.TaskOutcome = nil, nil
		record.GrossAmount = new(gross.StringFixed(8))
		record.RefundAmount = new(gross.Sub(net).StringFixed(8))
		record.NetAmount = new(net.StringFixed(8))
		data, err := common.Marshal([]UsageQuantity{{Metric: "task", Quantity: "1", UnitQuantity: 1, UnitPrice: net.StringFixed(8), Amount: net.StringFixed(8)}})
		if err != nil {
			return err
		}
		record.Quantities = JSONValue(data)
		record.Status = "settled"
		switch {
		case grossQuota == 0:
			record.Status = "not_charged"
		case outcome.Quota == 0:
			record.Status = "refunded"
		case outcome.Quota < charge.Quota:
			record.Status = "partially_refunded"
		}
		record.RequestStatus = outcome.RequestStatus
		record.PriceVersion = charge.PriceVersion
		record.UpdatedAt = max(record.UpdatedAt, common.GetTimestamp())
		record.SettledAt = new(record.UpdatedAt)
		return insertUsageRevision(tx, &record)
	})
}

func insertUsageRevision(tx *gorm.DB, record *UsageRecord) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}, {Name: "record_id"}, {Name: "version"}},
		DoNothing: true,
	}).Create(record).Error
}

func whereExactUsageIdentifier(query *gorm.DB, column, value string) *gorm.DB {
	// Retain the indexed equality check on MySQL, then enforce the wire
	// contract's case sensitivity even under a case-insensitive collation.
	query = query.Where(column+" = ?", value)
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		query = query.Where("BINARY "+column+" = ?", value)
	}
	return query
}
