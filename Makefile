.PHONY: setup build test lint deploy-local verify clean

setup:
	@echo "Setting up local development environment..."
	./local-setup/scripts/setup.sh

build:
	@echo "Building Naira services (placeholder)..."

test:
	@echo "Running tests..."

lint:
	@echo "Running linting..."

deploy-local:
	@echo "Deploying Naira locally..."
	kubectl apply -k local-setup/kustomize/base

verify:
	@echo "Verifying local setup..."
	./local-setup/scripts/verify.sh

clean:
	@echo "Cleaning up local environment..."
	kind delete cluster --name naira-local || true
