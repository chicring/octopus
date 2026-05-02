package model

// StatsRealtime 实时监控指标（基于滑动窗口，不持久化）
type StatsRealtime struct {
	WindowSizeSec int     `json:"window_size_sec"`
	RPS           float64 `json:"rps"`
	RPM           int64   `json:"rpm"`
	TPS           float64 `json:"tps"`
	TPM           int64   `json:"tpm"`
}
