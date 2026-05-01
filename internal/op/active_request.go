package op

import (
	"sort"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/utils/snowflake"
)

// ActiveRequestStatus 活跃请求状态
type ActiveRequestStatus string

const (
	ActiveRequestForwarding      ActiveRequestStatus = "forwarding"         // 转发中
	ActiveRequestWaitingFirstTok ActiveRequestStatus = "waiting_first_token" // 等待首Token
	ActiveRequestStreaming       ActiveRequestStatus = "streaming"          // 流式传输中
	ActiveRequestProcessing      ActiveRequestStatus = "processing"        // 非流式请求处理中
)

// ActiveRequest 活跃请求信息
type ActiveRequest struct {
	ID               int64               `json:"id"`
	StartTime       int64               `json:"start_time"`        // 开始时间戳（秒）
	RequestModel    string              `json:"request_model"`     // 请求模型名称
	APIKeyName      string              `json:"api_key_name"`      // API Key 名称
	Status          ActiveRequestStatus `json:"status"`            // 当前状态
	ChannelName     string              `json:"channel_name"`      // 当前渠道名称
	AttemptCount    int                 `json:"attempt_count"`     // 尝试次数
}

var (
	activeRequests     = make(map[int64]*ActiveRequest)
	activeRequestsLock sync.RWMutex
)

// ActiveRequestRegister 注册一个活跃请求
func ActiveRequestRegister(requestModel, apiKeyName string) int64 {
	id := snowflake.GenerateID()
	req := &ActiveRequest{
		ID:            id,
		StartTime:    time.Now().Unix(),
		RequestModel: requestModel,
		APIKeyName:   apiKeyName,
		Status:       ActiveRequestForwarding,
		AttemptCount: 1,
	}

	activeRequestsLock.Lock()
	activeRequests[id] = req
	activeRequestsLock.Unlock()

	notifyActiveRequestEvent("active_register", req)
	return id
}

// ActiveRequestUpdateStatus 更新活跃请求状态
func ActiveRequestUpdateStatus(id int64, status ActiveRequestStatus) {
	activeRequestsLock.Lock()
	req, ok := activeRequests[id]
	if !ok {
		activeRequestsLock.Unlock()
		return
	}
	req.Status = status
	activeRequestsLock.Unlock()

	notifyActiveRequestEvent("active_update", req)
}

// ActiveRequestUpdateChannel 更新活跃请求的渠道信息
func ActiveRequestUpdateChannel(id int64, channelName string, attemptCount int) {
	activeRequestsLock.Lock()
	req, ok := activeRequests[id]
	if !ok {
		activeRequestsLock.Unlock()
		return
	}
	req.ChannelName = channelName
	req.AttemptCount = attemptCount
	activeRequestsLock.Unlock()

	notifyActiveRequestEvent("active_update", req)
}

// ActiveRequestUnregister 注销活跃请求
func ActiveRequestUnregister(id int64) {
	activeRequestsLock.Lock()
	req, ok := activeRequests[id]
	if !ok {
		activeRequestsLock.Unlock()
		return
	}
	delete(activeRequests, id)
	activeRequestsLock.Unlock()

	notifyActiveRequestEvent("active_complete", req)
}

// ActiveRequestList 返回当前所有活跃请求快照（按开始时间降序）
func ActiveRequestList() []ActiveRequest {
	activeRequestsLock.RLock()
	defer activeRequestsLock.RUnlock()

	result := make([]ActiveRequest, 0, len(activeRequests))
	for _, req := range activeRequests {
		result = append(result, *req)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime > result[j].StartTime
	})
	return result
}

// ActiveRequestCount 返回当前活跃请求数量
func ActiveRequestCount() int {
	activeRequestsLock.RLock()
	defer activeRequestsLock.RUnlock()
	return len(activeRequests)
}

// --- SSE 事件推送 ---

var activeRequestSubscribers = make(map[chan ActiveRequestEvent]struct{})
var activeRequestSubscribersLock sync.RWMutex

// ActiveRequestEvent SSE 事件
type ActiveRequestEvent struct {
	Type    string         `json:"type"`    // active_register / active_update / active_complete
	Request ActiveRequest  `json:"request"`
}

func notifyActiveRequestEvent(eventType string, req *ActiveRequest) {
	event := ActiveRequestEvent{
		Type:    eventType,
		Request: *req,
	}

	activeRequestSubscribersLock.RLock()
	defer activeRequestSubscribersLock.RUnlock()

	for ch := range activeRequestSubscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// ActiveRequestSubscribe 订阅活跃请求事件
func ActiveRequestSubscribe() chan ActiveRequestEvent {
	ch := make(chan ActiveRequestEvent, 64)
	activeRequestSubscribersLock.Lock()
	activeRequestSubscribers[ch] = struct{}{}
	activeRequestSubscribersLock.Unlock()
	return ch
}

// ActiveRequestUnsubscribe 取消订阅活跃请求事件
func ActiveRequestUnsubscribe(ch chan ActiveRequestEvent) {
	activeRequestSubscribersLock.Lock()
	delete(activeRequestSubscribers, ch)
	activeRequestSubscribersLock.Unlock()
	close(ch)
}
