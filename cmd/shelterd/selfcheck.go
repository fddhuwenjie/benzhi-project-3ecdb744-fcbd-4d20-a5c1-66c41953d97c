package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/web"
	"shelter-drill-gate/internal/workflow"
)

func runSelfCheck(address string) error {
	temporary, err := os.CreateTemp("", "shelter-drill-gate-*.db")
	if err != nil {
		return err
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		return err
	}
	defer os.Remove(path)
	repository, err := store.Open(path)
	if err != nil {
		return err
	}
	defer repository.Close()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听自检地址 %s: %w", address, err)
	}
	server := &http.Server{Handler: web.New(workflow.New(repository)).Handler(), ReadHeaderTimeout: 3 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	baseURL := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 4 * time.Second}
	if err := waitHealthy(client, baseURL); err != nil {
		return err
	}
	if err := executeBusinessCheck(client, baseURL); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return err
	}
	select {
	case serveErr := <-serveDone:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			return serveErr
		}
	case <-ctx.Done():
		return fmt.Errorf("自检服务未按时退出")
	}
	return nil
}

func waitHealthy(client *http.Client, baseURL string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("自检 HTTP 服务未就绪")
}

func executeBusinessCheck(client *http.Client, baseURL string) error {
	now := time.Now().UTC()
	start, end := now.Add(-10*time.Minute).Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339)
	create := map[string]any{
		"request_id": "self-create", "actor_id": "coordinator-self", "title": "回环自检演练", "site_name": "自检避难场所",
		"scenario_risks": []string{"地震"}, "planned_start": start, "planned_end": end,
		"checkpoints": []map[string]any{{"sequence": 1, "name": "集结核验", "responsible_role": "疏散岗", "time_limit_seconds": 120, "minimum_capacity": 20, "communication_required": true, "accessibility_required": true}},
	}
	view, err := postView(client, baseURL+"/api/drills", create)
	if err != nil {
		return err
	}
	id, revision := view.Drill.ID, view.Drill.Revision
	view, err = postView(client, baseURL+"/api/drills/"+id+"/freeze", mutationBody("self-freeze", revision, "coordinator-self"))
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	activation := mutationBody("self-activation", revision, "coordinator-self")
	activation["evacuation_route"] = activationRecord(now.Add(-time.Minute), now.Add(time.Hour), "疏散路线")
	activation["communication_gear"] = activationRecord(now.Add(-time.Minute), now.Add(time.Hour), "通信设备")
	activation["accessible_facility"] = activationRecord(now.Add(-time.Minute), now.Add(time.Hour), "无障碍设施")
	activation["personnel_ready"] = activationRecord(now.Add(-time.Minute), now.Add(time.Hour), "人员到位")
	view, err = postView(client, baseURL+"/api/drills/"+id+"/activation", activation)
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	view, err = postView(client, baseURL+"/api/drills/"+id+"/start", mutationBody("self-start", revision, "coordinator-self"))
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	observation := mutationBody("self-observation", revision, "coordinator-self")
	observation["checkpoint_id"], observation["observed_at"] = view.Drill.Checkpoints[0].ID, now.Format(time.RFC3339)
	observation["participant_count"], observation["elapsed_seconds"] = 10, 90
	observation["communication_passed"], observation["accessibility_passed"] = true, true
	observation["evidence_digest"] = selfDigest("现场自检证据")
	view, err = postView(client, baseURL+"/api/drills/"+id+"/observations", observation)
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	if len(view.Drill.Findings) != 1 || view.Drill.State != "remediation" {
		return fmt.Errorf("自检未产生预期严重整改项")
	}
	findingID := view.Drill.Findings[0].ID
	plan := mutationBody("self-plan", revision, "coordinator-self")
	plan["cause"], plan["corrective_action"], plan["owner_id"] = "签到遗漏", "补齐人员并复核名册", "owner-self"
	plan["retest_planned_at"] = now.Add(5 * time.Minute).Format(time.RFC3339)
	view, err = postView(client, baseURL+"/api/drills/"+id+"/findings/"+findingID+"/plan", plan)
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	retest := mutationBody("self-retest", revision, "coordinator-self")
	retest["observed_at"], retest["participant_count"], retest["elapsed_seconds"] = now.Add(5*time.Minute).Format(time.RFC3339), 25, 80
	retest["communication_passed"], retest["accessibility_passed"] = true, true
	retest["evidence_digest"] = selfDigest("定向复测证据")
	view, err = postView(client, baseURL+"/api/drills/"+id+"/findings/"+findingID+"/retest", retest)
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	view, err = postView(client, baseURL+"/api/drills/"+id+"/submit-review", mutationBody("self-submit", revision, "coordinator-self"))
	if err != nil {
		return err
	}
	revision = view.Drill.Revision
	decision := mutationBody("self-approve", revision, "reviewer-self")
	decision["decision"], decision["reason"] = "approved", "基线、现场事实和整改复测证据一致"
	decision["snapshot_digest"] = view.Drill.CurrentSnapshot.PayloadDigest
	checklist := make([]map[string]string, 0, len(view.Drill.CurrentSnapshot.Checklist))
	for _, item := range view.Drill.CurrentSnapshot.Checklist {
		checklist = append(checklist, map[string]string{"item_id": item.ID, "conclusion": "passed", "opinion": "自检核验一致"})
	}
	decision["checklist"] = checklist
	view, err = postView(client, baseURL+"/api/drills/"+id+"/review-decision", decision)
	if err != nil {
		return err
	}
	if view.Drill.Dossier == nil || view.Drill.State != "certified" {
		return fmt.Errorf("自检未生成终态档案")
	}
	var verification struct {
		Valid   bool   `json:"valid"`
		Message string `json:"message"`
	}
	if err := postJSON(client, baseURL+"/api/drills/"+id+"/dossier/verify", map[string]any{}, &verification); err != nil {
		return err
	}
	if !verification.Valid {
		return fmt.Errorf("档案验证失败: %s", verification.Message)
	}
	return nil
}

func activationRecord(collectedAt, validUntil time.Time, source string) map[string]string {
	return map[string]string{"collected_at": collectedAt.Format(time.RFC3339), "valid_until": validUntil.Format(time.RFC3339), "content_digest": selfDigest(source)}
}

func mutationBody(requestID string, revision int64, actorID string) map[string]any {
	return map[string]any{"request_id": requestID, "expected_revision": revision, "actor_id": actorID}
}

func postView(client *http.Client, url string, body any) (workflow.DrillView, error) {
	var result workflow.DrillView
	err := postJSON(client, url, body, &result)
	return result, err
}

func postJSON(client *http.Client, url string, body, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("POST %s 返回 %d: %s", url, response.StatusCode, data)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("解码 %s 响应: %w", url, err)
	}
	return nil
}

func selfDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
