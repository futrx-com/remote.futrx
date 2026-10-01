package project

import "context"

// PersistentStorage reports host bind-mounted data separately from LXD root.
// Bytes is absent until a complete sample succeeds; failures never become zero.
type PersistentQuota struct {
	Driver     string  `json:"driver,omitempty"`
	Required   bool    `json:"required"`
	Enforced   bool    `json:"enforced"`
	LimitBytes *uint64 `json:"limitBytes,omitempty"`
	Detail     string  `json:"detail,omitempty"`
}

type PersistentStorage struct {
	Quota          *PersistentQuota `json:"quota,omitempty"`
	Bytes          *uint64          `json:"bytes,omitempty"`
	SampledAt      int64            `json:"sampledAt,omitempty"`
	Pending        bool             `json:"pending"`
	Error          string           `json:"error,omitempty"`
	AvailableBytes *uint64          `json:"availableBytes,omitempty"`
	UsagePercent   *float64         `json:"usagePercent,omitempty"`
	InodePercent   *float64         `json:"inodePercent,omitempty"`
	Warning        bool             `json:"warning"`
}
type StorageReader interface {
	Read(context.Context, string) PersistentStorage
}
type DiskQuotaInfo struct {
	Pool      string `json:"pool,omitempty"`
	Driver    string `json:"driver,omitempty"`
	Supported bool   `json:"supported"`
	Detail    string `json:"detail,omitempty"`
}
