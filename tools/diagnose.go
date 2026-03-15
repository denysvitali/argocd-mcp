package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	healthlib "github.com/argoproj/gitops-engine/pkg/health"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- Output types ---

// DiagnosticSeverity classifies the overall health of the application.
type DiagnosticSeverity string

const (
	SeverityHealthy  DiagnosticSeverity = "healthy"
	SeverityDegraded DiagnosticSeverity = "degraded"
	SeverityCritical DiagnosticSeverity = "critical"
	SeverityUnknown  DiagnosticSeverity = "unknown"
)

// RootCauseSignal is a single identified problem with remediation guidance.
type RootCauseSignal struct {
	// Source is the ArgoCD data source that revealed this signal.
	// Examples: "sync_status", "health_check", "kubernetes_event", "pod_logs", "resource_diff"
	Source string `json:"source" yaml:"source"`

	// Resource identifies which Kubernetes resource is involved (may be empty for app-level signals).
	Resource string `json:"resource,omitempty" yaml:"resource,omitempty"`

	// Signal is a terse description of what went wrong.
	Signal string `json:"signal" yaml:"signal"`

	// Detail provides the raw evidence: error message, log snippet, event reason, or diff excerpt.
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`

	// Remediation is a concrete action the operator can take.
	Remediation string `json:"remediation" yaml:"remediation"`

	// ToolCall is the exact MCP tool call (tool name + arguments in YAML) the LLM can execute
	// immediately to carry out the remediation. Empty when no automated action is available.
	ToolCall string `json:"tool_call,omitempty" yaml:"tool_call,omitempty"`
}

// FailureCategory is a machine-readable label for the primary failure mode
// detected during diagnosis. Using this field an LLM or automation can take
// category-specific remediation actions without re-parsing free-form text.
type FailureCategory string

const (
	FailureCategoryCrashLoop  FailureCategory = "CrashLoopBackOff"
	FailureCategoryOOMKilled  FailureCategory = "OOMKilled"
	FailureCategoryImagePull  FailureCategory = "ImagePullBackOff"
	FailureCategorySyncFailed FailureCategory = "SyncFailed"
	FailureCategoryDegraded   FailureCategory = "DegradedDeployment"
	FailureCategoryQuota      FailureCategory = "QuotaExceeded"
	FailureCategoryScheduling FailureCategory = "PodSchedulingFailed"
	FailureCategoryConfig     FailureCategory = "ConfigError"
	FailureCategoryNetwork    FailureCategory = "NetworkError"
	FailureCategoryOutOfSync  FailureCategory = "OutOfSync"
	FailureCategoryHealthy    FailureCategory = "Healthy"
	FailureCategoryUnknown    FailureCategory = "Unknown"
)

// DiagnosticReport is the top-level response from diagnose_application.
type DiagnosticReport struct {
	// Application is the name of the ArgoCD application that was diagnosed.
	Application string `json:"application" yaml:"application"`

	// Severity is the overall severity classification.
	Severity DiagnosticSeverity `json:"severity" yaml:"severity"`

	// Category is a machine-readable failure classification that automation can
	// switch on without re-parsing the free-form Summary or RootCauses fields.
	Category FailureCategory `json:"category" yaml:"category"`

	// SyncStatus is the raw ArgoCD sync phase (Synced / OutOfSync / Unknown).
	SyncStatus string `json:"sync_status" yaml:"sync_status"`

	// HealthStatus is the raw ArgoCD health status (Healthy / Degraded / Progressing / Missing / Unknown).
	HealthStatus string `json:"health_status" yaml:"health_status"`

	// CurrentRevision is the Git SHA that is currently deployed.
	CurrentRevision string `json:"current_revision,omitempty" yaml:"current_revision,omitempty"`

	// TargetRevision is the Git ref / SHA that ArgoCD wants to deploy.
	TargetRevision string `json:"target_revision,omitempty" yaml:"target_revision,omitempty"`

	// OutOfSyncResources is a summary list of resources that diverge from Git.
	OutOfSyncResources []string `json:"out_of_sync_resources,omitempty" yaml:"out_of_sync_resources,omitempty"`

	// UnhealthyResources is a summary list of resources with non-Healthy status.
	UnhealthyResources []string `json:"unhealthy_resources,omitempty" yaml:"unhealthy_resources,omitempty"`

	// RootCauses is the ordered list of identified problems, most critical first.
	RootCauses []RootCauseSignal `json:"root_causes,omitempty" yaml:"root_causes,omitempty"`

	// Summary is a single paragraph plain-English briefing ready to present to an operator.
	Summary string `json:"summary" yaml:"summary"`

	// NextActions lists the recommended immediate remediation steps in priority order.
	NextActions []string `json:"next_actions,omitempty" yaml:"next_actions,omitempty"`

	// DataSourcesQueried describes which APIs were called (for transparency / debugging).
	DataSourcesQueried []string `json:"data_sources_queried" yaml:"data_sources_queried"`

	// DiagnosedAt is the UTC timestamp of when the diagnosis was run.
	DiagnosedAt string `json:"diagnosed_at" yaml:"diagnosed_at"`
}

// --- Parallel data-fetch helpers ---

// appSnapshot holds all raw data fetched concurrently for a single application.
type appSnapshot struct {
	app     *v1alpha1.Application
	appErr  error
	managed []*v1alpha1.ResourceDiff
	mgrErr  error
	tree    *v1alpha1.ApplicationTree
	treeErr error
	events  []parsedEvent
	evtErr  error
	logs    string // current container log snippets from unhealthy pods
	logsErr error
	// previousLogs contains logs from previously-terminated (crashed) containers.
	// This is the most valuable signal for CrashLoopBackOff and OOMKilled diagnosis.
	previousLogs string
}

// parsedEvent is a cleaned-up Kubernetes event extracted from the raw API response.
type parsedEvent struct {
	Type         string
	Reason       string
	Message      string
	ResourceKind string
	ResourceName string
}

// --- Handler ---

const (
	diagLogTailLines    = 50
	diagLogSinceSeconds = 900 // 15 minutes
	diagMaxRootCauses   = 10
	diagMaxLogChars     = 2000
)

func (tm *ToolManager) handleDiagnoseApplication(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	appName := String(arguments, "name", "")
	if appName == "" {
		return errorResult("name is required"), nil
	}

	// Fan out all reads concurrently so the total latency is bounded by the
	// slowest single API call rather than their sum.
	snap := tm.fetchAppSnapshot(ctx, appName)

	if snap.appErr != nil {
		return errorResult(fmt.Sprintf("failed to fetch application %q: %v", appName, snap.appErr)), nil
	}

	report := buildDiagnosticReport(appName, snap)
	return Result(report, nil)
}

// fetchAppSnapshot fires all ArgoCD reads concurrently and collects results.
func (tm *ToolManager) fetchAppSnapshot(ctx context.Context, appName string) appSnapshot {
	var snap appSnapshot
	var wg sync.WaitGroup

	// 1. Get application state.
	wg.Add(1)
	go func() {
		defer wg.Done()
		q := &application.ApplicationQuery{Name: &appName}
		snap.app, snap.appErr = tm.client.GetApplication(ctx, q)
	}()

	// 2. Get managed resources (diff information).
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap.managed, snap.mgrErr = tm.client.GetManagedResources(ctx, appName)
	}()

	// 3. Get resource tree (health per node).
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap.tree, snap.treeErr = tm.client.GetResourceTree(ctx, appName)
	}()

	// 4. Get application-level Kubernetes events.
	wg.Add(1)
	go func() {
		defer wg.Done()
		evtQuery := &application.ApplicationResourceEventsQuery{Name: &appName}
		raw, err := tm.client.GetApplicationEvents(ctx, evtQuery)
		if err != nil {
			snap.evtErr = err
			return
		}
		snap.events, snap.evtErr = extractWarningEvents(raw)
	}()

	wg.Wait()

	// 5. After the tree is available, fetch logs from unhealthy pods (best-effort).
	//    We do this after the WaitGroup so we can use tree data to pick pods.
	if snap.treeErr == nil && snap.tree != nil {
		snap.logs, snap.logsErr = tm.fetchUnhealthyPodLogs(ctx, appName, snap.tree, false)
		// Also fetch previous (crashed) container logs — the single most valuable
		// signal for CrashLoopBackOff and OOMKilled diagnosis.
		snap.previousLogs, _ = tm.fetchUnhealthyPodLogs(ctx, appName, snap.tree, true)
	}

	return snap
}

// fetchUnhealthyPodLogs fetches the tail of recent logs from Pods that are not Healthy.
// When previous is true, it fetches logs from the previously-terminated (crashed) container
// instead of the currently running one — this is the primary signal for CrashLoopBackOff.
func (tm *ToolManager) fetchUnhealthyPodLogs(ctx context.Context, appName string, tree *v1alpha1.ApplicationTree, previous bool) (string, error) {
	const maxPods = 3
	var logParts []string

	fetched := 0
	for i := range tree.Nodes {
		n := &tree.Nodes[i]
		if n.Kind != "Pod" {
			continue
		}
		healthy := n.Health != nil && n.Health.Status == healthlib.HealthStatusHealthy
		if healthy {
			continue
		}
		if fetched >= maxPods {
			break
		}

		tailLines := int64(diagLogTailLines)
		sinceSeconds := int64(diagLogSinceSeconds)
		podName := n.Name
		ns := n.Namespace
		kind := "Pod"
		groupStr := ""
		previousFlag := previous

		q := &application.ApplicationPodLogsQuery{
			Name:      &appName,
			PodName:   &podName,
			Namespace: &ns,
			Kind:      &kind,
			Group:     &groupStr,
			TailLines: &tailLines,
			Previous:  &previousFlag,
		}
		if !previous {
			q.SinceSeconds = &sinceSeconds
		}

		entries, err := tm.client.GetApplicationLogs(ctx, q)
		if err != nil {
			// Previous logs may simply not exist for a pod that hasn't crashed yet; skip silently.
			continue
		}
		if len(entries) == 0 {
			continue
		}
		fetched++

		label := podName
		if previous {
			label = podName + " (previous/crashed)"
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("=== Pod: %s (ns: %s) ===\n", label, ns))
		for _, entry := range entries {
			sb.WriteString(entry.Content)
			sb.WriteByte('\n')
		}
		logParts = append(logParts, sb.String())
	}

	return strings.Join(logParts, "\n"), nil
}

// buildDiagnosticReport assembles the full DiagnosticReport from the snapshot.
func buildDiagnosticReport(appName string, snap appSnapshot) DiagnosticReport {
	report := DiagnosticReport{
		Application: appName,
		DiagnosedAt: time.Now().UTC().Format(time.RFC3339),
	}

	sources := []string{"application_status"}

	// --- Application-level status ---
	app := snap.app
	syncStatus := string(app.Status.Sync.Status)
	healthStatus := string(app.Status.Health.Status)
	report.SyncStatus = syncStatus
	report.HealthStatus = healthStatus
	report.CurrentRevision = app.Status.Sync.Revision
	if app.Spec.Source != nil {
		report.TargetRevision = app.Spec.Source.TargetRevision
	}

	// Determine overall severity.
	report.Severity = classifySeverity(healthStatus, syncStatus)

	// --- Collect root cause signals ---
	var causes []RootCauseSignal

	// Signal: application health status.
	if healthStatus != string(healthlib.HealthStatusHealthy) &&
		healthStatus != string(healthlib.HealthStatusProgressing) {
		causes = append(causes, RootCauseSignal{
			Source:      "health_check",
			Signal:      fmt.Sprintf("Application health is %s", healthStatus),
			Detail:      fmt.Sprintf("Application %q has health status %s", appName, healthStatus),
			Remediation: healthRemediationHint(healthStatus, appName),
			ToolCall:    healthRemediationToolCall(healthStatus, appName),
		})
	}

	// Signal: application sync operation state error.
	if app.Status.OperationState != nil && app.Status.OperationState.Message != "" {
		phase := string(app.Status.OperationState.Phase)
		if phase == "Error" || phase == "Failed" {
			causes = append(causes, RootCauseSignal{
				Source:      "sync_operation",
				Signal:      fmt.Sprintf("Last sync operation %s: %s", phase, app.Status.OperationState.Message),
				Detail:      app.Status.OperationState.Message,
				Remediation: "Investigate sync failure. Check resource-level errors with get_application_diff, then fix the Git source and re-sync.",
				ToolCall:    yamlToolCall("get_application_diff", map[string]interface{}{"name": appName}),
			})
		}
	}

	// --- Out-of-sync resources (from managed resources diff) ---
	if snap.mgrErr == nil {
		sources = append(sources, "managed_resources_diff")
		var outOfSync []string
		for _, r := range snap.managed {
			if r == nil {
				continue
			}
			if !r.Modified {
				continue
			}
			label := fmt.Sprintf("%s/%s", r.Kind, r.Name)
			outOfSync = append(outOfSync, label)

			// Produce a brief diff excerpt as evidence.
			targetYAML := stripManagedFieldsYaml(r.TargetState)
			liveYAML := stripManagedFieldsYaml(r.NormalizedLiveState)
			diffText := computeDiff(targetYAML, liveYAML)
			if len(diffText) > 500 {
				diffText = diffText[:500] + "... (truncated)"
			}

			causes = append(causes, RootCauseSignal{
				Source:      "resource_diff",
				Resource:    label,
				Signal:      fmt.Sprintf("%s is out-of-sync with Git", label),
				Detail:      diffText,
				Remediation: fmt.Sprintf("Sync application to reconcile %s with the desired state in Git.", label),
				ToolCall:    yamlToolCall("sync_application", map[string]interface{}{"name": appName}),
			})
		}
		report.OutOfSyncResources = outOfSync
	}

	// --- Unhealthy resources from resource tree ---
	if snap.treeErr == nil && snap.tree != nil {
		sources = append(sources, "resource_tree")
		var unhealthy []string
		for i := range snap.tree.Nodes {
			n := &snap.tree.Nodes[i]
			if n.Health == nil || n.Health.Status == healthlib.HealthStatusHealthy {
				continue
			}
			label := fmt.Sprintf("%s/%s", n.Kind, n.Name)
			unhealthy = append(unhealthy, label)

			if n.Health.Message != "" {
				causes = append(causes, RootCauseSignal{
					Source:      "health_check",
					Resource:    label,
					Signal:      fmt.Sprintf("%s is %s", label, string(n.Health.Status)),
					Detail:      n.Health.Message,
					Remediation: resourceHealthRemediationHint(n.Kind, n.Name, appName),
					ToolCall:    resourceHealthToolCall(n.Kind, n.Name, n.Namespace, appName),
				})
			}
		}
		report.UnhealthyResources = unhealthy
	}

	// --- Kubernetes Warning events ---
	if snap.evtErr == nil && len(snap.events) > 0 {
		sources = append(sources, "kubernetes_events")
		for _, evt := range snap.events {
			if evt.Type != "Warning" {
				continue
			}
			resource := ""
			if evt.ResourceKind != "" && evt.ResourceName != "" {
				resource = fmt.Sprintf("%s/%s", evt.ResourceKind, evt.ResourceName)
			}
			causes = append(causes, RootCauseSignal{
				Source:      "kubernetes_event",
				Resource:    resource,
				Signal:      fmt.Sprintf("Warning event: %s - %s", evt.Reason, truncateString(evt.Message, 200)),
				Detail:      evt.Message,
				Remediation: eventRemediationHint(evt.Reason, evt.ResourceKind, appName),
				ToolCall:    eventRemediationToolCall(evt.Reason, appName),
			})
		}
	}

	// --- Current pod log signals ---
	if snap.logs != "" {
		sources = append(sources, "pod_logs")
		// Extract error lines as evidence.
		errorLines := extractErrorLines(snap.logs, 5)
		if len(errorLines) > 0 {
			detail := strings.Join(errorLines, "\n")
			if len(detail) > diagMaxLogChars {
				detail = detail[:diagMaxLogChars] + "... (truncated)"
			}
			causes = append(causes, RootCauseSignal{
				Source:      "pod_logs",
				Signal:      "Error lines found in pod logs",
				Detail:      detail,
				Remediation: "Review full pod logs for root cause. Use get_logs with filter='error|exception|panic|fatal' to see more context.",
				ToolCall:    yamlToolCall("get_logs", map[string]interface{}{"name": appName, "filter": "error|exception|panic|fatal|Error|Exception"}),
			})
		}
	}

	// --- Previous (crashed) container log signals ---
	// This is the most valuable signal for CrashLoopBackOff and OOMKilled.
	// The previous container's final log lines typically contain the exact crash reason.
	if snap.previousLogs != "" {
		sources = append(sources, "previous_pod_logs")
		prevErrorLines := extractErrorLines(snap.previousLogs, 8)
		detail := snap.previousLogs
		if len(prevErrorLines) > 0 {
			detail = strings.Join(prevErrorLines, "\n")
		}
		if len(detail) > diagMaxLogChars {
			detail = detail[:diagMaxLogChars] + "... (truncated)"
		}
		causes = append(causes, RootCauseSignal{
			Source:      "previous_pod_logs",
			Signal:      "Logs from previously-terminated (crashed) container",
			Detail:      detail,
			Remediation: "These are the final log lines from the container before it crashed. Look for the crash reason, panic message, OOM signal, or fatal error.",
			ToolCall:    yamlToolCall("get_logs", map[string]interface{}{"name": appName, "previous": true}),
		})
	}

	// Cap root causes to keep context window manageable.
	if len(causes) > diagMaxRootCauses {
		causes = causes[:diagMaxRootCauses]
	}
	report.RootCauses = causes
	report.DataSourcesQueried = sources

	// Classify the failure category from all gathered signals.
	report.Category = classifyFailureCategory(healthStatus, syncStatus, snap.events, snap.logs, snap.previousLogs, report.UnhealthyResources)

	// Build next actions and summary.
	report.NextActions = buildNextActions(report)
	report.Summary = buildDiagnosticSummary(report)

	return report
}

// --- Failure category classification ---

// diagSignalPatterns maps failure categories to compiled regexes applied
// against the full corpus of events + log text.  Patterns are checked in
// priority order (most specific / most impactful first).
var diagSignalPatterns = []struct {
	category FailureCategory
	re       *regexp.Regexp
}{
	{FailureCategoryOOMKilled, regexp.MustCompile(`(?i)(OOMKilled|Out of memory|oom_kill|Killed process|memory limit exceeded)`)},
	{FailureCategoryImagePull, regexp.MustCompile(`(?i)(ImagePullBackOff|ErrImagePull|Failed to pull image|unauthorized|manifest unknown|image not found)`)},
	{FailureCategoryCrashLoop, regexp.MustCompile(`(?i)(CrashLoopBackOff|back-off restarting|container .* died|restarting failed container)`)},
	{FailureCategoryScheduling, regexp.MustCompile(`(?i)(FailedScheduling|Insufficient|Unschedulable|no nodes available|didn't match.*affinity|taint.*toleration)`)},
	{FailureCategoryQuota, regexp.MustCompile(`(?i)(exceeded quota|resource quota|LimitRange|forbidden.*exceeded)`)},
	{FailureCategoryConfig, regexp.MustCompile(`(?i)(secret.*not found|configmap.*not found|volume.*not found|failed to mount|permission denied|forbidden|invalid.*configuration)`)},
	{FailureCategoryDegraded, regexp.MustCompile(`(?i)(ProgressDeadlineExceeded|Deployment.*timed out|unavailable replicas|ReplicaSet.*failed)`)},
	{FailureCategorySyncFailed, regexp.MustCompile(`(?i)(ComparisonError|SyncFailed|hook.*failed|sync.*failed|failed to sync)`)},
	{FailureCategoryNetwork, regexp.MustCompile(`(?i)(connection refused|dial tcp|network unreachable|dns lookup|i/o timeout|connection reset)`)},
}

// classifyFailureCategory runs regex pattern matching across all available
// signals and returns the best matching FailureCategory.
func classifyFailureCategory(
	healthStatus, syncStatus string,
	events []parsedEvent,
	logs, previousLogs string,
	unhealthyResources []string,
) FailureCategory {
	// Healthy shortcut.
	if healthlib.HealthStatusCode(healthStatus) == healthlib.HealthStatusHealthy &&
		v1alpha1.SyncStatusCode(syncStatus) == v1alpha1.SyncStatusCodeSynced {
		return FailureCategoryHealthy
	}

	// Build a searchable corpus from all available text signals.
	var corpus strings.Builder
	for _, ev := range events {
		corpus.WriteString(ev.Reason)
		corpus.WriteByte(' ')
		corpus.WriteString(ev.Message)
		corpus.WriteByte('\n')
	}
	corpus.WriteString(logs)
	corpus.WriteString(previousLogs)
	for _, r := range unhealthyResources {
		corpus.WriteString(r)
		corpus.WriteByte('\n')
	}
	corpus.WriteString(healthStatus)
	corpus.WriteByte(' ')
	corpus.WriteString(syncStatus)

	text := corpus.String()

	for _, p := range diagSignalPatterns {
		if p.re.MatchString(text) {
			return p.category
		}
	}

	// Fallback: classify from status codes alone.
	if v1alpha1.SyncStatusCode(syncStatus) == v1alpha1.SyncStatusCodeOutOfSync {
		return FailureCategoryOutOfSync
	}
	if healthlib.HealthStatusCode(healthStatus) == healthlib.HealthStatusDegraded {
		return FailureCategoryDegraded
	}

	return FailureCategoryUnknown
}

// classifySeverity maps health/sync state to our three-level severity model.
func classifySeverity(healthStatus, syncStatus string) DiagnosticSeverity {
	switch healthlib.HealthStatusCode(healthStatus) {
	case healthlib.HealthStatusHealthy:
		if v1alpha1.SyncStatusCode(syncStatus) == v1alpha1.SyncStatusCodeSynced {
			return SeverityHealthy
		}
		return SeverityDegraded // healthy but out of sync
	case healthlib.HealthStatusDegraded:
		return SeverityCritical
	case healthlib.HealthStatusMissing:
		return SeverityCritical
	case healthlib.HealthStatusProgressing:
		return SeverityDegraded
	default:
		return SeverityUnknown
	}
}

// healthRemediationHint returns a plain-English remediation hint based on health status.
func healthRemediationHint(healthStatus, appName string) string {
	switch healthlib.HealthStatusCode(healthStatus) {
	case healthlib.HealthStatusDegraded:
		return fmt.Sprintf(
			"Application %q is Degraded. Check pod logs and events for crash-loop or OOM signals. "+
				"If a bad deployment is the cause, consider rolling back with rollback_application.", appName)
	case healthlib.HealthStatusMissing:
		return fmt.Sprintf(
			"Application %q resources are Missing. This typically means a sync is required or resources were manually deleted. "+
				"Run sync_application to reconcile.", appName)
	case healthlib.HealthStatusProgressing:
		return fmt.Sprintf(
			"Application %q is still Progressing. If it has been progressing for an unusual amount of time, "+
				"check pod events and logs for stuck rollout conditions.", appName)
	default:
		return fmt.Sprintf("Investigate %q health: check events and logs.", appName)
	}
}

// healthRemediationToolCall returns the suggested MCP tool call YAML for health-based remediation.
func healthRemediationToolCall(healthStatus, appName string) string {
	switch healthlib.HealthStatusCode(healthStatus) {
	case healthlib.HealthStatusDegraded:
		return yamlToolCall("get_logs", map[string]interface{}{"name": appName, "filter": "error|Error|exception|panic|OOMKilled"})
	case healthlib.HealthStatusMissing:
		return yamlToolCall("sync_application", map[string]interface{}{"name": appName})
	default:
		return yamlToolCall("get_application_events", map[string]interface{}{"name": appName})
	}
}

// resourceHealthRemediationHint returns a hint specific to an unhealthy Kubernetes resource.
func resourceHealthRemediationHint(kind, name, appName string) string {
	switch kind {
	case "Pod":
		return fmt.Sprintf("Pod %q is unhealthy. Retrieve its logs with get_logs and events with get_application_events to determine the cause (crash loop, OOM, image pull failure, etc.).", name)
	case "Deployment", "StatefulSet", "DaemonSet":
		return fmt.Sprintf("%s %q is unhealthy. Check the rollout status and underlying pod health.", kind, name)
	case "PersistentVolumeClaim":
		return fmt.Sprintf("PVC %q is unhealthy. It may be stuck in Pending state due to no available PersistentVolume or StorageClass misconfiguration.", name)
	case "Service":
		return fmt.Sprintf("Service %q is unhealthy. Check its endpoints and the health of backing pods.", name)
	default:
		return fmt.Sprintf("%s %q is unhealthy. Inspect events and the live manifest for more detail.", kind, name)
	}
}

// resourceHealthToolCall returns the suggested tool call for an unhealthy resource.
func resourceHealthToolCall(kind, name, namespace, appName string) string {
	switch kind {
	case "Pod":
		args := map[string]interface{}{"name": appName, "pod_name": name, "kind": "Pod"}
		if namespace != "" {
			args["namespace"] = namespace
		}
		return yamlToolCall("get_logs", args)
	default:
		args := map[string]interface{}{"name": appName, "kind": kind, "resource_name": name}
		if namespace != "" {
			args["namespace"] = namespace
		}
		return yamlToolCall("get_application_events", args)
	}
}

// eventRemediationHint maps known Kubernetes event reasons to actionable hints.
func eventRemediationHint(reason, kind, appName string) string {
	switch reason {
	case "BackOff", "CrashLoopBackOff":
		return "Container is crash-looping. Retrieve pod logs (including previous container) to identify the crash cause."
	case "OOMKilling", "OOMKilled":
		return "Container was OOM-killed. Increase its memory limits in the Git source and re-sync, or investigate memory leaks via logs."
	case "Failed", "FailedMount":
		return "Mount failure detected. Verify PVC/Secret/ConfigMap referenced by the pod exists and is bound."
	case "Pulling", "Failed to pull image", "ErrImagePull", "ImagePullBackOff":
		return "Image pull failure. Verify the image tag exists in the registry and that image pull credentials are correctly configured."
	case "Killing", "Preempting":
		return "Pod is being evicted or preempted. Check node resource pressure and consider raising pod priority or increasing node capacity."
	case "Unhealthy":
		return "Liveness/readiness probe failing. Review probe configuration and application startup time."
	case "FailedCreate":
		return "Pod creation failed. Check resource quotas, LimitRange, PodDisruptionBudgets, and admission webhook logs."
	default:
		return fmt.Sprintf("Warning event %q detected. Investigate with get_application_events for full context.", reason)
	}
}

// eventRemediationToolCall maps known event reasons to specific tool calls.
func eventRemediationToolCall(reason, appName string) string {
	switch reason {
	case "BackOff", "CrashLoopBackOff":
		return yamlToolCall("get_logs", map[string]interface{}{"name": appName, "previous": true})
	case "OOMKilling", "OOMKilled":
		return yamlToolCall("get_logs", map[string]interface{}{"name": appName, "filter": "OOM|memory"})
	case "ErrImagePull", "ImagePullBackOff":
		return yamlToolCall("get_application_events", map[string]interface{}{"name": appName, "kind": "Pod"})
	default:
		return yamlToolCall("get_application_events", map[string]interface{}{"name": appName})
	}
}

// buildNextActions produces a prioritised ordered list of actions for the operator.
func buildNextActions(r DiagnosticReport) []string {
	if r.Severity == SeverityHealthy {
		return []string{"No immediate action required. Application is healthy and synced."}
	}

	var actions []string
	seen := make(map[string]bool)

	for _, cause := range r.RootCauses {
		if cause.Remediation == "" {
			continue
		}
		key := cause.ToolCall
		if key == "" {
			key = cause.Signal
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		action := cause.Remediation
		if cause.ToolCall != "" {
			action = fmt.Sprintf("%s\n  Suggested call:\n%s", action, indentLines(cause.ToolCall, "    "))
		}
		actions = append(actions, action)
	}

	if len(actions) == 0 {
		actions = append(actions, fmt.Sprintf(
			"Run get_application_events name=%s to gather more context.", r.Application))
	}

	return actions
}

// buildDiagnosticSummary produces a plain-English paragraph summarising the incident.
func buildDiagnosticSummary(r DiagnosticReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Application %q is %s (health: %s, sync: %s)",
		r.Application, strings.ToUpper(string(r.Severity)), r.HealthStatus, r.SyncStatus))

	if r.CurrentRevision != "" {
		sb.WriteString(fmt.Sprintf(", currently at revision %s", shortSHA(r.CurrentRevision)))
	}
	sb.WriteString(". ")

	if len(r.OutOfSyncResources) > 0 {
		sb.WriteString(fmt.Sprintf("%d resource(s) are out-of-sync with Git: %s. ",
			len(r.OutOfSyncResources), strings.Join(r.OutOfSyncResources, ", ")))
	}

	if len(r.UnhealthyResources) > 0 {
		sb.WriteString(fmt.Sprintf("%d resource(s) are unhealthy: %s. ",
			len(r.UnhealthyResources), strings.Join(r.UnhealthyResources, ", ")))
	}

	if len(r.RootCauses) == 0 {
		sb.WriteString("No specific root cause signals were identified from available data sources.")
	} else {
		sb.WriteString(fmt.Sprintf("%d root cause signal(s) identified. Top signal: %s",
			len(r.RootCauses), r.RootCauses[0].Signal))
		if r.RootCauses[0].Detail != "" {
			excerpt := truncateString(r.RootCauses[0].Detail, 200)
			sb.WriteString(fmt.Sprintf(" (%s)", excerpt))
		}
		sb.WriteString(".")
	}

	return sb.String()
}

// --- Event parsing helpers ---

// extractWarningEvents parses the raw events interface into a structured list,
// keeping only events with a non-Normal type (i.e. Warning events).
func extractWarningEvents(raw interface{}) ([]parsedEvent, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal events: %w", err)
	}

	// The ArgoCD events response wraps items in a "items" array.
	type rawEventList struct {
		Items []struct {
			Type           string `json:"type"`
			Reason         string `json:"reason"`
			Message        string `json:"message"`
			InvolvedObject struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"involvedObject"`
		} `json:"items"`
	}
	var evtList rawEventList
	if err := json.Unmarshal(data, &evtList); err != nil {
		return nil, fmt.Errorf("failed to parse event list: %w", err)
	}

	var result []parsedEvent
	for _, item := range evtList.Items {
		if item.Type == "Normal" {
			continue
		}
		result = append(result, parsedEvent{
			Type:         item.Type,
			Reason:       item.Reason,
			Message:      item.Message,
			ResourceKind: item.InvolvedObject.Kind,
			ResourceName: item.InvolvedObject.Name,
		})
	}
	return result, nil
}

// --- Log analysis helpers ---

// extractErrorLines scans multi-pod log text and returns lines that contain
// error-related keywords.
func extractErrorLines(logs string, maxLines int) []string {
	errorKeywords := []string{"error", "Error", "ERROR", "exception", "Exception", "EXCEPTION",
		"panic", "PANIC", "fatal", "FATAL", "OOMKilled", "crash", "CRASH", "fail", "FAIL"}

	var result []string
	for _, line := range strings.Split(logs, "\n") {
		if len(result) >= maxLines {
			break
		}
		for _, kw := range errorKeywords {
			if strings.Contains(line, kw) {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					result = append(result, trimmed)
					break
				}
			}
		}
	}
	return result
}

// --- Formatting helpers ---

// yamlToolCall renders a tool name and args map as a compact YAML block
// that a user or LLM can paste directly into an MCP call.
func yamlToolCall(toolName string, args map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("tool: %s\n", toolName))
	sb.WriteString("args:\n")
	for k, v := range args {
		switch val := v.(type) {
		case bool:
			sb.WriteString(fmt.Sprintf("  %s: %t\n", k, val))
		case int, int64:
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, val))
		default:
			sb.WriteString(fmt.Sprintf("  %s: %q\n", k, fmt.Sprintf("%v", val)))
		}
	}
	return sb.String()
}

// indentLines prepends a prefix to every line of s.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// shortSHA truncates a Git SHA to its first 8 characters for display.
func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// Ensure the existing client.MaxLogEntries constant is accessible in this file.
var _ = client.MaxLogEntries
