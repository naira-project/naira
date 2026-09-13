#!/usr/bin/env bash
# Seed a sample model into MLflow using the script the team already has.
# Tilt runs this with resource_deps=['mlflow'] and a port-forward already up,
# so the ordering the Taskfile documented ("requires port-forward") is declared.
set -euo pipefail
cd "$(dirname "$0")/../.."
TOOLS=deploy/dev/stacks/mlops/tools/mlflow
VENV=.local/venvs/mlflow-registry
[ -d "$VENV" ] || python3 -m venv "$VENV"
"$VENV/bin/pip" install -q --upgrade pip
"$VENV/bin/pip" install -q -r "$TOOLS/requirements.txt"
MLFLOW_TRACKING_URI=http://127.0.0.1:5000 "$VENV/bin/python" "$TOOLS/register_dummy_model.py"
