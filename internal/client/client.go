package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/account"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/applicationset"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/cluster"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/session"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
)

// MaxLogEntries is the maximum number of log entries to return
const MaxLogEntries = 500

// ApplicationLogEntry represents a single log entry from an application pod
type ApplicationLogEntry struct {
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
	PodName   string `json:"pod_name,omitempty"`
}

// Rate limiting constants
const (
	rateLimitRequests = 10
	rateLimitBurst    = 20
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// Client wraps the ArgoCD API client with additional functionality
type Client struct {
	mu         sync.RWMutex
	client     apiclient.Client
	logger     *logrus.Logger
	server     string
	limiter    *rate.Limiter
	refreshFn  func(context.Context) (string, error)
	clientOpts apiclient.ClientOptions
}

// NewClient creates a new ArgoCD client
func NewClient(logger *logrus.Logger, server, token string, insecure, plaintext bool, certFile string, grpcWeb bool, grpcWebRootPath string) (*Client, error) {
	logger.Debugf("Creating ArgoCD client for server: %s", server)
	logger.Debugf("Client options - Insecure: %v, PlainText: %v, GRPCWeb: %v, GRPCWebRootPath: %s", insecure, plaintext, grpcWeb, grpcWebRootPath)

	opts := &apiclient.ClientOptions{
		ServerAddr:      server,
		AuthToken:       token,
		Insecure:        insecure,
		PlainText:       plaintext,
		CertFile:        certFile,
		GRPCWeb:         grpcWeb,
		GRPCWebRootPath: grpcWebRootPath,
	}

	logger.Debug("Initializing ArgoCD API client...")

	argoClient, err := apiclient.NewClient(opts)
	if err != nil {
		logger.Debugf("Failed to create ArgoCD client: %v", err)
		return nil, fmt.Errorf("failed to create ArgoCD client: %w", err)
	}

	logger.Debug("ArgoCD client created successfully")

	// Rate limiter: 10 requests per second with burst of 20
	limiter := rate.NewLimiter(rateLimitRequests, rateLimitBurst)

	return &Client{
		client:  argoClient,
		logger:  logger,
		server:  server,
		limiter: limiter,
	}, nil
}

// NewClientWithRefresh creates a new ArgoCD client with an optional token refresh function.
// When refreshFn is non-nil, any Unauthenticated error will trigger a token refresh and a
// single retry of the failed call.
func NewClientWithRefresh(logger *logrus.Logger, server, token string, insecure, plaintext bool, certFile string, grpcWeb bool, grpcWebRootPath string, refreshFn func(context.Context) (string, error)) (*Client, error) {
	c, err := NewClient(logger, server, token, insecure, plaintext, certFile, grpcWeb, grpcWebRootPath)
	if err != nil {
		return nil, err
	}
	c.refreshFn = refreshFn
	// Store opts without token; token is injected fresh on each refresh.
	c.clientOpts = apiclient.ClientOptions{
		ServerAddr:      server,
		Insecure:        insecure,
		PlainText:       plaintext,
		CertFile:        certFile,
		GRPCWeb:         grpcWeb,
		GRPCWebRootPath: grpcWebRootPath,
	}
	return c, nil
}

// isUnauthenticated returns true when err signals an expired/invalid session.
func isUnauthenticated(err error) bool {
	if err == nil {
		return false
	}
	if s, ok := grpcstatus.FromError(err); ok && s.Code() == codes.Unauthenticated {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid session") || strings.Contains(msg, "Unauthenticated")
}

// refreshAndRecreate fetches a new token and rebuilds c.client under the write lock.
func (c *Client) refreshAndRecreate(ctx context.Context) error {
	newToken, err := c.refreshFn(ctx)
	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}

	opts := c.clientOpts
	opts.AuthToken = newToken

	newArgoCDClient, err := apiclient.NewClient(&opts)
	if err != nil {
		return fmt.Errorf("failed to recreate ArgoCD client after refresh: %w", err)
	}

	c.mu.Lock()
	c.client = newArgoCDClient
	c.mu.Unlock()

	c.logger.Debug("ArgoCD client refreshed with new token")
	return nil
}

// do executes fn under a read lock. If fn returns an Unauthenticated error and a
// refreshFn is configured, it refreshes the token then retries fn exactly once.
func (c *Client) do(ctx context.Context, fn func() error) error {
	c.mu.RLock()
	err := fn()
	c.mu.RUnlock()

	if err == nil || !isUnauthenticated(err) || c.refreshFn == nil {
		return err
	}

	c.logger.Debug("Unauthenticated error detected, refreshing token...")
	if refreshErr := c.refreshAndRecreate(ctx); refreshErr != nil {
		return refreshErr
	}

	// Single retry under read lock.
	c.mu.RLock()
	err = fn()
	c.mu.RUnlock()
	return err
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
	var result *v1alpha1.ApplicationList
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.List(ctx, query)
		return err
	})
	return result, err
}

// GetApplication returns a single application
func (c *Client) GetApplication(ctx context.Context, query *application.ApplicationQuery) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.Get(ctx, query)
		return err
	})
	return result, err
}

// CreateApplication creates a new application
func (c *Client) CreateApplication(ctx context.Context, createReq *application.ApplicationCreateRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.Create(ctx, createReq)
		return err
	})
	return result, err
}

// UpdateApplication updates an existing application
func (c *Client) UpdateApplication(ctx context.Context, updateReq *application.ApplicationUpdateRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.Update(ctx, updateReq)
		return err
	})
	return result, err
}

// DeleteApplication deletes an application
func (c *Client) DeleteApplication(ctx context.Context, deleteReq *application.ApplicationDeleteRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		_, err = appClient.Delete(ctx, deleteReq)
		return err
	})
}

// SyncApplication triggers a sync for an application
func (c *Client) SyncApplication(ctx context.Context, syncReq *application.ApplicationSyncRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.Sync(ctx, syncReq)
		return err
	})
	return result, err
}

// GetApplicationManifests returns the manifests for an application
func (c *Client) GetApplicationManifests(ctx context.Context, query *application.ApplicationManifestQuery) ([]string, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result []string
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		resp, err := appClient.GetManifests(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to get application manifests: %w", err)
		}
		result = resp.Manifests
		return nil
	})
	return result, err
}

// RollbackApplication performs a rollback for an application
func (c *Client) RollbackApplication(ctx context.Context, rollbackReq *application.ApplicationRollbackRequest) (*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.Rollback(ctx, rollbackReq)
		return err
	})
	return result, err
}

// GetApplicationEvents returns events for an application
func (c *Client) GetApplicationEvents(ctx context.Context, query *application.ApplicationResourceEventsQuery) (*corev1.EventList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *corev1.EventList
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.ListResourceEvents(ctx, query)
		return err
	})
	return result, err
}

// GetApplicationLogs retrieves logs from a pod or resource in an application
func (c *Client) GetApplicationLogs(ctx context.Context, query *application.ApplicationPodLogsQuery) ([]ApplicationLogEntry, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var entries []ApplicationLogEntry
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		stream, err := appClient.PodLogs(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to get pod logs: %w", err)
		}

		entries = nil
		for {
			entry, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error receiving logs: %w", err)
			}
			entries = append(entries, ApplicationLogEntry{
				Content:   entry.GetContent(),
				Timestamp: entry.GetTimeStampStr(),
				PodName:   entry.GetPodName(),
			})
			if len(entries) >= MaxLogEntries {
				break
			}
		}
		return nil
	})
	return entries, err
}

// GetManagedResources returns the managed resources for an application with diff information
func (c *Client) GetManagedResources(ctx context.Context, appName string) ([]*v1alpha1.ResourceDiff, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result []*v1alpha1.ResourceDiff
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		appNamePtr := &appName
		query := &application.ResourcesQuery{
			ApplicationName: appNamePtr,
		}
		resp, err := appClient.ManagedResources(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to get managed resources: %w", err)
		}
		result = resp.Items
		return nil
	})
	return result, err
}

// GetResourceTree returns the resource tree for an application
func (c *Client) GetResourceTree(ctx context.Context, appName string) (*v1alpha1.ApplicationTree, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ApplicationTree
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		query := &application.ResourcesQuery{
			ApplicationName: &appName,
		}
		tree, err := appClient.ResourceTree(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to get resource tree: %w", err)
		}
		result = tree
		return nil
	})
	return result, err
}

// ListResourceActions lists available actions for a resource
func (c *Client) ListResourceActions(ctx context.Context, query *application.ApplicationResourceRequest) ([]*v1alpha1.ResourceAction, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result []*v1alpha1.ResourceAction
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		resp, err := appClient.ListResourceActions(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list resource actions: %w", err)
		}
		result = resp.Actions
		return nil
	})
	return result, err
}

// RunResourceAction runs an action on a resource
func (c *Client) RunResourceAction(ctx context.Context, actionReq *application.ResourceActionRunRequestV2) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = appClient.RunResourceActionV2(ctx, actionReq)
		if err != nil {
			return fmt.Errorf("failed to run resource action: %w", err)
		}
		return nil
	})
}

// GetApplicationResource returns a single application resource
func (c *Client) GetApplicationResource(ctx context.Context, query *application.ApplicationResourceRequest) (*application.ApplicationResourceResponse, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *application.ApplicationResourceResponse
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.GetResource(ctx, query)
		return err
	})
	return result, err
}

// PatchApplicationResource patches a single application resource
func (c *Client) PatchApplicationResource(ctx context.Context, patchReq *application.ApplicationResourcePatchRequest) (*application.ApplicationResourceResponse, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *application.ApplicationResourceResponse
	err := c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appClient.PatchResource(ctx, patchReq)
		return err
	})
	return result, err
}

// DeleteApplicationResource deletes a single application resource
func (c *Client) DeleteApplicationResource(ctx context.Context, deleteReq *application.ApplicationResourceDeleteRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = appClient.DeleteResource(ctx, deleteReq)
		if err != nil {
			return fmt.Errorf("failed to delete application resource: %w", err)
		}
		return nil
	})
}

// TerminateOperation terminates the currently running operation on an application
func (c *Client) TerminateOperation(ctx context.Context, req *application.OperationTerminateRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, appClient, err := c.client.NewApplicationClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = appClient.TerminateOperation(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to terminate operation: %w", err)
		}
		return nil
	})
}

// Project client methods

// ListProjects returns a list of projects
func (c *Client) ListProjects(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProjectList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.AppProjectList
	err := c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = projectClient.List(ctx, query)
		return err
	})
	return result, err
}

// GetProject returns a single project
func (c *Client) GetProject(ctx context.Context, query *project.ProjectQuery) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.AppProject
	err := c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = projectClient.Get(ctx, query)
		return err
	})
	return result, err
}

// CreateProject creates a new project
func (c *Client) CreateProject(ctx context.Context, createReq *project.ProjectCreateRequest) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.AppProject
	err := c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = projectClient.Create(ctx, createReq)
		return err
	})
	return result, err
}

// UpdateProject updates an existing project
func (c *Client) UpdateProject(ctx context.Context, updateReq *project.ProjectUpdateRequest) (*v1alpha1.AppProject, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.AppProject
	err := c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = projectClient.Update(ctx, updateReq)
		return err
	})
	return result, err
}

// DeleteProject deletes a project
func (c *Client) DeleteProject(ctx context.Context, query *project.ProjectQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = projectClient.Delete(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}
		return nil
	})
}

// GetProjectEvents returns events for a project
func (c *Client) GetProjectEvents(ctx context.Context, query *project.ProjectQuery) (*corev1.EventList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *corev1.EventList
	err := c.do(ctx, func() error {
		closer, projectClient, err := c.client.NewProjectClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = projectClient.ListEvents(ctx, query)
		return err
	})
	return result, err
}

// Repository client methods

// ListRepositories returns a list of repositories
func (c *Client) ListRepositories(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.RepositoryList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.RepositoryList
	err := c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = repoClient.List(ctx, query)
		return err
	})
	return result, err
}

// GetRepository returns a single repository
func (c *Client) GetRepository(ctx context.Context, query *repository.RepoQuery) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Repository
	err := c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = repoClient.Get(ctx, query)
		return err
	})
	return result, err
}

// CreateRepository creates a new repository
func (c *Client) CreateRepository(ctx context.Context, createReq *repository.RepoCreateRequest) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Repository
	err := c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = repoClient.Create(ctx, createReq)
		return err
	})
	return result, err
}

// UpdateRepository updates an existing repository
func (c *Client) UpdateRepository(ctx context.Context, updateReq *repository.RepoUpdateRequest) (*v1alpha1.Repository, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Repository
	err := c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = repoClient.Update(ctx, updateReq)
		return err
	})
	return result, err
}

// DeleteRepository deletes a repository
func (c *Client) DeleteRepository(ctx context.Context, query *repository.RepoQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = repoClient.Delete(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to delete repository: %w", err)
		}
		return nil
	})
}

// ValidateRepositoryAccess validates access to a repository
func (c *Client) ValidateRepositoryAccess(ctx context.Context, query *repository.RepoAccessQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, repoClient, err := c.client.NewRepoClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = repoClient.ValidateAccess(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to validate repository: %w", err)
		}
		return nil
	})
}

// Cluster client methods

// ListClusters returns a list of clusters
func (c *Client) ListClusters(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.ClusterList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ClusterList
	err := c.do(ctx, func() error {
		closer, clusterClient, err := c.client.NewClusterClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = clusterClient.List(ctx, query)
		return err
	})
	return result, err
}

// GetCluster returns a single cluster
func (c *Client) GetCluster(ctx context.Context, query *cluster.ClusterQuery) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Cluster
	err := c.do(ctx, func() error {
		closer, clusterClient, err := c.client.NewClusterClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = clusterClient.Get(ctx, query)
		return err
	})
	return result, err
}

// CreateCluster creates a new cluster
func (c *Client) CreateCluster(ctx context.Context, createReq *cluster.ClusterCreateRequest) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Cluster
	err := c.do(ctx, func() error {
		closer, clusterClient, err := c.client.NewClusterClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = clusterClient.Create(ctx, createReq)
		return err
	})
	return result, err
}

// UpdateCluster updates an existing cluster
func (c *Client) UpdateCluster(ctx context.Context, updateReq *cluster.ClusterUpdateRequest) (*v1alpha1.Cluster, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.Cluster
	err := c.do(ctx, func() error {
		closer, clusterClient, err := c.client.NewClusterClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = clusterClient.Update(ctx, updateReq)
		return err
	})
	return result, err
}

// DeleteCluster deletes a cluster
func (c *Client) DeleteCluster(ctx context.Context, query *cluster.ClusterQuery) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, clusterClient, err := c.client.NewClusterClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		_, err = clusterClient.Delete(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to delete cluster: %w", err)
		}
		return nil
	})
}

// Account client methods

// GetAccount returns account information
func (c *Client) GetAccount(ctx context.Context, name string) (*account.Account, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *account.Account
	err := c.do(ctx, func() error {
		closer, accountClient, err := c.client.NewAccountClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = accountClient.GetAccount(ctx, &account.GetAccountRequest{Name: name})
		return err
	})
	return result, err
}

// CanI checks if the current user can perform an action
func (c *Client) CanI(ctx context.Context, action, scope string) (string, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return "", fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result string
	err := c.do(ctx, func() error {
		closer, accountClient, err := c.client.NewAccountClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		resp, err := accountClient.CanI(ctx, &account.CanIRequest{Action: action, Resource: scope})
		if err != nil {
			return fmt.Errorf("failed to check permissions: %w", err)
		}
		result = resp.Value
		return nil
	})
	return result, err
}

// ApplicationSet client methods

// ListApplicationSets returns a list of ApplicationSets
func (c *Client) ListApplicationSets(ctx context.Context, query *applicationset.ApplicationSetListQuery) (*v1alpha1.ApplicationSetList, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ApplicationSetList
	err := c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appSetClient.List(ctx, query)
		return err
	})
	return result, err
}

// GetApplicationSet returns a single ApplicationSet
func (c *Client) GetApplicationSet(ctx context.Context, query *applicationset.ApplicationSetGetQuery) (*v1alpha1.ApplicationSet, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ApplicationSet
	err := c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appSetClient.Get(ctx, query)
		return err
	})
	return result, err
}

// GetApplicationSetResourceTree returns the resource tree for an ApplicationSet
func (c *Client) GetApplicationSetResourceTree(ctx context.Context, query *applicationset.ApplicationSetTreeQuery) (*v1alpha1.ApplicationSetTree, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ApplicationSetTree
	err := c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appSetClient.ResourceTree(ctx, query)
		return err
	})
	return result, err
}

// CreateApplicationSet creates a new ApplicationSet
func (c *Client) CreateApplicationSet(ctx context.Context, req *applicationset.ApplicationSetCreateRequest) (*v1alpha1.ApplicationSet, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result *v1alpha1.ApplicationSet
	err := c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		result, err = appSetClient.Create(ctx, req)
		return err
	})
	return result, err
}

// DeleteApplicationSet deletes an ApplicationSet
func (c *Client) DeleteApplicationSet(ctx context.Context, req *applicationset.ApplicationSetDeleteRequest) error {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	return c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()
		_, err = appSetClient.Delete(ctx, req)
		return err
	})
}

// PreviewApplicationSet calls the Generate API to dry-run an ApplicationSet spec and
// return the list of Applications it would produce, without creating anything.
func (c *Client) PreviewApplicationSet(ctx context.Context, appSet *v1alpha1.ApplicationSet) ([]*v1alpha1.Application, error) {
	if err := c.WaitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}
	var result []*v1alpha1.Application
	err := c.do(ctx, func() error {
		closer, appSetClient, err := c.client.NewApplicationSetClient()
		if err != nil {
			return err
		}
		defer closer.Close()

		resp, err := appSetClient.Generate(ctx, &applicationset.ApplicationSetGenerateRequest{
			ApplicationSet: appSet,
		})
		if err != nil {
			return fmt.Errorf("failed to generate applicationset preview: %w", err)
		}
		result = resp.GetApplications()
		return nil
	})
	return result, err
}

// Ping checks connectivity and auth against the ArgoCD server.
// It logs the server version on success and the authenticated username on auth success.
// Returns an error only if the version check (no-auth) fails; auth failure is logged as a warning.
func (c *Client) Ping(ctx context.Context) error {
	// 1. Version check — no auth required, confirms basic connectivity.
	verCloser, verClient, err := c.client.NewVersionClient()
	if err != nil {
		return fmt.Errorf("failed to create version client: %w", err)
	}
	defer verCloser.Close()

	verResp, err := verClient.Version(ctx, &empty.Empty{})
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}
	c.logger.WithFields(logrus.Fields{
		"server":  c.server,
		"version": verResp.GetVersion(),
	}).Info("Connected to ArgoCD server")

	// 2. Session check — requires a valid token.
	sessCloser, sessClient, err := c.client.NewSessionClient()
	if err != nil {
		c.logger.Warnf("Auth check skipped: failed to create session client: %v", err)
		return nil
	}
	defer sessCloser.Close()

	userInfo, err := sessClient.GetUserInfo(ctx, &session.GetUserInfoRequest{})
	if err != nil {
		c.logger.Warnf("Authentication check failed: %v", err)
		return nil
	}
	c.logger.WithField("username", userInfo.GetUsername()).Info("Authenticated successfully")
	return nil
}

// Server returns the configured server address
func (c *Client) Server() string {
	return c.server
}

// WithTimeout returns a context with timeout
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}
