# Agent Instructions

## Scope

These instructions apply to the entire repository.

## Project Overview

This repository contains a Go CLI named `aws-eip-binding`. The tool binds a
target IP address to the current EC2 instance:

- IPv4 targets use Elastic IP association APIs.
- IPv6 targets are assigned to the instance primary ENI, moving the address
  from another ENI first when needed.
- `POD_NAME` can be passed as the CLI argument to resolve the target IP from
  environment variables for Kubernetes init-container use.

Core package code lives under `eip/`. The executable entry point is `main.go`.

## Common Commands

- Format Go code: `gofmt -w . && go fix ./... && go mod tidy`
- Run unit tests: `go test ./...`
- Run one test: `go test ./... -run TestName`
- Build the CLI: `go build -o aws-eip-binding`
- Build the container image: `docker build -t aws-eip-binding .`
- Check Terraform E2E formatting:
  `terraform -chdir=test/e2e/terraform fmt -recursive -check`
- Validate Terraform E2E config after provider init:
  `terraform -chdir=test/e2e/terraform validate`
- Run Terraform-backed AWS E2E tests:
  `AWS_REGION=us-east-1 scripts/e2e-terraform.sh`

The Terraform E2E test creates real AWS infrastructure and runs the CLI on a
temporary EC2 instance through SSM. Use an isolated test account or region.

## Coding Guidelines

- Follow idiomatic Go and keep files formatted with `gofmt`.
- Keep AWS and IMDS access behind small interfaces so binder behavior can be
  unit tested without real network calls.
- Prefer `net/netip` for IP parsing and address-family decisions.
- Keep CLI parsing and environment resolution in `eip/config.go`; keep AWS
  binding behavior in `eip/binder.go`.
- Avoid adding new dependencies unless they materially simplify the code.
- Do not use AWS SDK helper functions such as `aws.String`, `aws.Int`, or similar helpers to allocate pointer values. This project targets Go 1.26, so use `new(expr)` instead.

## Testing Guidelines

- Add or update unit tests for behavioral changes in `eip/` and `main_test.go`.
- Unit tests should not require real AWS credentials, IMDS access, or networked
  EC2 endpoints.
- Do not add cloud-emulator-dependent tests; use the Terraform-backed AWS E2E
  harness for EC2/IMDS/IAM behavior.
- When changing IPv6 behavior, cover subnet CIDR checks, ENI selection, and
  address move/unassign paths.
