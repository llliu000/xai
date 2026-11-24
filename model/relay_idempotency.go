package model

import (
	"context"

	"gorm.io/gorm/clause"
)

type RelayIdempotency struct {
	Scope          string `gorm:"type:char(64);primaryKey"`
	TokenID        int    `gorm:"not null"`
	Fingerprint    string `gorm:"type:char(64);not null"`
	RequestID      string `gorm:"size:64;not null"`
	CreatedAt      int64  `gorm:"not null"`
	FinishedAt     int64
	ResponseStatus int
	Response       string `gorm:"type:text"`
}

// ClaimRelayRequest admits a submitter only after the winning claim has been
// persisted. Reading the winner also handles MySQL's found-rows mode.
func ClaimRelayRequest(ctx context.Context, proposed RelayIdempotency) (RelayIdempotency, error) {
	db := DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&proposed).Error; err != nil {
		return RelayIdempotency{}, err
	}
	var winner RelayIdempotency
	err := db.Where("scope = ?", proposed.Scope).Take(&winner).Error
	return winner, err
}

func FinishRelayRequest(ctx context.Context, claim RelayIdempotency, finishedAt int64, status int, response string) error {
	return DB.WithContext(ctx).Model(&RelayIdempotency{}).
		Where("scope = ? AND request_id = ? AND finished_at = 0", claim.Scope, claim.RequestID).
		Updates(map[string]any{"finished_at": finishedAt, "response_status": status, "response": response}).Error
}
