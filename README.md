# GitopsCompose

GitopsCompose is a GitOps continuous delivery tool for single node Docker Compose deployments.

## How it works

- It repeatedly checks a local repository for remote changes
- If changes exist:
  1. Get all `docker-compose.yml` files of the local git head
  2. Get all `docker-compose.yml` files of the remote git head
  3. Stop all removed compose stacks
  4. Pull changes
  5. Start all new compose stacks
  6. Prepare (image pull) and restart (stop and start) compose stacks that have changed or are not running

> GitopsCompose tries to exit early when errors occur (e.g. when the local repository is not clean). When it proceeds to step 3, errors are tracked but all operations continue (e.g. a failed stop of a removed deployment will not prevent other stacks to be updated).

## How you could use it

Maintain a GIT repository to store all deployments on your host:

```text
.
├── my_app
│   └── production
│       └── docker-compose.yml
│       └── .env
│   └── staging
│       └── docker-compose.yml
│       └── .env
├── reverse_proxy
│   └── docker-compose.yml
├── monitoring
│   └── docker-compose.yml
├── gitops (do not trigger updates remotely)
│   └── docker-compose.yml
```

## Monitoring

Prometheus metrics are exported under "localhost:2112"

```yaml
# HELP gitops_check_error_total Total number of failed checks.
# TYPE gitops_check_error_total counter
gitops_check_error_total 0
# HELP gitops_check_last_error_timestamp_seconds Unix timestamp of the last failed check.
# TYPE gitops_check_last_error_timestamp_seconds gauge
gitops_check_last_error_timestamp_seconds 0
# HELP gitops_check_last_success_timestamp_seconds Unix timestamp of the last successful check.
# TYPE gitops_check_last_success_timestamp_seconds gauge
gitops_check_last_success_timestamp_seconds 1.745424793e+09
# HELP gitops_check_last_timestamp_seconds Unix timestamp of the last check.
# TYPE gitops_check_last_timestamp_seconds gauge
gitops_check_last_timestamp_seconds 1.745424793e+09
# HELP gitops_check_success_total Total number of successful checks.
# TYPE gitops_check_success_total counter
gitops_check_success_total 1
# HELP gitops_deployment_active_total Number of active deployments by status
# TYPE gitops_deployment_active_total gauge
gitops_deployment_active_total{status="error"} 0
gitops_deployment_active_total{status="ok"} 0
gitops_deployment_active_total{status="removal_failed"} 0
# HELP gitops_deployment_error_total Total number of failed deployments.
# TYPE gitops_deployment_error_total counter
gitops_deployment_error_total 0
# HELP gitops_deployment_success_total Total number of successful deployments.
# TYPE gitops_deployment_success_total counter
gitops_deployment_success_total 0
```
