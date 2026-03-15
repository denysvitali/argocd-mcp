package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// --- Kubernetes metrics client interface (allows mocking in tests) ---

// KubeMetricsClient abstracts Kubernetes pod metrics retrieval.
type KubeMetricsClient interface {
	// GetPodMetrics returns metrics for all pods in a namespace matching the label selector.
	GetPodMetrics(ctx context.Context, namespace, labelSelector string) (*metricsv1beta1.PodMetricsList, error)
	// GetNamespacePods returns all pods in a namespace matching the label selector.
	GetNamespacePods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)
}

// realKubeMetricsClient wraps the real Kubernetes clients.
type realKubeMetricsClient struct {
	metricsClient *metricsclient.Clientset
	kubeClient    *kubernetes.Clientset
}

func (r *realKubeMetricsClient) GetPodMetrics(ctx context.Context, namespace, labelSelector string) (*metricsv1beta1.PodMetricsList, error) {
	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}
	return r.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, opts)
}

func (r *realKubeMetricsClient) GetNamespacePods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}
	return r.kubeClient.CoreV1().Pods(namespace).List(ctx, opts)
}

// NewKubeMetricsClientFromConfig builds a KubeMetricsClient from in-cluster config or
// the given kubeconfig path (empty string = use ~/.kube/config or KUBECONFIG env var).
func NewKubeMetricsClientFromConfig(kubeconfigPath string) (KubeMetricsClient, error) {
	var cfg *rest.Config
	var err error

	if kubeconfigPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		// Try in-cluster first, fall back to default kubeconfig.
		cfg, err = rest.InClusterConfig()
		if err != nil {
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	metricsCS, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics clientset: %w", err)
	}

	kubeCS, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube clientset: %w", err)
	}

	return &realKubeMetricsClient{
		metricsClient: metricsCS,
		kubeClient:    kubeCS,
	}, nil
}

// --- Output types (no map[string]interface{} per project rules) ---

// ContainerEfficiency holds efficiency data for a single container.
type ContainerEfficiency struct {
	ContainerName string `json:"container_name" yaml:"container_name"`

	// Declared resource requests/limits (from the live manifest).
	CPURequestMillis    int64  `json:"cpu_request_millis" yaml:"cpu_request_millis"`
	MemoryRequestBytes  int64  `json:"memory_request_bytes" yaml:"memory_request_bytes"`
	CPULimitMillis      int64  `json:"cpu_limit_millis,omitempty" yaml:"cpu_limit_millis,omitempty"`
	MemoryLimitBytes    int64  `json:"memory_limit_bytes,omitempty" yaml:"memory_limit_bytes,omitempty"`
	CPURequestFormatted string `json:"cpu_request" yaml:"cpu_request"`
	MemRequestFormatted string `json:"memory_request" yaml:"memory_request"`

	// Observed actual usage from Kubernetes Metrics API (may be zero if unavailable).
	CPUUsageMillis     int64  `json:"cpu_usage_millis,omitempty" yaml:"cpu_usage_millis,omitempty"`
	MemoryUsageBytes   int64  `json:"memory_usage_bytes,omitempty" yaml:"memory_usage_bytes,omitempty"`
	CPUUsageFormatted  string `json:"cpu_usage,omitempty" yaml:"cpu_usage,omitempty"`
	MemUsageFormatted  string `json:"memory_usage,omitempty" yaml:"memory_usage,omitempty"`
	MetricsAvailable   bool   `json:"metrics_available" yaml:"metrics_available"`

	// Efficiency ratios (usage / request). Empty if metrics unavailable.
	CPUEfficiencyPct float64 `json:"cpu_efficiency_pct,omitempty" yaml:"cpu_efficiency_pct,omitempty"`
	MemEfficiencyPct float64 `json:"mem_efficiency_pct,omitempty" yaml:"mem_efficiency_pct,omitempty"`

	// Flags and recommendations.
	CPUOverProvisioned bool   `json:"cpu_over_provisioned" yaml:"cpu_over_provisioned"`
	MemOverProvisioned bool   `json:"mem_over_provisioned" yaml:"mem_over_provisioned"`
	MissingRequests    bool   `json:"missing_requests" yaml:"missing_requests"`
	Recommendation     string `json:"recommendation,omitempty" yaml:"recommendation,omitempty"`

	// Suggested right-sized requests (usage * headroom factor).
	SuggestedCPURequest string `json:"suggested_cpu_request,omitempty" yaml:"suggested_cpu_request,omitempty"`
	SuggestedMemRequest string `json:"suggested_memory_request,omitempty" yaml:"suggested_memory_request,omitempty"`

	// Estimated monthly wasted cost at default cloud pricing.
	EstimatedMonthlyCPUWasteDollars float64 `json:"estimated_monthly_cpu_waste_dollars,omitempty" yaml:"estimated_monthly_cpu_waste_dollars,omitempty"`
	EstimatedMonthlyMemWasteDollars float64 `json:"estimated_monthly_mem_waste_dollars,omitempty" yaml:"estimated_monthly_mem_waste_dollars,omitempty"`
}

// WorkloadEfficiency holds efficiency data for a single workload (Deployment, StatefulSet, etc.).
type WorkloadEfficiency struct {
	WorkloadName string                `json:"workload_name" yaml:"workload_name"`
	WorkloadKind string                `json:"workload_kind" yaml:"workload_kind"`
	Namespace    string                `json:"namespace" yaml:"namespace"`
	Replicas     int32                 `json:"replicas" yaml:"replicas"`
	Containers   []ContainerEfficiency `json:"containers" yaml:"containers"`

	// Aggregated waste for the whole workload (requests * replicas - actual usage * replicas).
	TotalMonthlyCPUWasteDollars float64 `json:"total_monthly_cpu_waste_dollars,omitempty" yaml:"total_monthly_cpu_waste_dollars,omitempty"`
	TotalMonthlyMemWasteDollars float64 `json:"total_monthly_mem_waste_dollars,omitempty" yaml:"total_monthly_mem_waste_dollars,omitempty"`
}

// ResourceEfficiencyReport is the top-level response from analyze_resource_efficiency.
type ResourceEfficiencyReport struct {
	ApplicationName string               `json:"application_name" yaml:"application_name"`
	Namespace       string               `json:"namespace" yaml:"namespace"`
	MetricsAvailable bool                `json:"metrics_available" yaml:"metrics_available"`
	Workloads       []WorkloadEfficiency `json:"workloads" yaml:"workloads"`

	// Grand totals across all workloads in the app.
	TotalWorkloads         int     `json:"total_workloads" yaml:"total_workloads"`
	OverProvisionedCount   int     `json:"over_provisioned_count" yaml:"over_provisioned_count"`
	MissingRequestsCount   int     `json:"missing_requests_count" yaml:"missing_requests_count"`
	TotalMonthlyCPUWaste   float64 `json:"total_monthly_cpu_waste_dollars,omitempty" yaml:"total_monthly_cpu_waste_dollars,omitempty"`
	TotalMonthlyMemWaste   float64 `json:"total_monthly_mem_waste_dollars,omitempty" yaml:"total_monthly_mem_waste_dollars,omitempty"`
	TotalMonthlyWaste      float64 `json:"total_monthly_waste_dollars,omitempty" yaml:"total_monthly_waste_dollars,omitempty"`

	// Cost model used for estimates.
	CostModelCPUPerVCPUHour float64 `json:"cost_model_cpu_per_vcpu_hour" yaml:"cost_model_cpu_per_vcpu_hour"`
	CostModelMemPerGBHour   float64 `json:"cost_model_mem_per_gb_hour" yaml:"cost_model_mem_per_gb_hour"`

	// Plain-English summary for LLM consumption.
	Summary string `json:"summary" yaml:"summary"`
}

// --- Cost model defaults (AWS/GCP/Azure general-purpose on-demand blended average) ---
const (
	defaultCPUCostPerVCPUHour = 0.048  // USD per vCPU-hour
	defaultMemCostPerGBHour   = 0.006  // USD per GB-hour
	hoursPerMonth             = 730.0

	// Headroom factor added on top of observed usage for right-sizing suggestions.
	rightSizingHeadroomFactor = 1.20

	// Thresholds below which a resource is flagged as over-provisioned.
	cpuOverProvisionedThreshold = 0.40 // using <40% of requested CPU
	memOverProvisionedThreshold = 0.50 // using <50% of requested memory
)

// --- Live manifest types for JSON unmarshalling ---

// liveWorkload represents the subset of a Deployment/StatefulSet/DaemonSet live manifest
// that we need for resource analysis.
type liveWorkload struct {
	Kind     string          `json:"kind"`
	Metadata liveMetadata    `json:"metadata"`
	Spec     liveWorkloadSpec `json:"spec"`
}

type liveMetadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type liveWorkloadSpec struct {
	Replicas *int32        `json:"replicas"`
	Template liveTemplate  `json:"template"`
}

type liveTemplate struct {
	Metadata liveMetadata       `json:"metadata"`
	Spec     livePodSpec        `json:"spec"`
}

type livePodSpec struct {
	Containers []liveContainer `json:"containers"`
}

type liveContainer struct {
	Name      string             `json:"name"`
	Resources liveResourceReqs   `json:"resources"`
}

type liveResourceReqs struct {
	Requests map[string]string `json:"requests"`
	Limits   map[string]string `json:"limits"`
}

// aggUsage holds aggregated CPU/memory usage across all pods for a given container name.
type aggUsage struct {
	totalCPUMillis int64
	totalMemBytes  int64
	podCount       int
}

// containerUsage holds per-container CPU and memory usage from the metrics API.
type containerUsage struct {
	cpuMillis   int64
	memoryBytes int64
}

// --- Handler ---

func (tm *ToolManager) handleAnalyzeResourceEfficiency(ctx context.Context, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	appName := String(arguments, "name", "")
	if appName == "" {
		return errorResult("name is required"), nil
	}

	cpuCostPerVCPUHour := Float64(arguments, "cpu_cost_per_vcpu_hour", defaultCPUCostPerVCPUHour)
	memCostPerGBHour := Float64(arguments, "mem_cost_per_gb_hour", defaultMemCostPerGBHour)
	if cpuCostPerVCPUHour <= 0 {
		cpuCostPerVCPUHour = defaultCPUCostPerVCPUHour
	}
	if memCostPerGBHour <= 0 {
		memCostPerGBHour = defaultMemCostPerGBHour
	}

	// 1. Fetch the live managed resources for the app. Each resource's LiveState
	//    contains the full live manifest JSON, including resource requests/limits.
	managedResources, err := tm.client.GetManagedResources(ctx, appName)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to get managed resources for %q: %v", appName, err)), nil
	}

	// 2. Filter to workload kinds and parse their live manifests.
	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	type parsedWorkload struct {
		liveWorkload
		kind string
	}

	var workloads []parsedWorkload
	for _, res := range managedResources {
		if res == nil || !workloadKinds[res.Kind] {
			continue
		}
		if res.LiveState == "" {
			continue
		}
		var wl liveWorkload
		if parseErr := json.Unmarshal([]byte(res.LiveState), &wl); parseErr != nil {
			tm.logger.Warnf("analyze_resource_efficiency: failed to parse live state for %s/%s: %v", res.Kind, res.Name, parseErr)
			continue
		}
		wl.Kind = res.Kind
		workloads = append(workloads, parsedWorkload{liveWorkload: wl, kind: res.Kind})
	}

	// 3. Determine the app namespace (from the first workload found, or from ArgoCD app spec).
	appNamespace := ""
	if len(workloads) > 0 {
		appNamespace = workloads[0].Metadata.Namespace
	}

	// 4. Attempt to fetch metrics for the app namespace.
	metricsAvailable := tm.kubeMetrics != nil
	// podMetrics maps pod name -> container name -> usage quantities.
	podMetrics := map[string]map[string]containerUsage{}

	if metricsAvailable && appNamespace != "" {
		podMetricsList, metricsErr := tm.kubeMetrics.GetPodMetrics(ctx, appNamespace, "")
		if metricsErr != nil {
			tm.logger.Warnf("analyze_resource_efficiency: metrics API unavailable for namespace %q: %v", appNamespace, metricsErr)
			metricsAvailable = false
		} else {
			for i := range podMetricsList.Items {
				pm := &podMetricsList.Items[i]
				containers := map[string]containerUsage{}
				for _, cm := range pm.Containers {
					cpuQ := cm.Usage[corev1.ResourceCPU]
					memQ := cm.Usage[corev1.ResourceMemory]
					containers[cm.Name] = containerUsage{
						cpuMillis:   cpuQ.MilliValue(),
						memoryBytes: memQ.Value(),
					}
				}
				podMetrics[pm.Name] = containers
			}
		}
	}

	// 5. For each workload, fetch pod names (to join with metrics) and build the report.
	podsByWorkload := map[string][]string{} // workload name -> list of pod names
	if metricsAvailable && tm.kubeMetrics != nil && appNamespace != "" {
		// Get all pods in the namespace; we'll match by owner reference label patterns.
		podList, podErr := tm.kubeMetrics.GetNamespacePods(ctx, appNamespace, "")
		if podErr != nil {
			tm.logger.Warnf("analyze_resource_efficiency: failed to list pods in %q: %v", appNamespace, podErr)
		} else {
			for i := range podList.Items {
				pod := &podList.Items[i]
				for _, owner := range pod.OwnerReferences {
					// Pods owned by ReplicaSets (which belong to Deployments) use
					// the RS name which includes the deployment name as prefix.
					// We match conservatively: if any owner name starts with workload name.
					for _, wl := range workloads {
						wlName := wl.Metadata.Name
						if owner.Name == wlName || (len(owner.Name) > len(wlName) && owner.Name[:len(wlName)] == wlName) {
							podsByWorkload[wlName] = append(podsByWorkload[wlName], pod.Name)
						}
					}
				}
			}
		}
	}

	// 6. Build per-workload efficiency data.
	report := ResourceEfficiencyReport{
		ApplicationName:         appName,
		Namespace:               appNamespace,
		MetricsAvailable:        metricsAvailable,
		CostModelCPUPerVCPUHour: cpuCostPerVCPUHour,
		CostModelMemPerGBHour:   memCostPerGBHour,
	}

	for _, wl := range workloads {
		replicas := int32(1)
		if wl.Spec.Replicas != nil {
			replicas = *wl.Spec.Replicas
		}

		wlReport := WorkloadEfficiency{
			WorkloadName: wl.Metadata.Name,
			WorkloadKind: wl.kind,
			Namespace:    wl.Metadata.Namespace,
			Replicas:     replicas,
		}

		// Aggregate per-container usage across pods belonging to this workload.
		containerAgg := map[string]*aggUsage{}
		if metricsAvailable {
			for _, podName := range podsByWorkload[wl.Metadata.Name] {
				if containerMap, ok := podMetrics[podName]; ok {
					for cName, usage := range containerMap {
						if _, exists := containerAgg[cName]; !exists {
							containerAgg[cName] = &aggUsage{}
						}
						containerAgg[cName].totalCPUMillis += usage.cpuMillis
						containerAgg[cName].totalMemBytes += usage.memoryBytes
						containerAgg[cName].podCount++
					}
				}
			}
		}

		for _, c := range wl.Spec.Template.Spec.Containers {
			ce := buildContainerEfficiency(c, containerAgg[c.Name], replicas, cpuCostPerVCPUHour, memCostPerGBHour)
			wlReport.Containers = append(wlReport.Containers, ce)
			wlReport.TotalMonthlyCPUWasteDollars += ce.EstimatedMonthlyCPUWasteDollars
			wlReport.TotalMonthlyMemWasteDollars += ce.EstimatedMonthlyMemWasteDollars

			if ce.MissingRequests {
				report.MissingRequestsCount++
			}
			if ce.CPUOverProvisioned || ce.MemOverProvisioned {
				report.OverProvisionedCount++
			}
		}

		report.Workloads = append(report.Workloads, wlReport)
		report.TotalMonthlyCPUWaste += wlReport.TotalMonthlyCPUWasteDollars
		report.TotalMonthlyMemWaste += wlReport.TotalMonthlyMemWasteDollars
		report.TotalWorkloads++
	}

	report.TotalMonthlyWaste = report.TotalMonthlyCPUWaste + report.TotalMonthlyMemWaste
	report.Summary = buildEfficiencySummary(&report)

	return Result(report, nil)
}

// buildContainerEfficiency constructs a ContainerEfficiency for one container.
// agg is the aggregated actual usage across pods (nil if metrics unavailable).
func buildContainerEfficiency(c liveContainer, agg *aggUsage, replicas int32, cpuCost, memCost float64) ContainerEfficiency {
	ce := ContainerEfficiency{
		ContainerName: c.Name,
	}

	// Parse declared requests.
	if cpuReqStr, ok := c.Resources.Requests["cpu"]; ok {
		if q, err := resource.ParseQuantity(cpuReqStr); err == nil {
			ce.CPURequestMillis = q.MilliValue()
			ce.CPURequestFormatted = formatMilliCPU(ce.CPURequestMillis)
		}
	} else {
		ce.MissingRequests = true
	}
	if memReqStr, ok := c.Resources.Requests["memory"]; ok {
		if q, err := resource.ParseQuantity(memReqStr); err == nil {
			ce.MemoryRequestBytes = q.Value()
			ce.MemRequestFormatted = formatBytes(ce.MemoryRequestBytes)
		}
	} else {
		ce.MissingRequests = true
	}

	// Parse declared limits (informational only).
	if cpuLimStr, ok := c.Resources.Limits["cpu"]; ok {
		if q, err := resource.ParseQuantity(cpuLimStr); err == nil {
			ce.CPULimitMillis = q.MilliValue()
		}
	}
	if memLimStr, ok := c.Resources.Limits["memory"]; ok {
		if q, err := resource.ParseQuantity(memLimStr); err == nil {
			ce.MemoryLimitBytes = q.Value()
		}
	}

	// Compute efficiency if metrics are available.
	if agg != nil && agg.podCount > 0 {
		ce.MetricsAvailable = true
		avgCPUMillis := agg.totalCPUMillis / int64(agg.podCount)
		avgMemBytes := agg.totalMemBytes / int64(agg.podCount)

		ce.CPUUsageMillis = avgCPUMillis
		ce.MemoryUsageBytes = avgMemBytes
		ce.CPUUsageFormatted = formatMilliCPU(avgCPUMillis)
		ce.MemUsageFormatted = formatBytes(avgMemBytes)

		if ce.CPURequestMillis > 0 {
			ce.CPUEfficiencyPct = roundPct(float64(avgCPUMillis) / float64(ce.CPURequestMillis) * 100)
			ce.CPUOverProvisioned = ce.CPUEfficiencyPct < cpuOverProvisionedThreshold*100

			// Suggest right-sized request: observed avg * headroom, per replica.
			suggestedCPU := int64(math.Ceil(float64(avgCPUMillis) * rightSizingHeadroomFactor))
			ce.SuggestedCPURequest = formatMilliCPU(suggestedCPU)

			// Waste = (requested - actual_avg) per pod * replicas * hours * cost.
			wastedCPUMillisPerPod := ce.CPURequestMillis - avgCPUMillis
			if wastedCPUMillisPerPod > 0 {
				wastedVCPUs := float64(wastedCPUMillisPerPod) / 1000.0 * float64(replicas)
				ce.EstimatedMonthlyCPUWasteDollars = roundDollars(wastedVCPUs * hoursPerMonth * cpuCost)
			}
		}

		if ce.MemoryRequestBytes > 0 {
			ce.MemEfficiencyPct = roundPct(float64(avgMemBytes) / float64(ce.MemoryRequestBytes) * 100)
			ce.MemOverProvisioned = ce.MemEfficiencyPct < memOverProvisionedThreshold*100

			suggestedMemBytes := int64(math.Ceil(float64(avgMemBytes) * rightSizingHeadroomFactor))
			ce.SuggestedMemRequest = formatBytes(suggestedMemBytes)

			wastedMemBytesPerPod := ce.MemoryRequestBytes - avgMemBytes
			if wastedMemBytesPerPod > 0 {
				wastedGB := float64(wastedMemBytesPerPod) / (1024 * 1024 * 1024) * float64(replicas)
				ce.EstimatedMonthlyMemWasteDollars = roundDollars(wastedGB * hoursPerMonth * memCost)
			}
		}
	}

	// Build a human-readable recommendation.
	ce.Recommendation = buildContainerRecommendation(ce)

	return ce
}

func buildContainerRecommendation(ce ContainerEfficiency) string {
	if ce.MissingRequests {
		return "Missing resource requests: set CPU and memory requests to enable the scheduler to make good placement decisions and to allow efficiency analysis."
	}
	if !ce.MetricsAvailable {
		return "Metrics API unavailable: install metrics-server to enable live usage analysis and right-sizing recommendations."
	}
	if ce.CPUOverProvisioned && ce.MemOverProvisioned {
		return fmt.Sprintf(
			"Severely over-provisioned: CPU at %.0f%% efficiency, memory at %.0f%% efficiency. "+
				"Consider reducing requests to CPU=%s, memory=%s (observed average + 20%% headroom). "+
				"Estimated monthly savings: $%.2f CPU + $%.2f memory.",
			ce.CPUEfficiencyPct, ce.MemEfficiencyPct,
			ce.SuggestedCPURequest, ce.SuggestedMemRequest,
			ce.EstimatedMonthlyCPUWasteDollars, ce.EstimatedMonthlyMemWasteDollars,
		)
	}
	if ce.CPUOverProvisioned {
		return fmt.Sprintf(
			"CPU over-provisioned at %.0f%% efficiency. Consider reducing CPU request to %s. "+
				"Estimated monthly savings: $%.2f.",
			ce.CPUEfficiencyPct, ce.SuggestedCPURequest, ce.EstimatedMonthlyCPUWasteDollars,
		)
	}
	if ce.MemOverProvisioned {
		return fmt.Sprintf(
			"Memory over-provisioned at %.0f%% efficiency. Consider reducing memory request to %s. "+
				"Estimated monthly savings: $%.2f.",
			ce.MemEfficiencyPct, ce.SuggestedMemRequest, ce.EstimatedMonthlyMemWasteDollars,
		)
	}
	return fmt.Sprintf(
		"Well-sized: CPU at %.0f%% efficiency, memory at %.0f%% efficiency.",
		ce.CPUEfficiencyPct, ce.MemEfficiencyPct,
	)
}

func buildEfficiencySummary(r *ResourceEfficiencyReport) string {
	if r.TotalWorkloads == 0 {
		return fmt.Sprintf("No workloads (Deployments/StatefulSets/DaemonSets) found in application %q.", r.ApplicationName)
	}
	if !r.MetricsAvailable {
		return fmt.Sprintf(
			"Analyzed %d workload(s) in application %q (namespace: %s). "+
				"Live usage metrics are not available - install metrics-server to enable right-sizing recommendations. "+
				"%d workload(s) have missing resource requests.",
			r.TotalWorkloads, r.ApplicationName, r.Namespace, r.MissingRequestsCount,
		)
	}
	return fmt.Sprintf(
		"Analyzed %d workload(s) in application %q (namespace: %s). "+
			"%d container(s) are over-provisioned. "+
			"%d container(s) have missing resource requests. "+
			"Estimated total monthly waste: $%.2f (CPU: $%.2f, memory: $%.2f) "+
			"at $%.3f/vCPU-hour and $%.4f/GB-hour.",
		r.TotalWorkloads, r.ApplicationName, r.Namespace,
		r.OverProvisionedCount, r.MissingRequestsCount,
		r.TotalMonthlyWaste, r.TotalMonthlyCPUWaste, r.TotalMonthlyMemWaste,
		r.CostModelCPUPerVCPUHour, r.CostModelMemPerGBHour,
	)
}

// --- Formatting helpers ---

func formatMilliCPU(millis int64) string {
	if millis == 0 {
		return "0m"
	}
	if millis >= 1000 {
		// Express as fractional cores if >= 1 core.
		cores := float64(millis) / 1000.0
		if cores == float64(int64(cores)) {
			return fmt.Sprintf("%d", int64(cores))
		}
		return fmt.Sprintf("%.3g", cores)
	}
	return fmt.Sprintf("%dm", millis)
}

func formatBytes(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.2fGi", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.2fMi", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.2fKi", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func roundPct(v float64) float64 {
	return math.Round(v*100) / 100
}

func roundDollars(v float64) float64 {
	return math.Round(v*100) / 100
}
