package runtime

import (
	"sync"
	"sync/atomic"
)

type RequestProfile struct {
	Headers    map[string]string `json:"headers"`
	Reasoning  *ReasoningProfile `json:"reasoning,omitempty"`
	CapturedAt string            `json:"captured_at,omitempty"`
}

type ReasoningProfile struct {
	Effort string `json:"effort"`
}

const MaxLogEntries = 100

type LogEntry struct {
	Seq     int64  `json:"seq"`
	At      string `json:"at"`
	Message string `json:"message"`
}

type Metrics struct {
	totalProxyRequests  atomic.Int64
	inspectedResponses  atomic.Int64
	matchedResponses    atomic.Int64
	blockedResponses    atomic.Int64
	matchedStreaming    atomic.Int64
	matchedNonStreaming atomic.Int64
	blockedStreaming    atomic.Int64
	blockedNonStreaming atomic.Int64
	nextLogSeq          atomic.Int64
	mu                  sync.Mutex
	logEntries          []LogEntry
	requestProfile      RequestProfile
}

type Snapshot struct {
	TotalProxyRequestCount   int64          `json:"total_proxy_request_count"`
	InspectedResponseCount   int64          `json:"inspected_response_count"`
	MatchedResponseCount     int64          `json:"matched_response_count"`
	BlockedResponseCount     int64          `json:"blocked_response_count"`
	MatchedStreamingCount    int64          `json:"matched_streaming_count"`
	MatchedNonStreamingCount int64          `json:"matched_non_streaming_count"`
	BlockedStreamingCount    int64          `json:"blocked_streaming_count"`
	BlockedNonStreamingCount int64          `json:"blocked_non_streaming_count"`
	Logs                     []LogEntry     `json:"logs,omitempty"`
	RequestProfile           RequestProfile `json:"request_profile"`
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.Lock()
	logs := append([]LogEntry(nil), m.logEntries...)
	profile := cloneRequestProfile(m.requestProfile)
	m.mu.Unlock()
	return Snapshot{
		TotalProxyRequestCount:   m.totalProxyRequests.Load(),
		InspectedResponseCount:   m.inspectedResponses.Load(),
		MatchedResponseCount:     m.matchedResponses.Load(),
		BlockedResponseCount:     m.blockedResponses.Load(),
		MatchedStreamingCount:    m.matchedStreaming.Load(),
		MatchedNonStreamingCount: m.matchedNonStreaming.Load(),
		BlockedStreamingCount:    m.blockedStreaming.Load(),
		BlockedNonStreamingCount: m.blockedNonStreaming.Load(),
		Logs:                     logs,
		RequestProfile:           profile,
	}
}

func (m *Metrics) RecordProxyAttempt() {
	if m == nil {
		return
	}
	m.totalProxyRequests.Add(1)
}

func (m *Metrics) RecordInspectedResponse(matched bool, stream bool) {
	if m == nil {
		return
	}
	m.inspectedResponses.Add(1)
	if !matched {
		return
	}
	m.matchedResponses.Add(1)
	if stream {
		m.matchedStreaming.Add(1)
		return
	}
	m.matchedNonStreaming.Add(1)
}

func (m *Metrics) RecordBlockedResponse(stream bool) {
	if m == nil {
		return
	}
	m.blockedResponses.Add(1)
	if stream {
		m.blockedStreaming.Add(1)
		return
	}
	m.blockedNonStreaming.Add(1)
}

func (m *Metrics) Reset() {
	if m == nil {
		return
	}
	m.totalProxyRequests.Store(0)
	m.inspectedResponses.Store(0)
	m.matchedResponses.Store(0)
	m.blockedResponses.Store(0)
	m.matchedStreaming.Store(0)
	m.matchedNonStreaming.Store(0)
	m.blockedStreaming.Store(0)
	m.blockedNonStreaming.Store(0)
	m.nextLogSeq.Store(0)
	m.mu.Lock()
	m.logEntries = nil
	m.requestProfile = RequestProfile{}
	m.mu.Unlock()
}

func (m *Metrics) AppendLog(at string, message string) LogEntry {
	if m == nil {
		return LogEntry{At: at, Message: message}
	}
	entry := LogEntry{
		Seq:     m.nextLogSeq.Add(1),
		At:      at,
		Message: message,
	}
	m.mu.Lock()
	m.logEntries = append(m.logEntries, entry)
	if len(m.logEntries) > MaxLogEntries {
		m.logEntries = append([]LogEntry(nil), m.logEntries[len(m.logEntries)-MaxLogEntries:]...)
	}
	m.mu.Unlock()
	return entry
}

func (m *Metrics) SetRequestProfile(profile RequestProfile) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.requestProfile = cloneRequestProfile(profile)
	m.mu.Unlock()
}

func cloneRequestProfile(profile RequestProfile) RequestProfile {
	cloned := profile
	if profile.Headers != nil {
		cloned.Headers = make(map[string]string, len(profile.Headers))
		for key, value := range profile.Headers {
			cloned.Headers[key] = value
		}
	}
	if profile.Reasoning != nil {
		copied := *profile.Reasoning
		cloned.Reasoning = &copied
	}
	return cloned
}
