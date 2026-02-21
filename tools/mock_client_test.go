package tools

import (
	"context"
	"fmt"

	"github.com/argocd-mcp/argocd-mcp/internal/client"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

// MockArgoClient is a mock implementation of ArgoClient interface for testing.
type MockArgoClient struct {
	// Application methods
	ListApplicationsFn        func(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.ApplicationList, error)
	GetApplicationFn          func(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.Application, error)
	CreateApplicationFn       func(ctx context.Context, createReq *application.ApplicationCreateRequest) (*v1alpha1.Application, error)
	UpdateApplicationFn       func(ctx context.Context, updateReq *application.ApplicationUpdateRequest) (*v1alpha1.Application, error)
	DeleteApplicationFn       func(ctx context.Context, deleteReq *application.ApplicationDeleteRequest) error
	SyncApplicationFn         func(ctx context.Context, syncReq *application.ApplicationSyncRequest) (*v1alpha1.Application, error)
	GetApplicationManifestsFn func(ctx context.Context, query *application.ApplicationManifestQuery) ([]string, error)
	RollbackApplicationFn     func(ctx context.Context, rollbackReq *application.ApplicationRollbackRequest) (*v1alpha1.Application, error)
	GetApplicationEventsFn    func(ctx context.Context, query *application.ApplicationResourceEventsQuery) (interface{}, error)
	GetApplicationLogsFn      func(ctx context.Context, query *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error)
	GetManagedResourcesFn     func(ctx context.Context, appName string) ([]*v1alpha1.ResourceDiff, error)
	ListResourceActionsFn     func(ctx context.Context, query *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error)
	//lint:ignore SA1019 ResourceActionRunRequest is deprecated but required for the API
	RunResourceActionFn func(ctx context.Context, actionReq *application.ResourceActionRunRequest) error
	GetApplicationResourceFn  func(ctx context.Context, query *application.ApplicationResourceRequest) (interface{}, error)
	PatchApplicationResourceFn func(ctx context.Context, patchReq *application.ApplicationResourcePatchRequest) (interface{}, error)
	DeleteApplicationResourceFn func(ctx context.Context, deleteReq *application.ApplicationResourceDeleteRequest) error

	// Project methods
	ListProjectsFn   func(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProjectList, error)
	GetProjectFn     func(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProject, error)
	CreateProjectFn  func(ctx context.Context, createReq *project.ProjectCreateRequest) (*v1alpha1.AppProject, error)
	UpdateProjectFn  func(ctx context.Context, updateReq *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error)
	DeleteProjectFn  func(ctx context.Context, query *project.ProjectQuery) error
	GetProjectEventsFn func(ctx context.Context, query *project.ProjectQuery) (interface{}, error)

	// Repository methods
	ListRepositoriesFn func(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.RepositoryList, error)
	GetRepositoryFn    func(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.Repository, error)
	CreateRepositoryFn func(ctx context.Context, createReq *repository.RepoCreateRequest) (*v1alpha1.Repository, error)
	UpdateRepositoryFn func(ctx context.Context, updateReq *repository.RepoUpdateRequest) (*v1alpha1.Repository, error)
	DeleteRepositoryFn func(ctx context.Context, query *repository.RepoQuery) error
	ValidateRepositoryAccessFn func(ctx context.Context, query *repository.RepoAccessQuery) error

	// Cluster methods
	ListClustersFn   func(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.ClusterList, error)
	GetClusterFn     func(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.Cluster, error)
	CreateClusterFn  func(ctx context.Context, createReq *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error)
	UpdateClusterFn  func(ctx context.Context, updateReq *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error)
	DeleteClusterFn  func(ctx context.Context, query *cluster.ClusterQuery) error

	// Call tracking
	ListApplicationsCalls        []*MockCall
	GetApplicationCalls          []*MockCall
	CreateApplicationCalls       []*MockCall
	UpdateApplicationCalls       []*MockCall
	DeleteApplicationCalls       []*MockCall
	SyncApplicationCalls         []*MockCall
	GetApplicationManifestsCalls []*MockCall
	RollbackApplicationCalls     []*MockCall
	GetApplicationEventsCalls    []*MockCall
	GetApplicationLogsCalls      []*MockCall
	GetManagedResourcesCalls     []*MockCall
	ListResourceActionsCalls     []*MockCall
	RunResourceActionCalls       []*MockCall
	GetApplicationResourceCalls  []*MockCall
	PatchApplicationResourceCalls []*MockCall
	DeleteApplicationResourceCalls []*MockCall

	ListProjectsCalls   []*MockCall
	GetProjectCalls     []*MockCall
	CreateProjectCalls  []*MockCall
	UpdateProjectCalls  []*MockCall
	DeleteProjectCalls  []*MockCall
	GetProjectEventsCalls []*MockCall

	ListRepositoriesCalls []*MockCall
	GetRepositoryCalls    []*MockCall
	CreateRepositoryCalls []*MockCall
	UpdateRepositoryCalls []*MockCall
	DeleteRepositoryCalls []*MockCall
	ValidateRepositoryAccessCalls []*MockCall

	ListClustersCalls   []*MockCall
	GetClusterCalls     []*MockCall
	CreateClusterCalls  []*MockCall
	UpdateClusterCalls  []*MockCall
	DeleteClusterCalls  []*MockCall
}

// MockCall represents a method call with its arguments.
type MockCall struct {
	Args interface{}
	Err  error
	Ret  interface{}
}

// Application methods

func (m *MockArgoClient) ListApplications(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
	m.ListApplicationsCalls = append(m.ListApplicationsCalls, &MockCall{Args: query})
	if m.ListApplicationsFn != nil {
		return m.ListApplicationsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListApplications not mocked")
}

func (m *MockArgoClient) GetApplication(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.Application, error) {
	m.GetApplicationCalls = append(m.GetApplicationCalls, &MockCall{Args: query})
	if m.GetApplicationFn != nil {
		return m.GetApplicationFn(ctx, query)
	}
	return nil, fmt.Errorf("GetApplication not mocked")
}

func (m *MockArgoClient) CreateApplication(ctx context.Context, createReq *application.ApplicationCreateRequest) (*v1alpha1.Application, error) {
	m.CreateApplicationCalls = append(m.CreateApplicationCalls, &MockCall{Args: createReq})
	if m.CreateApplicationFn != nil {
		return m.CreateApplicationFn(ctx, createReq)
	}
	return nil, fmt.Errorf("CreateApplication not mocked")
}

func (m *MockArgoClient) UpdateApplication(ctx context.Context, updateReq *application.ApplicationUpdateRequest) (*v1alpha1.Application, error) {
	m.UpdateApplicationCalls = append(m.UpdateApplicationCalls, &MockCall{Args: updateReq})
	if m.UpdateApplicationFn != nil {
		return m.UpdateApplicationFn(ctx, updateReq)
	}
	return nil, fmt.Errorf("UpdateApplication not mocked")
}

func (m *MockArgoClient) DeleteApplication(ctx context.Context, deleteReq *application.ApplicationDeleteRequest) error {
	m.DeleteApplicationCalls = append(m.DeleteApplicationCalls, &MockCall{Args: deleteReq})
	if m.DeleteApplicationFn != nil {
		return m.DeleteApplicationFn(ctx, deleteReq)
	}
	return fmt.Errorf("DeleteApplication not mocked")
}

func (m *MockArgoClient) SyncApplication(ctx context.Context, syncReq *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
	m.SyncApplicationCalls = append(m.SyncApplicationCalls, &MockCall{Args: syncReq})
	if m.SyncApplicationFn != nil {
		return m.SyncApplicationFn(ctx, syncReq)
	}
	return nil, fmt.Errorf("SyncApplication not mocked")
}

func (m *MockArgoClient) GetApplicationManifests(ctx context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
	m.GetApplicationManifestsCalls = append(m.GetApplicationManifestsCalls, &MockCall{Args: query})
	if m.GetApplicationManifestsFn != nil {
		return m.GetApplicationManifestsFn(ctx, query)
	}
	return nil, fmt.Errorf("GetApplicationManifests not mocked")
}

func (m *MockArgoClient) RollbackApplication(ctx context.Context, rollbackReq *application.ApplicationRollbackRequest) (*v1alpha1.Application, error) {
	m.RollbackApplicationCalls = append(m.RollbackApplicationCalls, &MockCall{Args: rollbackReq})
	if m.RollbackApplicationFn != nil {
		return m.RollbackApplicationFn(ctx, rollbackReq)
	}
	return nil, fmt.Errorf("RollbackApplication not mocked")
}

func (m *MockArgoClient) GetApplicationEvents(ctx context.Context, query *application.ApplicationResourceEventsQuery) (interface{}, error) {
	m.GetApplicationEventsCalls = append(m.GetApplicationEventsCalls, &MockCall{Args: query})
	if m.GetApplicationEventsFn != nil {
		return m.GetApplicationEventsFn(ctx, query)
	}
	return nil, fmt.Errorf("GetApplicationEvents not mocked")
}

func (m *MockArgoClient) GetApplicationLogs(ctx context.Context, query *application.ApplicationPodLogsQuery) ([]client.ApplicationLogEntry, error) {
	m.GetApplicationLogsCalls = append(m.GetApplicationLogsCalls, &MockCall{Args: query})
	if m.GetApplicationLogsFn != nil {
		return m.GetApplicationLogsFn(ctx, query)
	}
	return nil, fmt.Errorf("GetApplicationLogs not mocked")
}

func (m *MockArgoClient) GetManagedResources(ctx context.Context, appName string) ([]*v1alpha1.ResourceDiff, error) {
	m.GetManagedResourcesCalls = append(m.GetManagedResourcesCalls, &MockCall{Args: appName})
	if m.GetManagedResourcesFn != nil {
		return m.GetManagedResourcesFn(ctx, appName)
	}
	return nil, fmt.Errorf("GetManagedResources not mocked")
}

func (m *MockArgoClient) ListResourceActions(ctx context.Context, query *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error) {
	m.ListResourceActionsCalls = append(m.ListResourceActionsCalls, &MockCall{Args: query})
	if m.ListResourceActionsFn != nil {
		return m.ListResourceActionsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListResourceActions not mocked")
}

//lint:ignore SA1019 ResourceActionRunRequest is deprecated but required for the API
func (m *MockArgoClient) RunResourceAction(ctx context.Context, actionReq *application.ResourceActionRunRequest) error { //nolint:staticcheck
	m.RunResourceActionCalls = append(m.RunResourceActionCalls, &MockCall{Args: actionReq})
	if m.RunResourceActionFn != nil {
		return m.RunResourceActionFn(ctx, actionReq)
	}
	return fmt.Errorf("RunResourceAction not mocked")
}

func (m *MockArgoClient) GetApplicationResource(ctx context.Context, query *application.ApplicationResourceRequest) (interface{}, error) {
	m.GetApplicationResourceCalls = append(m.GetApplicationResourceCalls, &MockCall{Args: query})
	if m.GetApplicationResourceFn != nil {
		return m.GetApplicationResourceFn(ctx, query)
	}
	return nil, fmt.Errorf("GetApplicationResource not mocked")
}

func (m *MockArgoClient) PatchApplicationResource(ctx context.Context, patchReq *application.ApplicationResourcePatchRequest) (interface{}, error) {
	m.PatchApplicationResourceCalls = append(m.PatchApplicationResourceCalls, &MockCall{Args: patchReq})
	if m.PatchApplicationResourceFn != nil {
		return m.PatchApplicationResourceFn(ctx, patchReq)
	}
	return nil, fmt.Errorf("PatchApplicationResource not mocked")
}

func (m *MockArgoClient) DeleteApplicationResource(ctx context.Context, deleteReq *application.ApplicationResourceDeleteRequest) error {
	m.DeleteApplicationResourceCalls = append(m.DeleteApplicationResourceCalls, &MockCall{Args: deleteReq})
	if m.DeleteApplicationResourceFn != nil {
		return m.DeleteApplicationResourceFn(ctx, deleteReq)
	}
	return fmt.Errorf("DeleteApplicationResource not mocked")
}

// Project methods

func (m *MockArgoClient) ListProjects(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProjectList, error) {
	m.ListProjectsCalls = append(m.ListProjectsCalls, &MockCall{Args: query})
	if m.ListProjectsFn != nil {
		return m.ListProjectsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListProjects not mocked")
}

func (m *MockArgoClient) GetProject(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProject, error) {
	m.GetProjectCalls = append(m.GetProjectCalls, &MockCall{Args: query})
	if m.GetProjectFn != nil {
		return m.GetProjectFn(ctx, query)
	}
	return nil, fmt.Errorf("GetProject not mocked")
}

func (m *MockArgoClient) CreateProject(ctx context.Context, createReq *project.ProjectCreateRequest) (*v1alpha1.AppProject, error) {
	m.CreateProjectCalls = append(m.CreateProjectCalls, &MockCall{Args: createReq})
	if m.CreateProjectFn != nil {
		return m.CreateProjectFn(ctx, createReq)
	}
	return nil, fmt.Errorf("CreateProject not mocked")
}

func (m *MockArgoClient) UpdateProject(ctx context.Context, updateReq *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error) {
	m.UpdateProjectCalls = append(m.UpdateProjectCalls, &MockCall{Args: updateReq})
	if m.UpdateProjectFn != nil {
		return m.UpdateProjectFn(ctx, updateReq)
	}
	return nil, fmt.Errorf("UpdateProject not mocked")
}

func (m *MockArgoClient) DeleteProject(ctx context.Context, query *project.ProjectQuery) error {
	m.DeleteProjectCalls = append(m.DeleteProjectCalls, &MockCall{Args: query})
	if m.DeleteProjectFn != nil {
		return m.DeleteProjectFn(ctx, query)
	}
	return fmt.Errorf("DeleteProject not mocked")
}

func (m *MockArgoClient) GetProjectEvents(ctx context.Context, query *project.ProjectQuery) (interface{}, error) {
	m.GetProjectEventsCalls = append(m.GetProjectEventsCalls, &MockCall{Args: query})
	if m.GetProjectEventsFn != nil {
		return m.GetProjectEventsFn(ctx, query)
	}
	return nil, fmt.Errorf("GetProjectEvents not mocked")
}

// Repository methods

func (m *MockArgoClient) ListRepositories(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.RepositoryList, error) {
	m.ListRepositoriesCalls = append(m.ListRepositoriesCalls, &MockCall{Args: query})
	if m.ListRepositoriesFn != nil {
		return m.ListRepositoriesFn(ctx, query)
	}
	return nil, fmt.Errorf("ListRepositories not mocked")
}

func (m *MockArgoClient) GetRepository(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.Repository, error) {
	m.GetRepositoryCalls = append(m.GetRepositoryCalls, &MockCall{Args: query})
	if m.GetRepositoryFn != nil {
		return m.GetRepositoryFn(ctx, query)
	}
	return nil, fmt.Errorf("GetRepository not mocked")
}

func (m *MockArgoClient) CreateRepository(ctx context.Context, createReq *repository.RepoCreateRequest) (*v1alpha1.Repository, error) {
	m.CreateRepositoryCalls = append(m.CreateRepositoryCalls, &MockCall{Args: createReq})
	if m.CreateRepositoryFn != nil {
		return m.CreateRepositoryFn(ctx, createReq)
	}
	return nil, fmt.Errorf("CreateRepository not mocked")
}

func (m *MockArgoClient) UpdateRepository(ctx context.Context, updateReq *repository.RepoUpdateRequest) (*v1alpha1.Repository, error) {
	m.UpdateRepositoryCalls = append(m.UpdateRepositoryCalls, &MockCall{Args: updateReq})
	if m.UpdateRepositoryFn != nil {
		return m.UpdateRepositoryFn(ctx, updateReq)
	}
	return nil, fmt.Errorf("UpdateRepository not mocked")
}

func (m *MockArgoClient) DeleteRepository(ctx context.Context, query *repository.RepoQuery) error {
	m.DeleteRepositoryCalls = append(m.DeleteRepositoryCalls, &MockCall{Args: query})
	if m.DeleteRepositoryFn != nil {
		return m.DeleteRepositoryFn(ctx, query)
	}
	return fmt.Errorf("DeleteRepository not mocked")
}

func (m *MockArgoClient) ValidateRepositoryAccess(ctx context.Context, query *repository.RepoAccessQuery) error {
	m.ValidateRepositoryAccessCalls = append(m.ValidateRepositoryAccessCalls, &MockCall{Args: query})
	if m.ValidateRepositoryAccessFn != nil {
		return m.ValidateRepositoryAccessFn(ctx, query)
	}
	return fmt.Errorf("ValidateRepositoryAccess not mocked")
}

// Cluster methods

func (m *MockArgoClient) ListClusters(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.ClusterList, error) {
	m.ListClustersCalls = append(m.ListClustersCalls, &MockCall{Args: query})
	if m.ListClustersFn != nil {
		return m.ListClustersFn(ctx, query)
	}
	return nil, fmt.Errorf("ListClusters not mocked")
}

func (m *MockArgoClient) GetCluster(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.Cluster, error) {
	m.GetClusterCalls = append(m.GetClusterCalls, &MockCall{Args: query})
	if m.GetClusterFn != nil {
		return m.GetClusterFn(ctx, query)
	}
	return nil, fmt.Errorf("GetCluster not mocked")
}

func (m *MockArgoClient) CreateCluster(ctx context.Context, createReq *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error) {
	m.CreateClusterCalls = append(m.CreateClusterCalls, &MockCall{Args: createReq})
	if m.CreateClusterFn != nil {
		return m.CreateClusterFn(ctx, createReq)
	}
	return nil, fmt.Errorf("CreateCluster not mocked")
}

func (m *MockArgoClient) UpdateCluster(ctx context.Context, updateReq *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error) {
	m.UpdateClusterCalls = append(m.UpdateClusterCalls, &MockCall{Args: updateReq})
	if m.UpdateClusterFn != nil {
		return m.UpdateClusterFn(ctx, updateReq)
	}
	return nil, fmt.Errorf("UpdateCluster not mocked")
}

func (m *MockArgoClient) DeleteCluster(ctx context.Context, query *cluster.ClusterQuery) error {
	m.DeleteClusterCalls = append(m.DeleteClusterCalls, &MockCall{Args: query})
	if m.DeleteClusterFn != nil {
		return m.DeleteClusterFn(ctx, query)
	}
	return fmt.Errorf("DeleteCluster not mocked")
}
