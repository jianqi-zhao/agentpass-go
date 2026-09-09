package agentpass

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// UsageService reads the user's live weekly allowance, shared across all apps.
type UsageService struct {
	client *Client
}

// UsagePeriod is one calendar quota period. Used credits are settled charges;
// Reserved credits belong to in-flight requests; Remaining accounts for both.
type UsagePeriod struct {
	StartsAt         string  `json:"starts_at"`
	EndsAt           string  `json:"ends_at"`
	Limit            float64 `json:"limit"`
	Used             float64 `json:"used"`
	Reserved         float64 `json:"reserved"`
	Remaining        float64 `json:"remaining"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetsAt         string  `json:"resets_at"`
	CanStartNewWork  bool    `json:"can_start_new_work"`
}

// UsageSummary includes aggregate user allowance, never other app identities.
// Fetch it live: historical settlement receipts are not current authorization.
type UsageSummary struct {
	Object  string `json:"object"`
	Scope   string `json:"scope"`
	GrantID string `json:"grant_id"`
	App     struct {
		ClientID string `json:"client_id"`
		Name     string `json:"name"`
	} `json:"app"`
	Week      UsagePeriod `json:"week"`
	UpdatedAt string      `json:"updated_at"`
}

// Current returns the shared user allowance for the current Monday-based UTC week.
func (service *UsageService) Current(ctx context.Context) (*UsageSummary, error) {
	request, err := service.client.newRequest(ctx, http.MethodGet, "/v1/usage", nil)
	if err != nil {
		return nil, err
	}
	if err := service.client.authorizeRequest(ctx, request); err != nil {
		return nil, err
	}

	summary := &UsageSummary{}
	if err := service.client.do(request, summary); err != nil {
		return nil, err
	}
	if err := validateUsageSummary(summary); err != nil {
		return nil, err
	}
	return summary, nil
}

func validateUsageSummary(summary *UsageSummary) error {
	if summary.Object != "agentpass.usage" || summary.Scope != "user" ||
		strings.TrimSpace(summary.GrantID) == "" ||
		strings.TrimSpace(summary.App.ClientID) == "" ||
		strings.TrimSpace(summary.App.Name) == "" {
		return fmt.Errorf("%w: malformed usage identity", ErrInvalidResponse)
	}
	for _, period := range []UsagePeriod{summary.Week} {
		startsAt, startErr := time.Parse(time.RFC3339Nano, period.StartsAt)
		endsAt, endErr := time.Parse(time.RFC3339Nano, period.EndsAt)
		expectedRemaining := period.Limit - period.Used - period.Reserved
		if expectedRemaining < 0 {
			expectedRemaining = 0
		}
		if startErr != nil || endErr != nil || !startsAt.Before(endsAt) ||
			!validCreditValue(period.Limit, false) ||
			!validCreditValue(period.Used, true) ||
			!validCreditValue(period.Reserved, true) ||
			!validCreditValue(period.Remaining, true) ||
			math.Abs(period.Remaining-expectedRemaining) > 1e-8 ||
			period.ResetsAt != period.EndsAt ||
			math.IsNaN(period.RemainingPercent) || math.IsInf(period.RemainingPercent, 0) ||
			math.Abs(period.RemainingPercent-math.Floor(expectedRemaining/period.Limit*10000)/100) > 1e-8 ||
			period.CanStartNewWork != (period.Remaining > 0) {
			return fmt.Errorf("%w: malformed usage period", ErrInvalidResponse)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, summary.UpdatedAt); err != nil {
		return fmt.Errorf("%w: malformed usage timestamp", ErrInvalidResponse)
	}
	return nil
}
