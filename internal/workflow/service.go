package workflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"shelter-drill-gate/internal/domain"
	"shelter-drill-gate/internal/store"
)

type Service struct {
	store            *store.Store
	now              func() time.Time
	certifiedCacheMu sync.RWMutex
	certifiedCache   map[string]domain.DrillCase
}

var fallbackIDCounter atomic.Uint64

func New(repository *store.Store) *Service {
	return &Service{
		store:          repository,
		now:            time.Now,
		certifiedCache: make(map[string]domain.DrillCase),
	}
}

type Mutation struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}

type DrillView struct {
	Drill            domain.DrillCase           `json:"drill"`
	Todo             []TodoItem                 `json:"todo"`
	ActivationStatus []ActivationEvidenceStatus `json:"activation_status"`
	NextCheckpoint   *domain.Checkpoint         `json:"next_checkpoint,omitempty"`
	LatestReceipt    *domain.JudgmentReceipt    `json:"latest_receipt,omitempty"`
	Replayed         bool                       `json:"replayed"`
}

type TodoItem struct {
	Kind           string `json:"kind"`
	ResourceID     string `json:"resource_id"`
	Label          string `json:"label"`
	Severity       string `json:"severity,omitempty"`
	DueStatus      string `json:"due_status,omitempty"`
	DueAt          string `json:"due_at,omitempty"`
	OwnerID        string `json:"owner_id,omitempty"`
	OverdueSeconds int64  `json:"overdue_seconds,omitempty"`
}

type ActivationEvidenceStatus struct {
	Category   string `json:"category"`
	Label      string `json:"label"`
	Registered bool   `json:"registered"`
	Valid      bool   `json:"valid"`
	ValidUntil string `json:"valid_until,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func (s *Service) Get(ctx context.Context, id string) (DrillView, error) {
	s.certifiedCacheMu.RLock()
	cached, found := s.certifiedCache[id]
	s.certifiedCacheMu.RUnlock()
	if found {
		drill, err := cloneDrillCase(cached)
		if err != nil {
			return DrillView{}, err
		}
		return viewFor(drill, false, s.now()), nil
	}
	drill, err := s.store.Load(ctx, id)
	if err != nil {
		return DrillView{}, err
	}
	if drill.State == domain.StateCertified {
		clone, err := cloneDrillCase(drill)
		if err != nil {
			return DrillView{}, err
		}
		s.certifiedCacheMu.Lock()
		s.certifiedCache[id] = clone
		s.certifiedCacheMu.Unlock()
	}
	return viewFor(drill, false, s.now()), nil
}

func (s *Service) List(ctx context.Context) ([]DrillView, error) {
	drills, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]DrillView, 0, len(drills))
	for _, drill := range drills {
		views = append(views, viewFor(drill, false, s.now()))
	}
	return views, nil
}

func viewFor(drill domain.DrillCase, replayed bool, now time.Time) DrillView {
	todo := make([]TodoItem, 0)
	activationStatus := activationStatuses(drill.Activation, drill.PlannedStart, now)
	if drill.State == domain.StateDraft {
		todo = append(todo, TodoItem{Kind: "freeze", ResourceID: drill.ID, Label: "冻结基线"})
	}
	if drill.State == domain.StateFrozen && !allActivationValid(activationStatus) {
		todo = append(todo, TodoItem{Kind: "activation", ResourceID: drill.ID, Label: "登记启用证据"})
	}
	if drill.State == domain.StateFrozen && allActivationValid(activationStatus) {
		todo = append(todo, TodoItem{Kind: "start", ResourceID: drill.ID, Label: "启动演练"})
	}
	if drill.State == domain.StateActive {
		if checkpoint, ok := drill.NextCheckpoint(); ok {
			todo = append(todo, TodoItem{Kind: "observation", ResourceID: checkpoint.ID, Label: "记录检查点：" + checkpoint.Name})
		}
	}
	findingTodos := make([]TodoItem, 0)
	for _, finding := range drill.Findings {
		if finding.Status == domain.FindingOpen {
			findingTodos = append(findingTodos, TodoItem{Kind: "remediation", ResourceID: finding.ID, Label: "制定整改措施", Severity: string(finding.Severity), DueStatus: "normal"})
		}
		if finding.Status == domain.FindingPlanned {
			dueStatus, overdue := dueClassification(finding.RetestPlannedAt, now)
			findingTodos = append(findingTodos, TodoItem{Kind: "retest", ResourceID: finding.ID, Label: "提交定向复测", Severity: string(finding.Severity), DueStatus: dueStatus, DueAt: finding.RetestPlannedAt, OwnerID: finding.OwnerID, OverdueSeconds: overdue})
		}
	}
	sort.SliceStable(findingTodos, func(i, j int) bool {
		left, right := findingTodos[i], findingTodos[j]
		if dueRank(left.DueStatus) != dueRank(right.DueStatus) {
			return dueRank(left.DueStatus) < dueRank(right.DueStatus)
		}
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if left.DueAt != right.DueAt {
			return left.DueAt < right.DueAt
		}
		return left.ResourceID < right.ResourceID
	})
	todo = append(todo, findingTodos...)
	if (drill.State == domain.StateActive || drill.State == domain.StateRemediation) && drill.AllLiveObservationsSubmitted() && len(drill.OpenFindings()) == 0 {
		todo = append(todo, TodoItem{Kind: "submit_review", ResourceID: drill.ID, Label: "提交独立复核"})
	}
	if drill.State == domain.StateReview {
		todo = append(todo, TodoItem{Kind: "review", ResourceID: drill.ID, Label: "独立复核决策"})
	}
	view := DrillView{Drill: drill, Todo: todo, ActivationStatus: activationStatus, Replayed: replayed}
	if next, ok := drill.NextCheckpoint(); ok {
		view.NextCheckpoint = &next
	}
	if len(drill.Receipts) > 0 {
		view.LatestReceipt = &drill.Receipts[len(drill.Receipts)-1]
	}
	return view
}

func activationStatuses(evidence *domain.ActivationEvidence, plannedStart string, now time.Time) []ActivationEvidenceStatus {
	labels := map[string]string{"evacuation_route": "疏散路线", "communication_gear": "通信设备", "accessible_facility": "无障碍设施", "personnel_ready": "人员到位"}
	result := make([]ActivationEvidenceStatus, 0, 4)
	if evidence == nil {
		for _, category := range []string{"evacuation_route", "communication_gear", "accessible_facility", "personnel_ready"} {
			result = append(result, ActivationEvidenceStatus{Category: category, Label: labels[category], Reason: "尚未登记"})
		}
		return result
	}
	for _, item := range domain.ActivationRecords(*evidence) {
		status := ActivationEvidenceStatus{Category: item.Category, Label: labels[item.Category], Registered: true, ValidUntil: item.Record.ValidUntil}
		collected, collectedErr := domain.ParseTime(item.Record.CollectedAt)
		until, untilErr := domain.ParseTime(item.Record.ValidUntil)
		start, startErr := domain.ParseTime(plannedStart)
		if collectedErr != nil || untilErr != nil || startErr != nil || domain.ValidateDigest(item.Category+".content_digest", item.Record.ContentDigest) != nil {
			status.Reason = "证据记录不完整"
		} else if collected.After(until) {
			status.Reason = "采集时间晚于有效截止时间"
		} else if until.Before(start) {
			status.Reason = "有效期未覆盖计划开始时间"
		} else if now.UTC().After(until) {
			status.Reason = "证据已过期"
		} else {
			status.Valid = true
		}
		result = append(result, status)
	}
	return result
}

func allActivationValid(statuses []ActivationEvidenceStatus) bool {
	if len(statuses) != 4 {
		return false
	}
	for _, status := range statuses {
		if !status.Valid {
			return false
		}
	}
	return true
}

func dueClassification(value string, now time.Time) (string, int64) {
	due, err := domain.ParseTime(value)
	if err != nil {
		return "overdue", 0
	}
	if now.UTC().After(due) {
		return "overdue", int64(now.UTC().Sub(due).Seconds())
	}
	if due.Sub(now.UTC()) <= 30*time.Minute {
		return "near_due", 0
	}
	return "normal", 0
}

func dueRank(value string) int {
	return map[string]int{"overdue": 0, "near_due": 1, "normal": 2}[value]
}

func severityRank(value string) int {
	return map[string]int{"critical": 0, "major": 1, "minor": 2}[value]
}

func fingerprint(operation string, input any) (string, error) {
	payload, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Input     any    `json:"input"`
	}{operation, input})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func newID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err == nil {
		return prefix + "_" + hex.EncodeToString(buffer)
	}
	sequence := fallbackIDCounter.Add(1)
	source := fmt.Sprintf("%s:%d:%d", prefix, time.Now().UnixNano(), sequence)
	digest := sha256.Sum256([]byte(source))
	return prefix + "_" + hex.EncodeToString(digest[:12])
}

func (s *Service) mutationTime() string { return domain.NormalizeTime(s.now()) }

func validateMutation(m Mutation) error {
	if m.RequestID == "" {
		return domain.Invalid("request_id", "不能为空")
	}
	if m.ActorID == "" {
		return domain.Invalid("actor_id", "不能为空")
	}
	if m.ExpectedRevision < 1 {
		return domain.Invalid("expected_revision", "必须为正整数")
	}
	return nil
}

func ensureRevision(drill domain.DrillCase, expected int64) error {
	if drill.Revision != expected {
		return &domain.RevisionConflict{Expected: expected, Current: drill.Revision}
	}
	return nil
}

func cloneDrillCase(source domain.DrillCase) (domain.DrillCase, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return domain.DrillCase{}, fmt.Errorf("克隆演练聚合: %w", err)
	}
	var clone domain.DrillCase
	if err := json.Unmarshal(data, &clone); err != nil {
		return domain.DrillCase{}, fmt.Errorf("克隆演练聚合: %w", err)
	}
	return clone, nil
}

func sortCheckpoints(checkpoints []domain.Checkpoint) {
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].Sequence < checkpoints[j].Sequence })
}

func (s *Service) replay(ctx context.Context, requestID, fingerprint string) (DrillView, bool, error) {
	raw, found, err := s.store.Replay(ctx, requestID, fingerprint)
	if err != nil || !found {
		return DrillView{}, found, err
	}
	var result DrillView
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, true, fmt.Errorf("解码幂等响应: %w", err)
	}
	result.Replayed = true
	return result, true, nil
}

func (s *Service) commit(ctx context.Context, drill domain.DrillCase, expected *int64, requestID, fp, actorID, eventType string, eventPayload any) (DrillView, error) {
	result := viewFor(drill, false, s.now())
	response, err := json.Marshal(result)
	if err != nil {
		return DrillView{}, err
	}
	commit, err := s.store.Commit(ctx, store.CommitInput{Drill: drill, ExpectedRevision: expected, RequestID: requestID, Fingerprint: fp, ActorID: actorID, EventType: eventType, EventPayload: eventPayload, OccurredAt: drill.UpdatedAt, Response: response})
	if err != nil {
		return DrillView{}, err
	}
	if commit.Replayed {
		if err := json.Unmarshal(commit.Response, &result); err != nil {
			return result, err
		}
		result.Replayed = true
	}
	return result, nil
}
