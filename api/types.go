package api

import "time"

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

// ModelStatus represents the lifecycle state of a model.
type ModelStatus string

const (
	ModelReady     ModelStatus = "ready"
	ModelStarting  ModelStatus = "starting"
	ModelStopping  ModelStatus = "stopping"
	ModelStopped   ModelStatus = "stopped"
	ModelShutdown  ModelStatus = "shutdown"
	ModelUnknown   ModelStatus = "unknown"
)

// ModelCapabilities describes what modalities a model supports.
type ModelCapabilities struct {
	Vision          bool `json:"vision,omitempty"`
	AudioTranscriptions bool `json:"audio_transcriptions,omitempty"`
	AudioSpeech     bool `json:"audio_speech,omitempty"`
	ImageGeneration bool `json:"image_generation,omitempty"`
	ImageToImage    bool `json:"image_to_image,omitempty"`
	FunctionCalling bool `json:"function_calling,omitempty"`
	Reranker        bool `json:"reranker,omitempty"`
}

// Model represents a configured model with its current state.
type Model struct {
	ID            string              `json:"id"`
	State         ModelStatus         `json:"state"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Unlisted      bool                `json:"unlisted"`
	PeerID        string              `json:"peerID"`
	Aliases       []string            `json:"aliases,omitempty"`
	Capabilities  *ModelCapabilities  `json:"capabilities,omitempty"`
	ContextLength int                 `json:"context_length,omitempty"`
	Strategy      string              `json:"strategy,omitempty"`
	Targets       []string            `json:"targets,omitempty"`
	Spillover     int                 `json:"spillover,omitempty"`
}

// ---------------------------------------------------------------------------
// Profile types
// ---------------------------------------------------------------------------

// Profile represents a named configuration profile.
type Profile struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Pins        map[string]string `json:"pins"`
}

// ProfileState is the response from GET /api/profiles.
type ProfileState struct {
	Active   string    `json:"active"`
	Profiles []Profile `json:"profiles"`
}

// ---------------------------------------------------------------------------
// Activity / metrics types
// ---------------------------------------------------------------------------

// TokenMetrics holds token usage and performance metrics for a single request.
type TokenMetrics struct {
	CachedTokens    int     `json:"cache_tokens"`
	DraftTokens     int     `json:"draft_tokens"`
	DraftAccTokens  int     `json:"draft_acc_tokens"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	PromptPerSecond float64 `json:"prompt_per_second"`
	TokensPerSecond float64 `json:"tokens_per_second"`
}

// ActivityLogEntry is a single row in the activity log.
type ActivityLogEntry struct {
	ID              int           `json:"id"`
	Timestamp       time.Time     `json:"timestamp"`
	Src             string        `json:"src"`
	Model           string        `json:"model"`
	ReqPath         string        `json:"req_path"`
	RespContentType string        `json:"resp_content_type"`
	RespStatusCode  int           `json:"resp_status_code"`
	Tokens          TokenMetrics  `json:"tokens"`
	DurationMs      int           `json:"duration_ms"`
	HasCapture      bool          `json:"has_capture"`
	ErrorMsg        string        `json:"error_msg,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ActivityPage is a paginated response from GET /api/metrics/activity.
type ActivityPage struct {
	Data       []ActivityLogEntry `json:"data"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	Total      int                `json:"total"`
	TotalPages int                `json:"total_pages"`
}

// ActivityStats is the response from GET /api/metrics/stats.
type ActivityStats struct {
	TotalRequests     int            `json:"total_requests"`
	TotalInputTokens  int            `json:"total_input_tokens"`
	TotalOutputTokens int            `json:"total_output_tokens"`
	TotalCacheTokens  int            `json:"total_cache_tokens"`
	PromptHistogram   *HistogramData `json:"prompt_histogram"`
	GenerationHistogram *HistogramData `json:"gen_histogram"`
}

// HistogramData for token throughput distributions.
type HistogramData struct {
	Bins    []int   `json:"bins"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	BinSize float64 `json:"binSize"`
	P99     float64 `json:"p99"`
	P95     float64 `json:"p95"`
	P50     float64 `json:"p50"`
}

// ---------------------------------------------------------------------------
// In-flight request types
// ---------------------------------------------------------------------------

// InflightRequestEntry represents a request currently being processed.
type InflightRequestEntry struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Model           string            `json:"model"`
	ReqPath         string            `json:"req_path"`
	Method          string            `json:"method"`
	ReqHeaders      map[string]string `json:"req_headers"`
	RemoteIP        string            `json:"remote_ip"`
	RespHeaders     map[string]string `json:"resp_headers"`
	RespBytes       int64             `json:"resp_bytes"`
	ElapsedMs       int64             `json:"elapsed_ms"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	ClientReceivedAt int64            `json:"client_received_at_ms,omitempty"`
}

// InFlightStats describes an SSE inflight event.
type InFlightStats struct {
	Operation string                  `json:"operation"` // "snapshot", "upsert", "remove"
	Requests  []InflightRequestEntry  `json:"requests,omitempty"`
	Request   *InflightRequestEntry   `json:"request,omitempty"`
	ID        string                  `json:"id,omitempty"`
}

// ---------------------------------------------------------------------------
// Hardware / performance types
// ---------------------------------------------------------------------------

// HardwareSnapshot is the response from GET /api/hardware.
type HardwareSnapshot struct {
	SchemaVersion  int                 `json:"schema_version"`
	CapturedAt     time.Time           `json:"captured_at"`
	Capture        HardwareCapture     `json:"capture"`
	Architecture   HardwareArchitecture `json:"architecture"`
	OperatingSystem HardwareOperatingSystem `json:"operating_system"`
	Environment    HardwareEnvironment   `json:"environment"`
	CPU            HardwareCPU         `json:"cpu"`
	Memory         HardwareMemory      `json:"memory"`
	Accelerators   []HardwareAccelerator `json:"accelerators"`
}

type HardwareCapture struct {
	Scope    string                                  `json:"scope"`
	Method   string                                  `json:"method"`
	Detector *struct{ Name string; Version string }   `json:"detector"`
}

type HardwareArchitecture struct {
	Name     string `json:"name"`
	RawName  *string `json:"raw_name,omitempty"`
}

type HardwareOperatingSystem struct {
	Family    string  `json:"family"`
	Name      *string `json:"name,omitempty"`
	Version   *string `json:"version,omitempty"`
	Kernel    *string `json:"kernel,omitempty"`
	RawFamily *string `json:"raw_family,omitempty"`
}

type HardwareEnvironment struct {
	Kind    string  `json:"kind"`
	Name    *string `json:"name,omitempty"`
	Version *string `json:"version,omitempty"`
	RawKind *string `json:"raw_kind,omitempty"`
}

type HardwareCPU struct {
	Vendor           *string `json:"vendor,omitempty"`
	Model            *string `json:"model,omitempty"`
	SocketCount      *int    `json:"socket_count,omitempty"`
	PhysicalCoreCount *int   `json:"physical_core_count,omitempty"`
	LogicalThreadCount *int   `json:"logical_thread_count,omitempty"`
}

type HardwareMemory struct {
	CapacityBytes int64 `json:"capacity_bytes"`
}

type HardwareAccelerator struct {
	Index          int                  `json:"index"`
	Kind           string               `json:"kind"` // "gpu", "npu", "other"
	Vendor         *string              `json:"vendor,omitempty"`
	Model          *string              `json:"model,omitempty"`
	Architecture   *string              `json:"architecture,omitempty"`
	Memory         *HardwareMemInfo     `json:"memory,omitempty"`
	Driver         *HardwareDriver      `json:"driver,omitempty"`
	PowerLimitWatts *int               `json:"power_limit_watts,omitempty"`
}

type HardwareMemInfo struct {
	Kind          string  `json:"kind"` // "dedicated", "unified", "shared_system", "unknown"
	CapacityBytes *int64  `json:"capacity_bytes,omitempty"`
}

type HardwareDriver struct {
	Name    *string `json:"name,omitempty"`
	Version *string `json:"version,omitempty"`
}

// SysStat is a system performance snapshot from SSE perfsys.
type SysStat struct {
	Timestamp      string    `json:"timestamp"`
	CPUUtilPerCore []float64 `json:"cpu_util_per_core"`
	MemTotalMB     int       `json:"mem_total_mb"`
	MemUsedMB      int       `json:"mem_used_mb"`
	MemFreeMB      int       `json:"mem_free_mb"`
	SwapTotalMB    int       `json:"swap_total_mb"`
	SwapUsedMB     int       `json:"swap_used_mb"`
	LoadAvg1       float64   `json:"load_avg_1"`
	LoadAvg5       float64   `json:"load_avg_5"`
	LoadAvg15      float64   `json:"load_avg_15"`
	NetIO          []NetIOStat `json:"net_io"`
}

// GpuStat is a GPU performance snapshot from SSE perfgpu.
type GpuStat struct {
	Timestamp     string  `json:"timestamp"`
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	UUID          string  `json:"uuid"`
	TempC         int     `json:"temp_c"`
	VramTempC     int     `json:"vram_temp_c"`
	GpuUtilPct    float64 `json:"gpu_util_pct"`
	MemUtilPct    float64 `json:"mem_util_pct"`
	MemUsedMB     int     `json:"mem_used_mb"`
	MemTotalMB    int     `json:"mem_total_mb"`
	FanSpeedPct   float64 `json:"fan_speed_pct"`
	PowerDrawW    float64 `json:"power_draw_w"`
}

// PerformanceResponse from GET /api/performance.
type PerformanceResponse struct {
	SysStats []SysStat `json:"sys_stats"`
	GpuStats []GpuStat `json:"gpu_stats"`
}

// NetIOStat is a network I/O stat.
type NetIOStat struct {
	Name       string `json:"name"`
	BytesRecv  int64  `json:"bytes_recv"`
	BytesSent  int64  `json:"bytes_sent"`
}

// ---------------------------------------------------------------------------
// Version / config types
// ---------------------------------------------------------------------------

// VersionInfo is the response from GET /api/version.
type VersionInfo struct {
	BuildDate string `json:"build_date"`
	Commit    string `json:"commit"`
	Version   string `json:"version"`
}

// UIConfig is the response from SSE uiConfig event.
type UIConfig struct {
	Activity struct {
		SessionID []string `json:"session_id"`
	} `json:"activity"`
}

// ---------------------------------------------------------------------------
// SSE event types
// ---------------------------------------------------------------------------

// APIEventEnvelope is the JSON structure of each SSE message from /api/events.
type APIEventEnvelope struct {
	Type string `json:"type"` // "modelStatus", "logData", "activity", "inflight", "uiConfig", "profileChanged", "perfsys", "perfgpu"
	Data string `json:"data"`
}

// LogDataEnvelope for logData SSE events.
type LogDataEnvelope struct {
	Source string `json:"source"` // "proxy" or "upstream"
	Data   string `json:"data"`
}

// ActivityEventEnvelope for activity SSE events.
type ActivityEventEnvelope struct {
	ID int `json:"id"`
}

// TailcatStatus from GET /api/tailcat.
type TailcatStatus struct {
	Enabled bool     `json:"enabled"`
	Address string   `json:"address"`
	Models  []string `json:"models"`
}

// ---------------------------------------------------------------------------
// Capture types
// ---------------------------------------------------------------------------

// ReqRespCapture is a captured request/response pair.
type ReqRespCapture struct {
	ID          int               `json:"id"`
	ReqPath     string            `json:"req_path"`
	ReqHeaders  map[string]string `json:"req_headers"`
	ReqBody     string            `json:"req_body"` // base64 encoded
	RespHeaders map[string]string `json:"resp_headers"`
	RespBody    string            `json:"resp_body"` // base64 encoded
}
