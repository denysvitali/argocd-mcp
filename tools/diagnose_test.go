package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	healthlib "github.com/argoproj/gitops-engine/pkg/health"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yaml "sigs.k8s.io/yaml"
)

// makeHealthyApp returns a minimal healthy, synced Application for testing.
func makeHealthyApp(name string) *v1alpha1.Application {
	targetRevision := "main"
	return &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{TargetRevision: targetRevision},
		},
		Status: v1alpha1.ApplicationStatus{
			Health: v1alpha1.AppHealthStatus{Status: healthlib.HealthStatusHealthy},
			Sync:   v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeSynced, Revision: "abc1234"},
		},
	}
}

// makeDegradedApp returns an Application that is degraded.
func makeDegradedApp(name string) *v1alpha1.Application {
	targetRevision := "main"
	return &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{TargetRevision: targetRevision},
		},
		Status: v1alpha1.ApplicationStatus{
			Health: v1alpha1.AppHealthStatus{
				Status:  healthlib.HealthStatusDegraded,
				Message: "Deployment has timed out waiting for rollout to complete",
			},
			Sync: v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeSynced, Revision: "def5678"},
		},
	}
}

// makeOutOfSyncApp returns an Application that is healthy but out-of-sync.
func makeOutOfSyncApp(name string) *v1alpha1.Application {
	targetRevision := "main"
	return &v1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{TargetRevision: targetRevision},
		},
		Status: v1alpha1.ApplicationStatus{
			Health: v1alpha1.AppHealthStatus{Status: healthlib.HealthStatusHealthy},
			Sync:   v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeOutOfSync, Revision: "abc1234"},
		},
	}
}

// emptyEventListJSON returns a raw events interface{} with no items.
func emptyEventListJSON() interface{} {
	type eventList struct {
		Items []interface{} `json:"items"`
	}
	return eventList{Items: []interface{}{}}
}

// warningEventListJSON returns a raw events interface{} containing one Warning event.
func warningEventListJSON(reason, message, kind, resName string) interface{} {
	data, _ := json.Marshal(map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"type":    "Warning",
				"reason":  reason,
				"message": message,
				"involvedObject": map[string]interface{}{
					"kind": kind,
					"name": resName,
				},
			},
		},
	})
	var result interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

func newTestLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.WarnLevel) // suppress noise in tests
	return l
}

// TestDiagnoseApplication_MissingName verifies that omitting the name parameter
// returns an error result.
func TestDiagnoseApplication_MissingName(t *testing.T) {
	mock := &MockArgoClient{}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for missing name")
	}
}

// TestDiagnoseApplication_HealthyApp verifies the report for a healthy, synced application.
func TestDiagnoseApplication_HealthyApp(t *testing.T) {
	app := makeHealthyApp("my-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "my-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	// Decode the YAML output.
	report := decodeReport(t, result)

	if report.Severity != SeverityHealthy {
		t.Errorf("expected severity=%s, got %s", SeverityHealthy, report.Severity)
	}
	if report.HealthStatus != string(healthlib.HealthStatusHealthy) {
		t.Errorf("expected health=Healthy, got %s", report.HealthStatus)
	}
	if report.SyncStatus != string(v1alpha1.SyncStatusCodeSynced) {
		t.Errorf("expected sync=Synced, got %s", report.SyncStatus)
	}
	if len(report.RootCauses) != 0 {
		t.Errorf("expected no root causes for healthy app, got %d", len(report.RootCauses))
	}
	if report.Application != "my-app" {
		t.Errorf("expected application=my-app, got %s", report.Application)
	}
}

// TestDiagnoseApplication_DegradedApp verifies the report for a degraded application,
// including that the health signal is captured as a root cause.
func TestDiagnoseApplication_DegradedApp(t *testing.T) {
	app := makeDegradedApp("broken-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "broken-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	report := decodeReport(t, result)

	if report.Severity != SeverityCritical {
		t.Errorf("expected severity=%s, got %s", SeverityCritical, report.Severity)
	}
	if len(report.RootCauses) == 0 {
		t.Error("expected at least one root cause for degraded app")
	}

	// The first root cause should be the health signal.
	found := false
	for _, rc := range report.RootCauses {
		if rc.Source == "health_check" {
			found = true
			if rc.Detail == "" {
				t.Error("expected non-empty detail for health root cause")
			}
			if rc.Remediation == "" {
				t.Error("expected non-empty remediation for health root cause")
			}
		}
	}
	if !found {
		t.Error("expected a health_check root cause signal")
	}
}

// TestDiagnoseApplication_OutOfSyncResources verifies that out-of-sync managed resources
// appear in the report and generate root cause signals.
func TestDiagnoseApplication_OutOfSyncResources(t *testing.T) {
	app := makeOutOfSyncApp("drifted-app")
	outOfSyncResource := &v1alpha1.ResourceDiff{
		Kind:                "Deployment",
		Name:                "web",
		Namespace:           "default",
		Modified:            true,
		TargetState:         `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":3}}`,
		NormalizedLiveState: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"web"},"spec":{"replicas":1}}`,
	}

	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{outOfSyncResource}, nil
		},
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "drifted-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	report := decodeReport(t, result)

	if len(report.OutOfSyncResources) == 0 {
		t.Error("expected at least one out-of-sync resource")
	}

	foundDiff := false
	for _, rc := range report.RootCauses {
		if rc.Source == "resource_diff" && rc.Resource == "Deployment/web" {
			foundDiff = true
			if rc.ToolCall == "" {
				t.Error("expected a tool_call for out-of-sync resource signal")
			}
		}
	}
	if !foundDiff {
		t.Errorf("expected resource_diff root cause for Deployment/web, root causes: %+v", report.RootCauses)
	}
}

// TestDiagnoseApplication_WarningEvents verifies that Kubernetes Warning events are
// captured as root cause signals.
func TestDiagnoseApplication_WarningEvents(t *testing.T) {
	app := makeHealthyApp("event-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return warningEventListJSON("BackOff", "Back-off restarting failed container", "Pod", "web-abc123"), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "event-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	report := decodeReport(t, result)

	foundEvt := false
	for _, rc := range report.RootCauses {
		if rc.Source == "kubernetes_event" {
			foundEvt = true
			if rc.Remediation == "" {
				t.Error("expected non-empty remediation for event root cause")
			}
		}
	}
	if !foundEvt {
		t.Error("expected a kubernetes_event root cause signal for BackOff event")
	}
}

// TestDiagnoseApplication_DataSources verifies that queried data sources are listed.
func TestDiagnoseApplication_DataSources(t *testing.T) {
	app := makeHealthyApp("ds-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "ds-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	report := decodeReport(t, result)

	if len(report.DataSourcesQueried) == 0 {
		t.Error("expected at least one data source in DataSourcesQueried")
	}

	// application_status must always be listed.
	found := false
	for _, src := range report.DataSourcesQueried {
		if src == "application_status" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'application_status' in DataSourcesQueried, got: %v", report.DataSourcesQueried)
	}
}

// TestDiagnoseApplication_NextActions verifies that next actions are populated for
// non-healthy applications.
func TestDiagnoseApplication_NextActions(t *testing.T) {
	app := makeDegradedApp("broken-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "broken-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	report := decodeReport(t, result)

	if len(report.NextActions) == 0 {
		t.Error("expected non-empty NextActions for a degraded application")
	}
}

// TestClassifySeverity verifies the severity classification logic.
func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		health   string
		sync     string
		expected DiagnosticSeverity
	}{
		{string(healthlib.HealthStatusHealthy), string(v1alpha1.SyncStatusCodeSynced), SeverityHealthy},
		{string(healthlib.HealthStatusHealthy), string(v1alpha1.SyncStatusCodeOutOfSync), SeverityDegraded},
		{string(healthlib.HealthStatusDegraded), string(v1alpha1.SyncStatusCodeSynced), SeverityCritical},
		{string(healthlib.HealthStatusMissing), string(v1alpha1.SyncStatusCodeSynced), SeverityCritical},
		{string(healthlib.HealthStatusProgressing), string(v1alpha1.SyncStatusCodeSynced), SeverityDegraded},
		{string(healthlib.HealthStatusUnknown), string(v1alpha1.SyncStatusCodeUnknown), SeverityUnknown},
	}
	for _, tt := range tests {
		got := classifySeverity(tt.health, tt.sync)
		if got != tt.expected {
			t.Errorf("classifySeverity(%q, %q) = %q, want %q", tt.health, tt.sync, got, tt.expected)
		}
	}
}

// TestExtractErrorLines verifies that error-keyword lines are extracted from log text.
func TestExtractErrorLines(t *testing.T) {
	logs := `
2024-01-01 normal log line
2024-01-01 Error: connection refused to database
2024-01-01 another normal line
2024-01-01 panic: runtime error
2024-01-01 yet another normal line
`
	lines := extractErrorLines(logs, 10)
	if len(lines) != 2 {
		t.Errorf("expected 2 error lines, got %d: %v", len(lines), lines)
	}
}

// TestYamlToolCall verifies that yamlToolCall produces valid YAML-like output.
func TestYamlToolCall(t *testing.T) {
	out := yamlToolCall("sync_application", map[string]interface{}{"name": "my-app"})
	if out == "" {
		t.Error("expected non-empty yamlToolCall output")
	}
	if !containsString(out, "sync_application") {
		t.Error("expected tool name in yamlToolCall output")
	}
	if !containsString(out, "my-app") {
		t.Error("expected arg value in yamlToolCall output")
	}
}

// --- helpers ---

// decodeReport parses the YAML text content from an MCP result into a DiagnosticReport.
func decodeReport(t *testing.T, result *mcp.CallToolResult) DiagnosticReport {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("result is nil or has no content")
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected mcp.TextContent, got %T", result.Content[0])
	}
	yamlText := textContent.Text

	// Convert YAML to JSON via the sigs.k8s.io/yaml library, then unmarshal into struct.
	var asMap interface{}
	if err := yaml.Unmarshal([]byte(yamlText), &asMap); err != nil {
		t.Fatalf("failed to unmarshal YAML result: %v\nYAML:\n%s", err, yamlText)
	}
	jsonBytes, err := json.Marshal(asMap)
	if err != nil {
		t.Fatalf("failed to re-marshal to JSON: %v", err)
	}
	var report DiagnosticReport
	if err := json.Unmarshal(jsonBytes, &report); err != nil {
		t.Fatalf("failed to unmarshal into DiagnosticReport: %v\nJSON: %s", err, jsonBytes)
	}
	return report
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestClassifyFailureCategory validates regex-based failure pattern classification.
func TestClassifyFailureCategory(t *testing.T) {
	tests := []struct {
		name         string
		health       string
		sync         string
		events       []parsedEvent
		logs         string
		previousLogs string
		want         FailureCategory
	}{
		{
			name:   "healthy and synced",
			health: string(healthlib.HealthStatusHealthy),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			want:   FailureCategoryHealthy,
		},
		{
			name:         "OOMKilled in previous logs",
			health:       string(healthlib.HealthStatusDegraded),
			sync:         string(v1alpha1.SyncStatusCodeSynced),
			previousLogs: "2024-01-01 Fatal: OOMKilled - container exceeded memory limit",
			want:         FailureCategoryOOMKilled,
		},
		{
			name:   "OOMKilled takes priority over CrashLoop",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			events: []parsedEvent{
				{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container"},
				{Type: "Warning", Reason: "OOMKilling", Message: "oom_kill process 1234"},
			},
			want: FailureCategoryOOMKilled,
		},
		{
			name:   "ImagePullBackOff from event",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			events: []parsedEvent{
				{Type: "Warning", Reason: "Failed", Message: "Failed to pull image \"myregistry/myapp:v2.0\": ErrImagePull"},
			},
			want: FailureCategoryImagePull,
		},
		{
			name:   "CrashLoopBackOff from event",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			events: []parsedEvent{
				{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container"},
			},
			want: FailureCategoryCrashLoop,
		},
		{
			name:   "CrashLoopBackOff from current logs",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			logs:   "2024-01-01 CrashLoopBackOff: too many restarts",
			want:   FailureCategoryCrashLoop,
		},
		{
			name:   "QuotaExceeded from event",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			events: []parsedEvent{
				{Type: "Warning", Reason: "FailedCreate", Message: "Error creating: pods is forbidden: exceeded quota"},
			},
			want: FailureCategoryQuota,
		},
		{
			name:   "PodSchedulingFailed from event",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			events: []parsedEvent{
				{Type: "Warning", Reason: "FailedScheduling", Message: "Insufficient memory: 3 Insufficient memory"},
			},
			want: FailureCategoryScheduling,
		},
		{
			name:   "OutOfSync fallback",
			health: string(healthlib.HealthStatusHealthy),
			sync:   string(v1alpha1.SyncStatusCodeOutOfSync),
			want:   FailureCategoryOutOfSync,
		},
		{
			name:   "Degraded fallback with no pattern match",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			want:   FailureCategoryDegraded,
		},
		{
			name:   "ConfigError from log",
			health: string(healthlib.HealthStatusDegraded),
			sync:   string(v1alpha1.SyncStatusCodeSynced),
			logs:   "Error: secret \"db-password\" not found in namespace production",
			want:   FailureCategoryConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFailureCategory(tt.health, tt.sync, tt.events, tt.logs, tt.previousLogs, nil)
			if got != tt.want {
				t.Errorf("classifyFailureCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDiagnoseApplication_PreviousLogsSignal verifies that previous container logs
// are fetched and included as a root cause signal, which is critical for CrashLoopBackOff.
func TestDiagnoseApplication_PreviousLogsSignal(t *testing.T) {
	app := makeDegradedApp("crashing-app")

	// Simulate an unhealthy pod in the resource tree.
	tree := &v1alpha1.ApplicationTree{
		Nodes: []v1alpha1.ResourceNode{
			{
				ResourceRef: v1alpha1.ResourceRef{
					Kind:      "Pod",
					Name:      "crashing-pod-abc",
					Namespace: "default",
				},
				Health: &v1alpha1.HealthStatus{
					Status:  healthlib.HealthStatusDegraded,
					Message: "CrashLoopBackOff",
				},
			},
		},
	}

	callCount := 0
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn:     func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) { return tree, nil },
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
		GetApplicationLogsFn: func(_ context.Context, q *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error) {
			callCount++
			if q.Previous != nil && *q.Previous {
				// Return a crash reason in the "previous" logs.
				return []client.ApplicationLogEntry{
					{Content: "panic: runtime error: nil pointer dereference"},
					{Content: "goroutine 1 [running]:"},
				}, nil
			}
			return []client.ApplicationLogEntry{
				{Content: "starting application..."},
			}, nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "crashing-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	report := decodeReport(t, result)

	// Verify that previous logs were fetched (callCount should be at least 2:
	// once for current logs, once for previous logs).
	if callCount < 2 {
		t.Errorf("expected GetApplicationLogs to be called at least twice (current + previous), got %d calls", callCount)
	}

	// Verify that a previous_pod_logs root cause signal was added.
	foundPrevLogs := false
	for _, rc := range report.RootCauses {
		if rc.Source == "previous_pod_logs" {
			foundPrevLogs = true
			if rc.Detail == "" {
				t.Error("expected non-empty detail in previous_pod_logs signal")
			}
		}
	}
	if !foundPrevLogs {
		t.Errorf("expected a previous_pod_logs root cause signal; got causes: %+v", report.RootCauses)
	}

	// Verify category classification.
	if report.Category != FailureCategoryCrashLoop && report.Category != FailureCategoryUnknown && report.Category != FailureCategoryDegraded {
		t.Errorf("unexpected category %q for crashing app", report.Category)
	}
}

// TestDiagnoseApplication_CategoryField verifies that the Category field is populated.
func TestDiagnoseApplication_CategoryField(t *testing.T) {
	app := makeHealthyApp("healthy-app")
	mock := &MockArgoClient{
		GetApplicationFn: func(_ context.Context, _ *application.ApplicationQuery) (*v1alpha1.Application, error) {
			return app, nil
		},
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) { return nil, nil },
		GetResourceTreeFn: func(_ context.Context, _ string) (*v1alpha1.ApplicationTree, error) {
			return &v1alpha1.ApplicationTree{}, nil
		},
		GetApplicationEventsFn: func(_ context.Context, _ *application.ApplicationResourceEventsQuery) (interface{}, error) {
			return emptyEventListJSON(), nil
		},
	}
	tm := NewToolManager(mock, newTestLogger(), false)

	result, err := tm.handleDiagnoseApplication(context.Background(), map[string]interface{}{"name": "healthy-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	report := decodeReport(t, result)

	if report.Category != FailureCategoryHealthy {
		t.Errorf("expected Category=%q for healthy app, got %q", FailureCategoryHealthy, report.Category)
	}
}
