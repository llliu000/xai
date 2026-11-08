package mock

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/dto"
)

const (
	Success          = "mock_success"
	Slow             = "mock_slow"
	Retry            = "mock_retry"
	ModerationFailed = "mock_moderation_failed"
	Unavailable      = "mock_unavailable"
	MaxConcurrency   = 100
)

type Config struct {
	DelayMS int `json:"delay_ms"`
}

type Error struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("mock upstream HTTP %d: %s: %s", e.StatusCode, e.Code, e.Message)
}

type Stats struct {
	Requests       int
	InFlight       int
	PeakInFlight   int
	Rejected       int
	GroupCreates   int
	UploadAttempts int
	EmptyIDs       int
	StatusAttempts int
	UploadsByURL   map[string]int
	AssetsByStatus map[string]int
}

type storedAsset struct {
	dto.GetAssetResponse
	polls         int
	statusRetries int
}

type channelState struct {
	mu            sync.Mutex
	slots         chan struct{}
	stats         Stats
	groups        []dto.GetAssetGroupResponse
	assets        []*storedAsset
	attempts      map[string]int
	beforeRequest func(string)
}

var channels = struct {
	sync.Mutex
	states map[int]*channelState
}{states: make(map[int]*channelState)}

type Adaptor struct {
	channelID int
	scenario  string
	config    Config
	state     *channelState
}

// Each channel keeps its own resources and 100-call limit across GetAdaptor
// calls, just as successive requests would reach the same upstream account.
func New(channelID int, scenario string, config Config) *Adaptor {
	channels.Lock()
	defer channels.Unlock()
	state := channels.states[channelID]
	if state == nil {
		state = &channelState{
			slots: make(chan struct{}, MaxConcurrency), attempts: make(map[string]int),
			stats: Stats{UploadsByURL: make(map[string]int)},
		}
		channels.states[channelID] = state
	}
	return &Adaptor{channelID: channelID, scenario: scenario, config: config, state: state}
}

// Reset clears a mock channel between independent tests.
func Reset(channelID int) {
	channels.Lock()
	defer channels.Unlock()
	delete(channels.states, channelID)
}

func (a *Adaptor) Stats() Stats {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	stats := a.state.stats
	stats.UploadsByURL = maps.Clone(stats.UploadsByURL)
	stats.AssetsByStatus = make(map[string]int)
	for _, asset := range a.state.assets {
		stats.AssetsByStatus[asset.Status]++
	}
	return stats
}

// SetBeforeRequest lets tests hold admitted calls at a deterministic barrier.
func (a *Adaptor) SetBeforeRequest(hook func(string)) {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	a.state.beforeRequest = hook
}

func (a *Adaptor) begin(action string) error {
	a.state.mu.Lock()
	a.state.stats.Requests++
	select {
	case a.state.slots <- struct{}{}:
		a.state.stats.InFlight++
		a.state.stats.PeakInFlight = max(a.state.stats.PeakInFlight, a.state.stats.InFlight)
	default:
		a.state.stats.Rejected++
		a.state.mu.Unlock()
		return &Error{StatusCode: http.StatusTooManyRequests, Code: "TooManyRequests", Message: "Mock upstream concurrency limit reached"}
	}
	hook := a.state.beforeRequest
	a.state.mu.Unlock()
	if hook != nil {
		hook(action)
	}
	if a.config.DelayMS > 0 {
		time.Sleep(time.Duration(a.config.DelayMS) * time.Millisecond)
	}
	return nil
}

func (a *Adaptor) end() {
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	a.state.stats.InFlight--
	<-a.state.slots
}

func (a *Adaptor) ReviewSkip() bool { return false }

func (a *Adaptor) CreateAssetGroup(req *dto.CreateAssetGroupRequest) (*dto.CreateAssetGroupResponse, error) {
	if err := a.begin("CreateAssetGroup"); err != nil {
		return nil, err
	}
	defer a.end()
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	if req.GroupType != "AIGC" {
		return nil, &Error{StatusCode: http.StatusBadRequest, Code: "InvalidRequest", Message: "Only AIGC groups are supported"}
	}
	a.state.stats.GroupCreates++
	now := time.Now().UTC().Format(time.RFC3339)
	group := dto.GetAssetGroupResponse{
		Id: fmt.Sprintf("mock-%d-group-%d", a.channelID, a.state.stats.GroupCreates), Name: req.Name,
		GroupType: req.GroupType, ProjectName: "default", CreateTime: now, UpdateTime: now,
	}
	a.state.groups = append(a.state.groups, group)
	return &dto.CreateAssetGroupResponse{Id: group.Id}, nil
}

func (a *Adaptor) CreateAssets(req *dto.CreateAssetRequest) (*dto.CreateAssetResponse, error) {
	if err := a.begin("CreateAsset"); err != nil {
		return nil, err
	}
	defer a.end()
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	if !slices.ContainsFunc(a.state.groups, func(group dto.GetAssetGroupResponse) bool { return group.Id == req.GroupId }) {
		return nil, &Error{StatusCode: http.StatusBadRequest, Code: "InvalidRequest", Message: "Unknown asset group"}
	}
	a.state.stats.UploadAttempts++
	key := req.GroupId + "\x00" + req.Name + "\x00" + req.URL
	a.state.attempts[key]++
	attempt := a.state.attempts[key]
	if a.scenario == Unavailable || (a.scenario == Retry && attempt == 1) {
		return nil, &Error{StatusCode: http.StatusServiceUnavailable, Code: "ServiceUnavailable", Message: "Mock temporary upload failure"}
	}
	if a.scenario == Retry && attempt == 2 {
		a.state.stats.EmptyIDs++
		return &dto.CreateAssetResponse{}, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	asset := &storedAsset{GetAssetResponse: dto.GetAssetResponse{
		Id: fmt.Sprintf("mock-%d-asset-%d", a.channelID, len(a.state.assets)+1), GroupId: req.GroupId,
		Name: req.Name, URL: req.URL, AssetType: req.AssetType, ProjectName: "default",
		Status: "Processing", CreateTime: now, UpdateTime: now,
	}}
	a.state.assets = append(a.state.assets, asset)
	a.state.stats.UploadsByURL[req.URL]++
	return &dto.CreateAssetResponse{Id: asset.Id}, nil
}

func (a *Adaptor) GetAsset(req *dto.GetAssetRequest) (*dto.GetAssetResponse, error) {
	if err := a.begin("GetAsset"); err != nil {
		return nil, err
	}
	defer a.end()
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	index := slices.IndexFunc(a.state.assets, func(asset *storedAsset) bool { return asset.Id == req.Id })
	if index < 0 {
		return nil, &Error{StatusCode: http.StatusNotFound, Code: "AssetNotFound", Message: "Unknown asset"}
	}
	a.state.stats.StatusAttempts++
	asset := a.state.assets[index]
	asset.statusRetries++
	if a.scenario == Retry && asset.statusRetries == 1 {
		return nil, &Error{StatusCode: http.StatusServiceUnavailable, Code: "ServiceUnavailable", Message: "Mock temporary status failure"}
	}
	asset.polls++
	if a.scenario != Slow || asset.polls > 2 {
		asset.Status = "Active"
		if a.scenario == ModerationFailed {
			asset.Status = "Failed"
			asset.Error = &dto.AssetError{Code: "ModerationFailed", Message: "Mock moderation rejected the asset"}
		}
	}
	asset.UpdateTime = time.Now().UTC().Format(time.RFC3339)
	result := asset.GetAssetResponse
	if result.Error != nil {
		result.Error = new(*result.Error)
	}
	return &result, nil
}

func (a *Adaptor) ListAssets(req *dto.ListAssetsRequest) (*dto.ListAssetsResponse, error) {
	if err := a.begin("ListAssets"); err != nil {
		return nil, err
	}
	defer a.end()
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	items := make([]dto.GetAssetResponse, 0, len(a.state.assets))
	for _, asset := range a.state.assets {
		if f := req.Filter; f != nil && ((len(f.GroupIds) > 0 && !slices.Contains(f.GroupIds, asset.GroupId)) ||
			(len(f.Statuses) > 0 && !slices.Contains(f.Statuses, asset.Status)) || (f.Name != nil && *f.Name != asset.Name)) {
			continue
		}
		item := asset.GetAssetResponse
		if item.Error != nil {
			item.Error = new(*item.Error)
		}
		items = append(items, item)
	}
	page, size := max(req.PageNumber, 1), min(max(req.PageSize, 1), 100)
	start := min((page-1)*size, len(items))
	return &dto.ListAssetsResponse{Items: items[start:min(start+size, len(items))], TotalCount: int64(len(items)), PageNumber: page, PageSize: size}, nil
}

func (a *Adaptor) ListAssetGroups(req *dto.ListAssetGroupsRequest) (*dto.ListAssetGroupsResponse, error) {
	if err := a.begin("ListAssetGroups"); err != nil {
		return nil, err
	}
	defer a.end()
	a.state.mu.Lock()
	defer a.state.mu.Unlock()
	items := make([]dto.GetAssetGroupResponse, 0, len(a.state.groups))
	for _, group := range a.state.groups {
		if f := req.Filter; f != nil && ((len(f.GroupIds) > 0 && !slices.Contains(f.GroupIds, group.Id)) ||
			(f.GroupType != nil && *f.GroupType != group.GroupType) || (f.Name != nil && *f.Name != group.Name)) {
			continue
		}
		items = append(items, group)
	}
	page, size := max(req.PageNumber, 1), min(max(req.PageSize, 1), 100)
	start := min((page-1)*size, len(items))
	return &dto.ListAssetGroupsResponse{Items: items[start:min(start+size, len(items))], TotalCount: int64(len(items)), PageNumber: page, PageSize: size}, nil
}

func (*Adaptor) CreateVisualValidateSession(*dto.CreateVisualValidateSessionRequest) (*dto.CreateVisualValidateSessionResponse, error) {
	return nil, &Error{StatusCode: http.StatusNotImplemented, Code: "NotImplemented", Message: "Mock human verification is not supported"}
}

func (*Adaptor) GetVisualValidateResult(*dto.GetVisualValidateResultRequest) (*dto.GetVisualValidateResultResponse, error) {
	return nil, &Error{StatusCode: http.StatusNotImplemented, Code: "NotImplemented", Message: "Mock human verification is not supported"}
}
