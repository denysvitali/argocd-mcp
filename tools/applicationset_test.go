package tools

import (
	"context"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/applicationset"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestToolManagerForAppSet(mock *MockArgoClient) *ToolManager {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return NewToolManager(mock, logger, false)
}

// --- list_applicationsets ---

func TestHandleListApplicationSets_Success(t *testing.T) {
	mock := &MockArgoClient{
		ListApplicationSetsFn: func(_ context.Context, query *applicationset.ApplicationSetListQuery) (*v1alpha1.ApplicationSetList, error) {
			return &v1alpha1.ApplicationSetList{
				Items: []v1alpha1.ApplicationSet{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "team-a-appset"},
						Spec: v1alpha1.ApplicationSetSpec{
							Generators: []v1alpha1.ApplicationSetGenerator{
								{Clusters: &v1alpha1.ClusterGenerator{}},
							},
							Template: v1alpha1.ApplicationSetTemplate{
								ApplicationSetTemplateMeta: v1alpha1.ApplicationSetTemplateMeta{Name: "{{cluster}}-app"},
								Spec:                       v1alpha1.ApplicationSpec{Project: "team-a"},
							},
						},
						Status: v1alpha1.ApplicationSetStatus{
							ResourcesCount: 3,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "preview-appset"},
						Spec: v1alpha1.ApplicationSetSpec{
							Generators: []v1alpha1.ApplicationSetGenerator{
								{PullRequest: &v1alpha1.PullRequestGenerator{}},
							},
							Template: v1alpha1.ApplicationSetTemplate{
								Spec: v1alpha1.ApplicationSpec{Project: "previews"},
							},
						},
					},
				},
			}, nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "list_applicationsets", map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "expected no error in result")

	text := parseResultText(t, result)
	assert.Contains(t, text, "team-a-appset")
	assert.Contains(t, text, "preview-appset")
	assert.Contains(t, text, "Clusters")
	assert.Contains(t, text, "PullRequest")
}

func TestHandleListApplicationSets_ProjectFilter(t *testing.T) {
	var capturedQuery *applicationset.ApplicationSetListQuery
	mock := &MockArgoClient{
		ListApplicationSetsFn: func(_ context.Context, query *applicationset.ApplicationSetListQuery) (*v1alpha1.ApplicationSetList, error) {
			capturedQuery = query
			return &v1alpha1.ApplicationSetList{}, nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	_, err := tm.CallTool(context.Background(), "list_applicationsets", map[string]interface{}{
		"project": "team-b",
	})
	require.NoError(t, err)
	require.NotNil(t, capturedQuery)
	assert.Equal(t, []string{"team-b"}, capturedQuery.Projects)
}

func TestHandleListApplicationSets_ClientError(t *testing.T) {
	mock := &MockArgoClient{
		ListApplicationSetsFn: func(_ context.Context, _ *applicationset.ApplicationSetListQuery) (*v1alpha1.ApplicationSetList, error) {
			return nil, assert.AnError
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "list_applicationsets", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- get_applicationset ---

func TestHandleGetApplicationSet_Success(t *testing.T) {
	mock := &MockArgoClient{
		GetApplicationSetFn: func(_ context.Context, query *applicationset.ApplicationSetGetQuery) (*v1alpha1.ApplicationSet, error) {
			assert.Equal(t, "my-appset", query.Name)
			return &v1alpha1.ApplicationSet{
				ObjectMeta: metav1.ObjectMeta{Name: "my-appset", Namespace: "argocd"},
				Spec: v1alpha1.ApplicationSetSpec{
					Generators: []v1alpha1.ApplicationSetGenerator{
						{Git: &v1alpha1.GitGenerator{RepoURL: "https://github.com/org/repo"}},
					},
					Template: v1alpha1.ApplicationSetTemplate{
						Spec: v1alpha1.ApplicationSpec{
							Project:     "production",
							Destination: v1alpha1.ApplicationDestination{Server: "https://k8s.example.com"},
						},
					},
					Strategy: &v1alpha1.ApplicationSetStrategy{
						RollingSync: &v1alpha1.ApplicationSetRolloutStrategy{},
					},
				},
				Status: v1alpha1.ApplicationSetStatus{
					ResourcesCount: 5,
					ApplicationStatus: []v1alpha1.ApplicationSetApplicationStatus{
						{Application: "prod-app-1", Status: "Healthy"},
						{Application: "prod-app-2", Status: "Progressing", Step: "1"},
					},
				},
			}, nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "get_applicationset", map[string]interface{}{
		"name": "my-appset",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := parseResultText(t, result)
	assert.Contains(t, text, "my-appset")
	assert.Contains(t, text, "Git")
	assert.Contains(t, text, "RollingSync")
	assert.Contains(t, text, "prod-app-1")
	assert.Contains(t, text, "production")
}

func TestHandleGetApplicationSet_MissingName(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "get_applicationset", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- preview_applicationset ---

func TestHandlePreviewApplicationSet_WithSpec(t *testing.T) {
	mock := &MockArgoClient{
		PreviewApplicationSetFn: func(_ context.Context, appSet *v1alpha1.ApplicationSet) ([]*v1alpha1.Application, error) {
			// Verify the spec was parsed correctly
			assert.Equal(t, "preview", appSet.Name)
			return []*v1alpha1.Application{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-a-myapp"},
					Spec: v1alpha1.ApplicationSpec{
						Project:     "team-x",
						Destination: v1alpha1.ApplicationDestination{Server: "https://cluster-a.k8s.io", Namespace: "myapp"},
						Source: &v1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/org/repo",
							Path:           "apps/myapp",
							TargetRevision: "main",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster-b-myapp"},
					Spec: v1alpha1.ApplicationSpec{
						Project:     "team-x",
						Destination: v1alpha1.ApplicationDestination{Server: "https://cluster-b.k8s.io", Namespace: "myapp"},
						Source: &v1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/org/repo",
							Path:           "apps/myapp",
							TargetRevision: "main",
						},
					},
				},
			}, nil
		},
	}

	spec := `
metadata:
  name: preview
spec:
  generators:
    - clusters: {}
  template:
    spec:
      project: team-x
`
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{
		"spec": spec,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, parseResultText(t, result))

	text := parseResultText(t, result)
	assert.Contains(t, text, "total_applications: 2")
	assert.Contains(t, text, "cluster-a-myapp")
	assert.Contains(t, text, "cluster-b-myapp")
	assert.Contains(t, text, "dry-run")
}

func TestHandlePreviewApplicationSet_WithExistingName(t *testing.T) {
	existingAppSet := &v1alpha1.ApplicationSet{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-appset"},
		Spec: v1alpha1.ApplicationSetSpec{
			Generators: []v1alpha1.ApplicationSetGenerator{{List: &v1alpha1.ListGenerator{}}},
			Template:   v1alpha1.ApplicationSetTemplate{Spec: v1alpha1.ApplicationSpec{Project: "default"}},
		},
	}
	var previewedAppSet *v1alpha1.ApplicationSet

	mock := &MockArgoClient{
		GetApplicationSetFn: func(_ context.Context, _ *applicationset.ApplicationSetGetQuery) (*v1alpha1.ApplicationSet, error) {
			return existingAppSet, nil
		},
		PreviewApplicationSetFn: func(_ context.Context, appSet *v1alpha1.ApplicationSet) ([]*v1alpha1.Application, error) {
			previewedAppSet = appSet
			return []*v1alpha1.Application{}, nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{
		"name": "existing-appset",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	require.NotNil(t, previewedAppSet)
	assert.Equal(t, "existing-appset", previewedAppSet.Name)
}

func TestHandlePreviewApplicationSet_BothParamsError(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{
		"spec": "some: yaml",
		"name": "some-name",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, parseResultText(t, result), "only one of")
}

func TestHandlePreviewApplicationSet_NeitherParamError(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandlePreviewApplicationSet_InvalidSpec(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{
		"spec": "{not valid yaml or json@@@",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandlePreviewApplicationSet_ClientError(t *testing.T) {
	mock := &MockArgoClient{
		PreviewApplicationSetFn: func(_ context.Context, _ *v1alpha1.ApplicationSet) ([]*v1alpha1.Application, error) {
			return nil, assert.AnError
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "preview_applicationset", map[string]interface{}{
		"spec": "metadata:\n  name: test\n",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, parseResultText(t, result), "preview failed")
}

// --- create_applicationset ---

func TestHandleCreateApplicationSet_SafeMode(t *testing.T) {
	mock := &MockArgoClient{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	tm := NewToolManager(mock, logger, true) // safe mode on

	result, err := tm.CallTool(context.Background(), "create_applicationset", map[string]interface{}{
		"spec": "metadata:\n  name: x\n",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	text := parseResultText(t, result)
	assert.Contains(t, text, "read-only mode")
}

func TestHandleCreateApplicationSet_Success(t *testing.T) {
	created := &v1alpha1.ApplicationSet{
		ObjectMeta: metav1.ObjectMeta{Name: "new-appset"},
		Spec: v1alpha1.ApplicationSetSpec{
			Generators: []v1alpha1.ApplicationSetGenerator{
				{Clusters: &v1alpha1.ClusterGenerator{}},
			},
			Template: v1alpha1.ApplicationSetTemplate{
				Spec: v1alpha1.ApplicationSpec{Project: "team-z"},
			},
		},
	}

	mock := &MockArgoClient{
		CreateApplicationSetFn: func(_ context.Context, req *applicationset.ApplicationSetCreateRequest) (*v1alpha1.ApplicationSet, error) {
			assert.Equal(t, "new-appset", req.Applicationset.Name)
			assert.False(t, req.Upsert)
			return created, nil
		},
	}

	spec := `
metadata:
  name: new-appset
spec:
  generators:
    - clusters: {}
  template:
    spec:
      project: team-z
`
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "create_applicationset", map[string]interface{}{
		"spec": spec,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, parseResultText(t, result))
	assert.Contains(t, parseResultText(t, result), "new-appset")
}

func TestHandleCreateApplicationSet_Upsert(t *testing.T) {
	mock := &MockArgoClient{
		CreateApplicationSetFn: func(_ context.Context, req *applicationset.ApplicationSetCreateRequest) (*v1alpha1.ApplicationSet, error) {
			assert.True(t, req.Upsert)
			return &v1alpha1.ApplicationSet{ObjectMeta: metav1.ObjectMeta{Name: "existing"}}, nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	_, err := tm.CallTool(context.Background(), "create_applicationset", map[string]interface{}{
		"spec":   "metadata:\n  name: existing\n",
		"upsert": true,
	})
	require.NoError(t, err)
}

func TestHandleCreateApplicationSet_MissingSpec(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "create_applicationset", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- delete_applicationset ---

func TestHandleDeleteApplicationSet_SafeMode(t *testing.T) {
	mock := &MockArgoClient{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	tm := NewToolManager(mock, logger, true)

	result, err := tm.CallTool(context.Background(), "delete_applicationset", map[string]interface{}{
		"name": "my-appset",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleDeleteApplicationSet_Success(t *testing.T) {
	mock := &MockArgoClient{
		DeleteApplicationSetFn: func(_ context.Context, req *applicationset.ApplicationSetDeleteRequest) error {
			assert.Equal(t, "old-appset", req.Name)
			return nil
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "delete_applicationset", map[string]interface{}{
		"name": "old-appset",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, parseResultText(t, result), "deleted")
}

func TestHandleDeleteApplicationSet_MissingName(t *testing.T) {
	mock := &MockArgoClient{}
	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "delete_applicationset", map[string]interface{}{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleDeleteApplicationSet_ClientError(t *testing.T) {
	mock := &MockArgoClient{
		DeleteApplicationSetFn: func(_ context.Context, _ *applicationset.ApplicationSetDeleteRequest) error {
			return assert.AnError
		},
	}

	tm := newTestToolManagerForAppSet(mock)
	result, err := tm.CallTool(context.Background(), "delete_applicationset", map[string]interface{}{
		"name": "gone",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

// --- helper functions ---

// formatApplicationSetSummary tests

func TestFormatApplicationSetSummary_GeneratorTypes(t *testing.T) {
	as := &v1alpha1.ApplicationSet{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-gen"},
		Spec: v1alpha1.ApplicationSetSpec{
			Generators: []v1alpha1.ApplicationSetGenerator{
				{Git: &v1alpha1.GitGenerator{}},
				{Clusters: &v1alpha1.ClusterGenerator{}},
				{Git: &v1alpha1.GitGenerator{}}, // duplicate should be deduplicated
			},
			Template: v1alpha1.ApplicationSetTemplate{
				Spec: v1alpha1.ApplicationSpec{Project: "test"},
			},
		},
		Status: v1alpha1.ApplicationSetStatus{
			ResourcesCount: 10,
		},
	}

	summary := formatApplicationSetSummary(as)
	assert.Equal(t, "multi-gen", summary.Name)
	assert.Equal(t, int64(10), summary.ApplicationCount)
	assert.Len(t, summary.GeneratorTypes, 2) // Git and Clusters, deduplicated
	assert.Contains(t, summary.GeneratorTypes, "Git")
	assert.Contains(t, summary.GeneratorTypes, "Clusters")
	assert.False(t, summary.HasErrors)
}

func TestFormatApplicationSetSummary_WithErrors(t *testing.T) {
	as := &v1alpha1.ApplicationSet{
		ObjectMeta: metav1.ObjectMeta{Name: "errored"},
		Spec: v1alpha1.ApplicationSetSpec{
			Generators: []v1alpha1.ApplicationSetGenerator{},
			Template:   v1alpha1.ApplicationSetTemplate{Spec: v1alpha1.ApplicationSpec{Project: "x"}},
		},
		Status: v1alpha1.ApplicationSetStatus{
			Conditions: []v1alpha1.ApplicationSetCondition{
				{
					Type:    v1alpha1.ApplicationSetConditionErrorOccurred,
					Status:  v1alpha1.ApplicationSetConditionStatusTrue,
					Message: "render error",
					Reason:  "RenderTemplateParamsError",
				},
			},
		},
	}

	summary := formatApplicationSetSummary(as)
	assert.True(t, summary.HasErrors)
	require.Len(t, summary.Conditions, 1)
	assert.Equal(t, "render error", summary.Conditions[0].Message)
}

func TestFormatApplicationSetDetail_Strategy(t *testing.T) {
	as := &v1alpha1.ApplicationSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rolling"},
		Spec: v1alpha1.ApplicationSetSpec{
			Generators: []v1alpha1.ApplicationSetGenerator{
				{Clusters: &v1alpha1.ClusterGenerator{}},
			},
			Template: v1alpha1.ApplicationSetTemplate{
				Spec: v1alpha1.ApplicationSpec{
					Project: "prod",
					Source: &v1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/org/repo",
						Path:           "charts/app",
						TargetRevision: "v1.2.3",
					},
					Destination: v1alpha1.ApplicationDestination{
						Server:    "https://k8s.example.com",
						Namespace: "production",
					},
				},
			},
			Strategy: &v1alpha1.ApplicationSetStrategy{
				RollingSync: &v1alpha1.ApplicationSetRolloutStrategy{},
			},
		},
	}

	detail := formatApplicationSetDetail(as)
	assert.Equal(t, "RollingSync", detail.Strategy)
	assert.Equal(t, "https://github.com/org/repo", detail.Template.RepoURL)
	assert.Equal(t, "charts/app", detail.Template.Path)
	assert.Equal(t, "v1.2.3", detail.Template.TargetRevision)
	assert.Equal(t, "https://k8s.example.com", detail.Template.DestServer)
}

// TestToolManager_ApplicationSetToolsInList verifies all 5 tools appear in the tool list.
func TestToolManager_ApplicationSetToolsInList(t *testing.T) {
	mock := &MockArgoClient{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	tm := NewToolManager(mock, logger, false)

	names := tm.GetToolNames()
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	expectedTools := []string{
		"list_applicationsets",
		"get_applicationset",
		"preview_applicationset",
		"create_applicationset",
		"delete_applicationset",
	}
	for _, want := range expectedTools {
		assert.True(t, nameSet[want], "tool %q should be registered", want)
	}
}
