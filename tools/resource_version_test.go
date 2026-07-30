package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func treeWith(nodes ...v1alpha1.ResourceNode) *v1alpha1.ApplicationTree {
	return &v1alpha1.ApplicationTree{Nodes: nodes}
}

func node(group, version, kind, namespace, name string) v1alpha1.ResourceNode {
	return v1alpha1.ResourceNode{
		ResourceRef: v1alpha1.ResourceRef{
			Group:     group,
			Version:   version,
			Kind:      kind,
			Namespace: namespace,
			Name:      name,
		},
	}
}

// TestResolveResourceVersion covers the case that made CRDs unaddressable:
// inferResourceVersion guesses "v1" for every custom group, so a resource
// served at v1alpha1 was rejected by the API with "could not find the
// requested resource".
func TestResolveResourceVersion(t *testing.T) {
	tree := treeWith(
		node("actions.github.com", "v1alpha1", "AutoscalingRunnerSet", "arc", "wanderlog-alt"),
		node("apps", "v1", "Deployment", "arc", "controller"),
		node("", "v1", "Pod", "arc", "controller-abc"),
	)

	tests := []struct {
		name                                 string
		group, kind, namespace, resourceName string
		want                                 string
	}{
		{"CRD served at v1alpha1", "actions.github.com", "AutoscalingRunnerSet", "arc", "wanderlog-alt", "v1alpha1"},
		{"built-in group", "apps", "Deployment", "arc", "controller", "v1"},
		{"core group", "", "Pod", "arc", "controller-abc", "v1"},
		{"namespace need not be given", "actions.github.com", "AutoscalingRunnerSet", "", "wanderlog-alt", "v1alpha1"},
		{"wrong namespace falls back to the guess", "actions.github.com", "AutoscalingRunnerSet", "other", "wanderlog-alt", "v1"},
		{"resource absent from the tree falls back", "actions.github.com", "AutoscalingRunnerSet", "arc", "ghost", "v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := NewToolManager(&MockArgoClient{
				GetResourceTreeFn: func(context.Context, string) (*v1alpha1.ApplicationTree, error) {
					return tree, nil
				},
			}, newTestLogger(), true, false)

			got := tm.resolveResourceVersion(context.Background(), "app", tt.group, tt.kind, tt.namespace, tt.resourceName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveResourceVersionFallsBackWhenTreeUnavailable(t *testing.T) {
	tm := NewToolManager(&MockArgoClient{
		GetResourceTreeFn: func(context.Context, string) (*v1alpha1.ApplicationTree, error) {
			return nil, errors.New("boom")
		},
	}, newTestLogger(), true, false)

	got := tm.resolveResourceVersion(context.Background(), "app", "apps", "Deployment", "ns", "web")
	assert.Equal(t, "v1", got, "must fall back to the group-based guess, not fail")
}

func TestResolveResourceVersionSkipsIncompleteInput(t *testing.T) {
	called := false
	tm := NewToolManager(&MockArgoClient{
		GetResourceTreeFn: func(context.Context, string) (*v1alpha1.ApplicationTree, error) {
			called = true
			return nil, nil
		},
	}, newTestLogger(), true, false)

	// Without a kind or resource name there is nothing to match on, so the
	// tree lookup is not worth a round trip.
	assert.Equal(t, "v1", tm.resolveResourceVersion(context.Background(), "app", "apps", "", "ns", ""))
	assert.False(t, called, "should not query the resource tree without a target")
}
