package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// --- Mock metrics client ---

type mockKubeMetricsClient struct {
	PodMetricsFn  func(ctx context.Context, namespace, labelSelector string) (*metricsv1beta1.PodMetricsList, error)
	NamespacePodsFn func(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error)
}

func (m *mockKubeMetricsClient) GetPodMetrics(ctx context.Context, namespace, labelSelector string) (*metricsv1beta1.PodMetricsList, error) {
	if m.PodMetricsFn != nil {
		return m.PodMetricsFn(ctx, namespace, labelSelector)
	}
	return &metricsv1beta1.PodMetricsList{}, nil
}

func (m *mockKubeMetricsClient) GetNamespacePods(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	if m.NamespacePodsFn != nil {
		return m.NamespacePodsFn(ctx, namespace, labelSelector)
	}
	return &corev1.PodList{}, nil
}

// --- Helpers ---

// makeLiveDeploymentJSON returns a JSON string representing a live Deployment manifest.
func makeLiveDeploymentJSON(name, namespace string, replicas int32, containers []liveContainer) string {
	wl := liveWorkload{
		Kind: "Deployment",
		Metadata: liveMetadata{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name},
		},
		Spec: liveWorkloadSpec{
			Replicas: &replicas,
			Template: liveTemplate{
				Metadata: liveMetadata{Labels: map[string]string{"app": name}},
				Spec:     livePodSpec{Containers: containers},
			},
		},
	}
	data, _ := json.Marshal(wl)
	return string(data)
}

func testToolManagerWithMetrics(mock *MockArgoClient, metricsClient KubeMetricsClient) *ToolManager {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return NewToolManagerWithMetrics(mock, metricsClient, logger, false)
}

// --- Tests ---

func TestAnalyzeResourceEfficiency_MissingName(t *testing.T) {
	tm := testToolManager(&MockArgoClient{}, false)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestAnalyzeResourceEfficiency_NoWorkloads(t *testing.T) {
	// App with only a Service resource (no Deployments), so no workloads are found.
	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, appName string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Service", Name: "my-svc", LiveState: `{"kind":"Service","metadata":{"name":"my-svc","namespace":"default"}}`},
			}, nil
		},
	}
	tm := testToolManager(mock, false)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name": "my-app",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.Equal(t, "my-app", data["application_name"])
	assert.EqualValues(t, 0, data["total_workloads"])
	assert.Contains(t, data["summary"].(string), "No workloads")
}

func TestAnalyzeResourceEfficiency_NoMetricsClient(t *testing.T) {
	// Workload with requests, but no metrics client provided.
	containers := []liveContainer{
		{
			Name: "app",
			Resources: liveResourceReqs{
				Requests: map[string]string{"cpu": "500m", "memory": "256Mi"},
				Limits:   map[string]string{"cpu": "1", "memory": "512Mi"},
			},
		},
	}
	liveState := makeLiveDeploymentJSON("my-deploy", "default", 2, containers)

	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Deployment", Name: "my-deploy", LiveState: liveState},
			}, nil
		},
	}

	// No metrics client — kubeMetrics is nil.
	tm := testToolManager(mock, false)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name": "my-app",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.Equal(t, "my-app", data["application_name"])
	assert.EqualValues(t, 1, data["total_workloads"])
	assert.False(t, data["metrics_available"].(bool))
	assert.Contains(t, data["summary"].(string), "metrics are not available")
}

func TestAnalyzeResourceEfficiency_WithMetrics_OverProvisioned(t *testing.T) {
	// Container requests 2 CPUs and 1Gi memory, but only uses 100m CPU and 100Mi memory.
	containers := []liveContainer{
		{
			Name: "app",
			Resources: liveResourceReqs{
				Requests: map[string]string{"cpu": "2", "memory": "1Gi"},
				Limits:   map[string]string{"cpu": "4", "memory": "2Gi"},
			},
		},
	}
	var replicas int32 = 3
	liveState := makeLiveDeploymentJSON("over-provisioned-deploy", "production", replicas, containers)

	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Deployment", Name: "over-provisioned-deploy", LiveState: liveState},
			}, nil
		},
	}

	metricsClient := &mockKubeMetricsClient{
		PodMetricsFn: func(_ context.Context, namespace, _ string) (*metricsv1beta1.PodMetricsList, error) {
			assert.Equal(t, "production", namespace)
			// Three pods, each using 100m CPU and 100Mi memory.
			makePodMetrics := func(podName string) metricsv1beta1.PodMetrics {
				return metricsv1beta1.PodMetrics{
					ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace},
					Containers: []metricsv1beta1.ContainerMetrics{
						{
							Name: "app",
							Usage: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				}
			}
			return &metricsv1beta1.PodMetricsList{
				Items: []metricsv1beta1.PodMetrics{
					makePodMetrics("over-provisioned-deploy-abc-1"),
					makePodMetrics("over-provisioned-deploy-abc-2"),
					makePodMetrics("over-provisioned-deploy-abc-3"),
				},
			}, nil
		},
		NamespacePodsFn: func(_ context.Context, namespace, _ string) (*corev1.PodList, error) {
			// Return three pods owned by the deployment's ReplicaSet.
			makePod := func(podName string) corev1.Pod {
				return corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podName,
						Namespace: namespace,
						OwnerReferences: []metav1.OwnerReference{
							{Name: "over-provisioned-deploy-abc", Kind: "ReplicaSet"},
						},
					},
				}
			}
			return &corev1.PodList{
				Items: []corev1.Pod{
					makePod("over-provisioned-deploy-abc-1"),
					makePod("over-provisioned-deploy-abc-2"),
					makePod("over-provisioned-deploy-abc-3"),
				},
			}, nil
		},
	}

	tm := testToolManagerWithMetrics(mock, metricsClient)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name": "cost-heavy-app",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.True(t, data["metrics_available"].(bool))
	assert.EqualValues(t, 1, data["total_workloads"])
	assert.EqualValues(t, 1, data["over_provisioned_count"])

	// Waste estimates must be positive.
	cpuWaste := data["total_monthly_cpu_waste_dollars"].(float64)
	memWaste := data["total_monthly_mem_waste_dollars"].(float64)
	assert.Greater(t, cpuWaste, 0.0, "should report positive CPU waste dollars")
	assert.Greater(t, memWaste, 0.0, "should report positive memory waste dollars")

	// Verify per-workload breakdown.
	workloads := data["workloads"].([]interface{})
	require.Len(t, workloads, 1)
	wl := workloads[0].(map[string]interface{})
	assert.Equal(t, "over-provisioned-deploy", wl["workload_name"])
	assert.EqualValues(t, 3, wl["replicas"])

	// Verify the container is flagged.
	wlContainers := wl["containers"].([]interface{})
	require.Len(t, wlContainers, 1)
	c := wlContainers[0].(map[string]interface{})
	assert.True(t, c["cpu_over_provisioned"].(bool))
	assert.True(t, c["mem_over_provisioned"].(bool))
	assert.NotEmpty(t, c["suggested_cpu_request"])
	assert.NotEmpty(t, c["suggested_memory_request"])
}

func TestAnalyzeResourceEfficiency_MissingRequests(t *testing.T) {
	// Container with no resource requests set at all.
	containers := []liveContainer{
		{
			Name:      "no-limits",
			Resources: liveResourceReqs{},
		},
	}
	var replicas int32 = 1
	liveState := makeLiveDeploymentJSON("unrestricted-deploy", "staging", replicas, containers)

	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Deployment", Name: "unrestricted-deploy", LiveState: liveState},
			}, nil
		},
	}

	tm := testToolManager(mock, false)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name": "unconstrained-app",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.EqualValues(t, 1, data["missing_requests_count"])
	workloads := data["workloads"].([]interface{})
	wl := workloads[0].(map[string]interface{})
	containers2 := wl["containers"].([]interface{})
	c := containers2[0].(map[string]interface{})
	assert.True(t, c["missing_requests"].(bool))
	assert.Contains(t, c["recommendation"].(string), "Missing resource requests")
}

func TestAnalyzeResourceEfficiency_WellSized(t *testing.T) {
	// Container using 80% of its CPU and 70% of its memory requests — not over-provisioned.
	containers := []liveContainer{
		{
			Name: "web",
			Resources: liveResourceReqs{
				Requests: map[string]string{"cpu": "500m", "memory": "512Mi"},
			},
		},
	}
	var replicas int32 = 2
	liveState := makeLiveDeploymentJSON("well-sized-deploy", "default", replicas, containers)

	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Deployment", Name: "well-sized-deploy", LiveState: liveState},
			}, nil
		},
	}

	metricsClient := &mockKubeMetricsClient{
		PodMetricsFn: func(_ context.Context, _, _ string) (*metricsv1beta1.PodMetricsList, error) {
			return &metricsv1beta1.PodMetricsList{
				Items: []metricsv1beta1.PodMetrics{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "well-sized-deploy-abc-1", Namespace: "default"},
						Containers: []metricsv1beta1.ContainerMetrics{
							{
								Name: "web",
								Usage: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("400m"),  // 80% of 500m
									corev1.ResourceMemory: resource.MustParse("358Mi"), // 70% of 512Mi
								},
							},
						},
					},
				},
			}, nil
		},
		NamespacePodsFn: func(_ context.Context, namespace, _ string) (*corev1.PodList, error) {
			return &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "well-sized-deploy-abc-1",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Name: "well-sized-deploy-abc", Kind: "ReplicaSet"},
							},
						},
					},
				},
			}, nil
		},
	}

	tm := testToolManagerWithMetrics(mock, metricsClient)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name": "well-sized-app",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.EqualValues(t, 0, data["over_provisioned_count"])

	workloads := data["workloads"].([]interface{})
	wl := workloads[0].(map[string]interface{})
	conts := wl["containers"].([]interface{})
	c := conts[0].(map[string]interface{})
	assert.False(t, c["cpu_over_provisioned"].(bool))
	assert.False(t, c["mem_over_provisioned"].(bool))
	assert.Contains(t, c["recommendation"].(string), "Well-sized")
}

func TestAnalyzeResourceEfficiency_CustomCostModel(t *testing.T) {
	// Verify that custom cost rates are reflected in the output.
	containers := []liveContainer{
		{
			Name: "api",
			Resources: liveResourceReqs{
				Requests: map[string]string{"cpu": "1", "memory": "1Gi"},
			},
		},
	}
	var replicas int32 = 1
	liveState := makeLiveDeploymentJSON("api-deploy", "default", replicas, containers)

	mock := &MockArgoClient{
		GetManagedResourcesFn: func(_ context.Context, _ string) ([]*v1alpha1.ResourceDiff, error) {
			return []*v1alpha1.ResourceDiff{
				{Kind: "Deployment", Name: "api-deploy", LiveState: liveState},
			}, nil
		},
	}

	metricsClient := &mockKubeMetricsClient{
		PodMetricsFn: func(_ context.Context, _, _ string) (*metricsv1beta1.PodMetricsList, error) {
			return &metricsv1beta1.PodMetricsList{
				Items: []metricsv1beta1.PodMetrics{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "api-deploy-abc-1", Namespace: "default"},
						Containers: []metricsv1beta1.ContainerMetrics{
							{
								Name: "api",
								Usage: corev1.ResourceList{
									// 10% CPU and 10% memory — severely over-provisioned.
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("102Mi"),
								},
							},
						},
					},
				},
			}, nil
		},
		NamespacePodsFn: func(_ context.Context, namespace, _ string) (*corev1.PodList, error) {
			return &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "api-deploy-abc-1",
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Name: "api-deploy-abc", Kind: "ReplicaSet"},
							},
						},
					},
				},
			}, nil
		},
	}

	// Use a 10x more expensive cloud.
	customCPUCost := 0.48
	customMemCost := 0.06

	tm := testToolManagerWithMetrics(mock, metricsClient)
	result, err := tm.CallTool(context.Background(), "analyze_resource_efficiency", map[string]interface{}{
		"name":                   "api-app",
		"cpu_cost_per_vcpu_hour": customCPUCost,
		"mem_cost_per_gb_hour":   customMemCost,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	data := parseResultYAML(t, result)
	assert.InDelta(t, customCPUCost, data["cost_model_cpu_per_vcpu_hour"].(float64), 0.001)
	assert.InDelta(t, customMemCost, data["cost_model_mem_per_gb_hour"].(float64), 0.001)

	// Waste must be ~10x what the default model would produce.
	defaultCPUWaste := (1.0 - 0.1) * hoursPerMonth * defaultCPUCostPerVCPUHour // 0.9 vCPU * 730h * $0.048
	customCPUWaste := (1.0 - 0.1) * hoursPerMonth * customCPUCost
	assert.InDelta(t, defaultCPUWaste*10, customCPUWaste, 0.5)
}

// --- Unit tests for formatting helpers ---

func TestFormatMilliCPU(t *testing.T) {
	tests := []struct {
		millis   int64
		expected string
	}{
		{0, "0m"},
		{100, "100m"},
		{500, "500m"},
		{1000, "1"},
		{2000, "2"},
		{1500, "1.5"},
		{250, "250m"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatMilliCPU(tt.millis))
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.00Ki"},
		{1024 * 1024, "1.00Mi"},
		{512 * 1024 * 1024, "512.00Mi"},
		{1024 * 1024 * 1024, "1.00Gi"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatBytes(tt.bytes))
		})
	}
}

func TestBuildContainerEfficiency_NoRequests(t *testing.T) {
	c := liveContainer{Name: "app", Resources: liveResourceReqs{}}
	ce := buildContainerEfficiency(c, nil, 1, defaultCPUCostPerVCPUHour, defaultMemCostPerGBHour)
	assert.True(t, ce.MissingRequests)
	assert.False(t, ce.MetricsAvailable)
	assert.Contains(t, ce.Recommendation, "Missing resource requests")
}

func TestBuildContainerEfficiency_NoMetrics(t *testing.T) {
	c := liveContainer{
		Name: "app",
		Resources: liveResourceReqs{
			Requests: map[string]string{"cpu": "500m", "memory": "256Mi"},
		},
	}
	ce := buildContainerEfficiency(c, nil, 2, defaultCPUCostPerVCPUHour, defaultMemCostPerGBHour)
	assert.False(t, ce.MissingRequests)
	assert.False(t, ce.MetricsAvailable)
	assert.EqualValues(t, 500, ce.CPURequestMillis)
	assert.EqualValues(t, 256*1024*1024, ce.MemoryRequestBytes)
	assert.Contains(t, ce.Recommendation, "Metrics API unavailable")
}

func TestBuildContainerEfficiency_OverProvisioned(t *testing.T) {
	c := liveContainer{
		Name: "app",
		Resources: liveResourceReqs{
			Requests: map[string]string{"cpu": "2000m", "memory": "2Gi"},
		},
	}
	// Using only 100m CPU (5%) and 200Mi memory (10%).
	agg := &aggUsage{
		totalCPUMillis: 100,
		totalMemBytes:  200 * 1024 * 1024,
		podCount:       1,
	}
	ce := buildContainerEfficiency(c, agg, 1, defaultCPUCostPerVCPUHour, defaultMemCostPerGBHour)
	assert.True(t, ce.CPUOverProvisioned)
	assert.True(t, ce.MemOverProvisioned)
	assert.InDelta(t, 5.0, ce.CPUEfficiencyPct, 0.1)
	assert.Greater(t, ce.EstimatedMonthlyCPUWasteDollars, 0.0)
	assert.Greater(t, ce.EstimatedMonthlyMemWasteDollars, 0.0)

	// Suggested request should be usage * 1.20 headroom.
	expectedSuggestedCPU := int64(100 * rightSizingHeadroomFactor) // 120m
	assert.Equal(t, formatMilliCPU(expectedSuggestedCPU), ce.SuggestedCPURequest)
}

func TestBuildContainerEfficiency_WellSized(t *testing.T) {
	c := liveContainer{
		Name: "app",
		Resources: liveResourceReqs{
			Requests: map[string]string{"cpu": "500m", "memory": "512Mi"},
		},
	}
	// Using 400m CPU (80%) and 358Mi memory (70%).
	agg := &aggUsage{
		totalCPUMillis: 400,
		totalMemBytes:  358 * 1024 * 1024,
		podCount:       1,
	}
	ce := buildContainerEfficiency(c, agg, 2, defaultCPUCostPerVCPUHour, defaultMemCostPerGBHour)
	assert.False(t, ce.CPUOverProvisioned)
	assert.False(t, ce.MemOverProvisioned)
	assert.InDelta(t, 80.0, ce.CPUEfficiencyPct, 0.1)
	assert.Contains(t, ce.Recommendation, "Well-sized")
}
