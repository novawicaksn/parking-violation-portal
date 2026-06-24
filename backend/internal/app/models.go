package app

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type Role string

const (
	RoleOfficer Role = "officer"
	RoleMember  Role = "member"
)

type ViolationType string

const (
	ViolationExpiredMeter     ViolationType = "expired_meter"
	ViolationNoParkingZone    ViolationType = "no_parking_zone"
	ViolationBlockingHydrant  ViolationType = "blocking_hydrant"
	ViolationDisabledSpot     ViolationType = "disabled_spot"
)

type ViolationStatus string

const (
	ViolationUnpaid ViolationStatus = "unpaid"
	ViolationPaid   ViolationStatus = "paid"
	ViolationFailed ViolationStatus = "failed"
)

type PaymentStatus string

const (
	PaymentPaid   PaymentStatus = "paid"
	PaymentFailed PaymentStatus = "failed"
)

type PaymentScenario string

const (
	PaymentScenarioSuccess PaymentScenario = "success"
	PaymentScenarioFailed  PaymentScenario = "failed"
)

type TimeWindow struct {
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Multiplier float64 `json:"multiplier"`
}

type RepeatMultipliers struct {
	Zero    float64 `json:"zero"`
	One     float64 `json:"one"`
	TwoPlus float64 `json:"two_plus"`
}

type FineRules struct {
	BaseAmounts       map[ViolationType]int64 `json:"base_amounts"`
	TimeWindows       []TimeWindow            `json:"time_windows"`
	RepeatMultipliers  RepeatMultipliers       `json:"repeat_multipliers"`
}

type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         Role   `json:"role"`
	Plate        string `json:"plate,omitempty"`
	BalanceCents  int64  `json:"balance_cents"`
	CreatedAt    string `json:"created_at"`
}

type RuleVersion struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Active    bool      `json:"active"`
	Rules     FineRules `json:"rules"`
	CreatedAt string    `json:"created_at"`
}

type Violation struct {
	ID                 string          `json:"id"`
	Plate              string          `json:"plate"`
	ViolationType      ViolationType   `json:"violation_type"`
	Location           string          `json:"location"`
	OccurredAt         string          `json:"occurred_at"`
	PhotoData          string          `json:"photo_data"`
	FineCents          int64           `json:"fine_cents"`
	RuleVersionID      string          `json:"rule_version_id"`
	RuleVersionNumber   int             `json:"rule_version_number"`
	RuleSnapshot       FineRules       `json:"rule_snapshot"`
	PriorUnpaidCount   int             `json:"prior_unpaid_count"`
	TimeMultiplier     float64         `json:"time_multiplier"`
	RepeatMultiplier   float64         `json:"repeat_multiplier"`
	Status             ViolationStatus `json:"status"`
	OwnerUserID        *string         `json:"owner_user_id,omitempty"`
	PaymentTransaction *PaymentRecord   `json:"payment_transaction,omitempty"`
	CreatedAt          string          `json:"created_at"`
}

type PaymentRecord struct {
	ID            string        `json:"id"`
	ViolationID   string        `json:"violation_id"`
	MemberUserID  string        `json:"member_user_id"`
	AmountCents   int64         `json:"amount_cents"`
	Scenario      PaymentScenario `json:"scenario"`
	Status        PaymentStatus `json:"status"`
	TransactionID string        `json:"transaction_id"`
	CreatedAt     string        `json:"created_at"`
}

type Notification struct {
	ID          string `json:"id"`
	ViolationID string `json:"violation_id"`
	UserID      string `json:"user_id"`
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type ViolationInput struct {
	Plate         string        `json:"plate"`
	ViolationType  ViolationType `json:"violation_type"`
	Location       string        `json:"location"`
	OccurredAt     string        `json:"occurred_at"`
	PhotoData      string        `json:"photo_data"`
}

type RuleInput struct {
	BaseAmounts      map[ViolationType]int64 `json:"base_amounts"`
	TimeWindows      []TimeWindow            `json:"time_windows"`
	RepeatMultipliers RepeatMultipliers       `json:"repeat_multipliers"`
}

func DefaultFineRules() FineRules {
	return FineRules{
		BaseAmounts: map[ViolationType]int64{
			ViolationExpiredMeter:    50000,
			ViolationNoParkingZone:   150000,
			ViolationBlockingHydrant: 250000,
			ViolationDisabledSpot:    500000,
		},
		TimeWindows: []TimeWindow{
			{Start: "06:00", End: "22:00", Multiplier: 1.0},
			{Start: "22:00", End: "06:00", Multiplier: 1.5},
		},
		RepeatMultipliers: RepeatMultipliers{Zero: 1.0, One: 1.5, TwoPlus: 2.0},
	}
}

func (r FineRules) Validate() error {
	for _, violationType := range []ViolationType{ViolationExpiredMeter, ViolationNoParkingZone, ViolationBlockingHydrant, ViolationDisabledSpot} {
		if _, ok := r.BaseAmounts[violationType]; !ok {
			return fmt.Errorf("missing base amount for %s", violationType)
		}
	}
	if len(r.TimeWindows) == 0 {
		return fmt.Errorf("at least one time window is required")
	}
	return nil
}

func (r FineRules) BaseAmountFor(violationType ViolationType) (int64, bool) {
	amount, ok := r.BaseAmounts[violationType]
	return amount, ok
}

func (r FineRules) TimeMultiplierFor(ts time.Time) float64 {
	localTime := ts.In(time.FixedZone("WIB", 7*3600))
	totalMinutes := localTime.Hour()*60 + localTime.Minute()
	for _, window := range r.TimeWindows {
		startMinutes, ok := minutesFromClock(window.Start)
		if !ok {
			continue
		}
		endMinutes, ok := minutesFromClock(window.End)
		if !ok {
			continue
		}
		if startMinutes <= endMinutes {
			if totalMinutes >= startMinutes && totalMinutes < endMinutes {
				return window.Multiplier
			}
			continue
		}
		if totalMinutes >= startMinutes || totalMinutes < endMinutes {
			return window.Multiplier
		}
	}
	return 1.0
}

func (r FineRules) RepeatMultiplierFor(priorUnpaidCount int) float64 {
	switch {
	case priorUnpaidCount <= 0:
		return r.RepeatMultipliers.Zero
	case priorUnpaidCount == 1:
		return r.RepeatMultipliers.One
	default:
		return r.RepeatMultipliers.TwoPlus
	}
}

func CalculateFine(rules FineRules, violationType ViolationType, occurredAt time.Time, priorUnpaidCount int) (int64, float64, float64, error) {
	baseAmount, ok := rules.BaseAmountFor(violationType)
	if !ok {
		return 0, 0, 0, fmt.Errorf("unknown violation type %s", violationType)
	}
	timeMultiplier := rules.TimeMultiplierFor(occurredAt)
	repeatMultiplier := rules.RepeatMultiplierFor(priorUnpaidCount)
	fine := float64(baseAmount) * timeMultiplier * repeatMultiplier
	return int64(math.Round(fine)), timeMultiplier, repeatMultiplier, nil
}

func minutesFromClock(value string) (int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func encodeJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func decodeJSON(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
