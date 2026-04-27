package model

type StatsAPIKeyDaily struct {
	APIKeyID uint   `json:"api_key_id" gorm:"primaryKey;not null"` // API Key ID
	Date     string `json:"date" gorm:"primaryKey;not null"`       // 日期，格式：20060102
	StatsMetrics
}

func (StatsAPIKeyDaily) TableName() string { return "stats_api_key_dailies" }

type StatsMetrics struct {
	InputToken     int64   `json:"input_token" gorm:"bigint"`
	OutputToken    int64   `json:"output_token" gorm:"bigint"`
	InputCost      float64 `json:"input_cost" gorm:"type:real"`
	OutputCost     float64 `json:"output_cost" gorm:"type:real"`
	WaitTime       int64   `json:"wait_time" gorm:"bigint"`
	OutputTime     int64   `json:"output_time" gorm:"bigint"`
	RequestSuccess int64   `json:"request_success" gorm:"bigint"`
	RequestFailed  int64   `json:"request_failed" gorm:"bigint"`
}

type StatsTotal struct {
	ID int `gorm:"primaryKey"`
	StatsMetrics
}

type StatsHourly struct {
	Date string `json:"date" gorm:"primaryKey;not null"` // 日期，格式：20060102
	Hour int    `json:"hour" gorm:"primaryKey;not null"` // 小时 0-23
	StatsMetrics
}

type StatsDaily struct {
	Date string `json:"date" gorm:"primaryKey"`
	StatsMetrics
}

type StatsModel struct {
	Name string `json:"name" gorm:"primaryKey;not null"`
	StatsMetrics
}

type StatsChannel struct {
	ChannelID int `json:"channel_id" gorm:"primaryKey"`
	StatsMetrics
}

type StatsAPIKey struct {
	APIKeyID int `json:"api_key_id" gorm:"primaryKey"`
	StatsMetrics
}

// Add aggregates another StatsMetrics into the current one.
func (s *StatsMetrics) Add(delta StatsMetrics) {
	s.InputToken += delta.InputToken
	s.OutputToken += delta.OutputToken
	s.InputCost += delta.InputCost
	s.OutputCost += delta.OutputCost
	s.WaitTime += delta.WaitTime
	s.OutputTime += delta.OutputTime
	s.RequestSuccess += delta.RequestSuccess
	s.RequestFailed += delta.RequestFailed
}
