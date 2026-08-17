# Build context: repo root (matches the plugin/catalog Dockerfile convention —
# see deploy/dev/stacks/core/tasks/docker.yml).
#
# Packages the two existing local-dev seed scripts (unmodified) into one image
# so a Kubernetes Job can run them against a fresh per-PR MLflow + OpenMetadata.
# No image previously existed for this — the dev workflow runs these scripts
# from a local venv via `task seed:all`, which assumes a developer's machine,
# not a Job.
FROM python:3.11-slim

WORKDIR /seed

COPY deploy/dev/stacks/mlops/tools/mlflow/requirements.txt ./mlflow-requirements.txt
COPY deploy/dev/stacks/mlops/tools/openmetadata/requirements.txt ./openmetadata-requirements.txt
RUN pip install --no-cache-dir \
      -r mlflow-requirements.txt \
      -r openmetadata-requirements.txt

COPY deploy/dev/stacks/mlops/tools/mlflow/register_dummy_model.py ./register_dummy_model.py
COPY deploy/dev/stacks/mlops/tools/openmetadata/seed_sample_tables.py ./seed_sample_tables.py
COPY deploy/e2e/seed/trigger_catalog_sync.py ./trigger_catalog_sync.py
COPY deploy/e2e/seed/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]
