package tools

import (
	"context"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

// ArgoClient defines the interface for interacting with the ArgoCD API.
// This interface allows for easy mocking in tests.
type ArgoClient interface {
	// Application methods
	ListApplications(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.ApplicationList, error)
	GetApplication(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.Application, error)
	CreateApplication(ctx context.Context, createReq *application.ApplicationCreateRequest) (*v1alpha1.Application, error)
	UpdateApplication(ctx context.Context, updateReq *application.ApplicationUpdateRequest) (*v1alpha1.Application, error)
	DeleteApplication(ctx context.Context, deleteReq *application.ApplicationDeleteRequest) error
	SyncApplication(ctx context.Context, syncReq *application.ApplicationSyncRequest) (*v1alpha1.Application, error)
	GetApplicationManifests(ctx context.Context, query *application.ApplicationManifestQuery) ([]string, error)
	RollbackApplication(ctx context.Context, rollbackReq *application.ApplicationRollbackRequest) (*v1alpha1.Application, error)
	GetApplicationEvents(ctx context.Context, query *application.ApplicationResourceEventsQuery) (interface{}, error)
	GetApplicationLogs(ctx context.Context, query *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error)
	GetManagedResources(ctx context.Context, appName string) ([]*v1alpha1.ResourceDiff, error)
	ListResourceActions(ctx context.Context, query *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error)
	//lint:ignore SA1019 ResourceActionRunRequest is deprecated but required for the API
	RunResourceAction(ctx context.Context, actionReq *application.ResourceActionRunRequest) error //nolint:staticcheck
	GetApplicationResource(ctx context.Context, query *application.ApplicationResourceRequest) (interface{}, error)
	PatchApplicationResource(ctx context.Context, patchReq *application.ApplicationResourcePatchRequest) (interface{}, error)
	DeleteApplicationResource(ctx context.Context, deleteReq *application.ApplicationResourceDeleteRequest) error

	// Project methods
	ListProjects(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProjectList, error)
	GetProject(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProject, error)
	CreateProject(ctx context.Context, createReq *project.ProjectCreateRequest) (*v1alpha1.AppProject, error)
	UpdateProject(ctx context.Context, updateReq *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error)
	DeleteProject(ctx context.Context, query *project.ProjectQuery) error
	GetProjectEvents(ctx context.Context, query *project.ProjectQuery) (interface{}, error)

	// Repository methods
	ListRepositories(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.RepositoryList, error)
	GetRepository(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.Repository, error)
	CreateRepository(ctx context.Context, createReq *repository.RepoCreateRequest) (*v1alpha1.Repository, error)
	UpdateRepository(ctx context.Context, updateReq *repository.RepoUpdateRequest) (*v1alpha1.Repository, error)
	DeleteRepository(ctx context.Context, query *repository.RepoQuery) error
	ValidateRepositoryAccess(ctx context.Context, query *repository.RepoAccessQuery) error

	// Cluster methods
	ListClusters(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.ClusterList, error)
	GetCluster(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.Cluster, error)
	CreateCluster(ctx context.Context, createReq *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error)
	UpdateCluster(ctx context.Context, updateReq *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error)
	DeleteCluster(ctx context.Context, query *cluster.ClusterQuery) error
}

// Compile-time check that *client.Client satisfies ArgoClient
var _ ArgoClient = (*client.Client)(nil)
