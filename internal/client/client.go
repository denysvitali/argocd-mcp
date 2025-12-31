package client

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// Client wraps the ArgoCD API client with additional functionality
type Client struct {
	client  apiclient.Client
	logger  *logrus.Logger
	server  string
	limiter *rate.Limiter
}

// NewClient creates a new ArgoCD client
func NewClient(logger *logrus.Logger, server, token string, insecure bool, certFile string) (*Client, error) {
	opts := &apiclient.ClientOptions{
		ServerAddr: server,
		AuthToken:  token,
		Insecure:   insecure,
		CertFile:   certFile,
	}

	argoClient, err := apiclient.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create ArgoCD client: %w", err)
	}

	// Rate limiter: 10 requests per second with burst of 20
	limiter := rate.NewLimiter(10, 20)

	return &Client{
		client:  argoClient,
		logger:  logger,
		server:  server,
		limiter: limiter,
	}, nil
}

// WaitForRateLimit waits for the rate limiter to allow the next request
func (c *Client) WaitForRateLimit(ctx context.Context) error {
	return c.limiter.Wait(ctx)
}

// Application client methods

// ListApplications returns a list of applications
func (c *Client) ListApplications(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.ApplicationList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.List(ctx, query)
}

// GetApplication returns a single application
func (c *Client) GetApplication(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.Get(ctx, query)
}

// CreateApplication creates a new application
func (c *Client) CreateApplication(ctx context.Context, createReq *application.ApplicationCreateRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.Create(ctx, createReq)
}

// UpdateApplication updates an existing application
func (c *Client) UpdateApplication(ctx context.Context, updateReq *application.ApplicationUpdateRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.Update(ctx, updateReq)
}

// DeleteApplication deletes an application
func (c *Client) DeleteApplication(ctx context.Context, deleteReq *application.ApplicationDeleteRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	_, err = appClient.Delete(ctx, deleteReq)
	return err
}

// SyncApplication triggers a sync for an application
func (c *Client) SyncApplication(ctx context.Context, syncReq *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.Sync(ctx, syncReq)
}

// GetApplicationManifests returns the manifests for an application
func (c *Client) GetApplicationManifests(ctx context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	resp, err := appClient.GetManifests(ctx, query)
	if err != nil {
		return nil, err
	}
	return resp.Manifests, nil
}

// RollbackApplication performs a rollback for an application
func (c *Client) RollbackApplication(ctx context.Context, rollbackReq *application.ApplicationRollbackRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.Rollback(ctx, rollbackReq)
}

// GetApplicationEvents returns events for an application
func (c *Client) GetApplicationEvents(ctx context.Context, query *application.ApplicationResourceEventsQuery) (interface{}, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	return appClient.ListResourceEvents(ctx, query)
}

// ListResourceActions lists available actions for a resource
func (c *Client) ListResourceActions(ctx context.Context, query *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	resp, err := appClient.ListResourceActions(ctx, query)
	if err != nil {
		return nil, err
	}
	return resp.Actions, nil
}

// RunResourceAction runs an action on a resource
func (c *Client) RunResourceAction(ctx context.Context, actionReq *application.ResourceActionRunRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, appClient, err := c.client.NewApplicationClient()
	if err != nil {
		return fmt.Errorf("failed to create app client: %w", err)
	}
	defer closer.Close()

	_, err = appClient.RunResourceAction(ctx, actionReq)
	return err
}

// Project client methods

// ListProjects returns a list of projects
func (c *Client) ListProjects(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProjectList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	return projectClient.List(ctx, query)
}

// GetProject returns a single project
func (c *Client) GetProject(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	return projectClient.Get(ctx, query)
}

// CreateProject creates a new project
func (c *Client) CreateProject(ctx context.Context, createReq *project.ProjectCreateRequest) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	return projectClient.Create(ctx, createReq)
}

// UpdateProject updates an existing project
func (c *Client) UpdateProject(ctx context.Context, updateReq *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	return projectClient.Update(ctx, updateReq)
}

// DeleteProject deletes a project
func (c *Client) DeleteProject(ctx context.Context, query *project.ProjectQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	_, err = projectClient.Delete(ctx, query)
	return err
}

// GetProjectEvents returns events for a project
func (c *Client) GetProjectEvents(ctx context.Context, query *project.ProjectQuery) (interface{}, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, projectClient, err := c.client.NewProjectClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create project client: %w", err)
	}
	defer closer.Close()

	return projectClient.ListEvents(ctx, query)
}

// Repository client methods

// ListRepositories returns a list of repositories
func (c *Client) ListRepositories(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.RepositoryList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	return repoClient.List(ctx, query)
}

// GetRepository returns a single repository
func (c *Client) GetRepository(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	return repoClient.Get(ctx, query)
}

// CreateRepository creates a new repository
func (c *Client) CreateRepository(ctx context.Context, createReq *repository.RepoCreateRequest) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	return repoClient.Create(ctx, createReq)
}

// UpdateRepository updates an existing repository
func (c *Client) UpdateRepository(ctx context.Context, updateReq *repository.RepoUpdateRequest) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	return repoClient.Update(ctx, updateReq)
}

// DeleteRepository deletes a repository
func (c *Client) DeleteRepository(ctx context.Context, query *repository.RepoQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	_, err = repoClient.Delete(ctx, query)
	return err
}

// ValidateRepositoryAccess validates access to a repository
func (c *Client) ValidateRepositoryAccess(ctx context.Context, query *repository.RepoAccessQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, repoClient, err := c.client.NewRepoClient()
	if err != nil {
		return fmt.Errorf("failed to create repo client: %w", err)
	}
	defer closer.Close()

	_, err = repoClient.ValidateAccess(ctx, query)
	return err
}

// Cluster client methods

// ListClusters returns a list of clusters
func (c *Client) ListClusters(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.ClusterList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, clusterClient, err := c.client.NewClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}
	defer closer.Close()

	return clusterClient.List(ctx, query)
}

// GetCluster returns a single cluster
func (c *Client) GetCluster(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, clusterClient, err := c.client.NewClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}
	defer closer.Close()

	return clusterClient.Get(ctx, query)
}

// CreateCluster creates a new cluster
func (c *Client) CreateCluster(ctx context.Context, createReq *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, clusterClient, err := c.client.NewClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}
	defer closer.Close()

	return clusterClient.Create(ctx, createReq)
}

// UpdateCluster updates an existing cluster
func (c *Client) UpdateCluster(ctx context.Context, updateReq *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, clusterClient, err := c.client.NewClusterClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}
	defer closer.Close()

	return clusterClient.Update(ctx, updateReq)
}

// DeleteCluster deletes a cluster
func (c *Client) DeleteCluster(ctx context.Context, query *cluster.ClusterQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, clusterClient, err := c.client.NewClusterClient()
	if err != nil {
		return fmt.Errorf("failed to create cluster client: %w", err)
	}
	defer closer.Close()

	_, err = clusterClient.Delete(ctx, query)
	return err
}

// Account client methods

// GetAccount returns account information
func (c *Client) GetAccount(ctx context.Context, name string) (*account.Account, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, accountClient, err := c.client.NewAccountClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create account client: %w", err)
	}
	defer closer.Close()

	return accountClient.GetAccount(ctx, &account.GetAccountRequest{Name: name})
}

// CanI checks if the current user can perform an action
func (c *Client) CanI(ctx context.Context, action, scope string) (string, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return "", fmt.Errorf("rate limit exceeded: %w", err)
	}

	closer, accountClient, err := c.client.NewAccountClient()
	if err != nil {
		return "", fmt.Errorf("failed to create account client: %w", err)
	}
	defer closer.Close()

	resp, err := accountClient.CanI(ctx, &account.CanIRequest{Action: action, Resource: scope})
	if err != nil {
		return "", err
	}
	return resp.Value, nil
}

// Server returns the configured server address
func (c *Client) Server() string {
	return c.server
}

// WithTimeout returns a context with timeout
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}
