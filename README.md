# ArgoCD MCP Server

A Model Context Protocol (MCP) server for ArgoCD, allowing you to interact with ArgoCD through MCP-compatible clients like Claude Code.

## Features

- **Application Management**: List, get, create, update, delete, sync, and rollback applications
- **Project Management**: Create and manage ArgoCD projects
- **Repository Management**: Add, update, and remove repository connections
- **Cluster Management**: Configure and manage cluster connections
- **Resource Operations**: Patch, delete, and run actions on application resources

## Installation

### From Source

```bash
git clone https://github.com/argocd-mcp/argocd-mcp.git
cd argocd-mcp
go build -o argocd-mcp ./cmd/argocd-mcp
```

### From Release

Download the appropriate binary from the [releases page](https://github.com/argocd-mcp/argocd-mcp/releases).

## Configuration

Create a configuration file at `~/.config/argocd-mcp/config.yaml`:

```yaml
argocd:
  server: "localhost:8080"
  username: "admin"
  password: "your-password"
  # OR use token directly:
  # token: "your-auth-token"

server:
  mcp_endpoint: "stdio"

logging:
  level: "info"
```

Alternatively, you can use environment variables:

```bash
export ARGOCD_MCP_ARGOCD_SERVER="localhost:8080"
export ARGOCD_MCP_ARGOCD_TOKEN="your-token"
```

## Usage

### Start the MCP Server

```bash
# Using stdio (default, recommended for Claude Code)
./argocd-mcp serve

# Using SSE
./argocd-mcp serve --mcp-endpoint sse
```

### CLI Commands

```bash
# Test connection
./argocd-mcp test

# Show current configuration
./argocd-mcp config show

# Initialize configuration interactively
./argocd-mcp config init
```

## Available Tools

### Application Tools

| Tool | Description |
|------|-------------|
| `list_applications` | List all applications with optional filtering |
| `get_application` | Get detailed information about an application |
| `create_application` | Create a new ArgoCD application |
| `update_application` | Update an existing application |
| `delete_application` | Delete an application |
| `sync_application` | Trigger a manual sync for an application |
| `get_application_manifests` | Get the manifests for an application |
| `get_application_resource` | Get details of a specific resource |
| `patch_application_resource` | Patch a resource within an application |
| `delete_application_resource` | Delete a resource from an application |
| `rollback_application` | Rollback to a previous version |
| `get_application_events` | Get events for an application |
| `list_resource_actions` | List available actions for a resource |
| `run_resource_action` | Run an action on a resource |

### Project Tools

| Tool | Description |
|------|-------------|
| `list_projects` | List all projects |
| `get_project` | Get project details |
| `create_project` | Create a new project |
| `update_project` | Update a project |
| `delete_project` | Delete a project |
| `get_project_events` | Get events for a project |

### Repository Tools

| Tool | Description |
|------|-------------|
| `list_repositories` | List configured repositories |
| `get_repository` | Get repository details |
| `create_repository` | Add a repository connection |
| `update_repository` | Update repository credentials |
| `delete_repository` | Remove a repository |
| `validate_repository` | Validate repository access |

### Cluster Tools

| Tool | Description |
|------|-------------|
| `list_clusters` | List configured clusters |
| `get_cluster` | Get cluster details |
| `create_cluster` | Add a cluster connection |
| `update_cluster` | Update cluster credentials |
| `delete_cluster` | Remove a cluster |

## Using with Claude Code

Add the following to your Claude Code configuration:

```json
{
  "mcpServers": {
    "argocd": {
      "command": "/path/to/argocd-mcp",
      "args": ["serve"],
      "env": {
        "ARGOCD_MCP_ARGOCD_SERVER": "localhost:8080",
        "ARGOCD_MCP_ARGOCD_TOKEN": "your-token"
      }
    }
  }
}
```

## Building for Release

Using [goreleaser](https://goreleaser.com/):

```bash
goreleaser release --snapshot --rm-dist
```

## License

MIT
