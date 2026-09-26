package team

import "time"

type CreateRequest struct {
	Name string `json:"name" binding:"required"`
}

type UpdateRequest struct {
	Name string `json:"name" binding:"required"`
	// QuotaVisible, when set, changes whether the team's sharing keys may read
	// the remaining quota of the team's models. Omitted leaves it unchanged.
	QuotaVisible *bool `json:"quota_visible,omitempty"`
}

type TeamInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Enabled bool   `json:"enabled"`
	// QuotaVisible reports whether the team's sharing keys may read quota.
	QuotaVisible bool      `json:"quota_visible"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ListResponse struct {
	Teams []TeamInfo `json:"teams"`
}
