#!/bin/sh
# Runs both seed scripts in sequence. Either one failing fails the Job —
# a half-seeded environment should not be reported as ready.
set -eu

echo "=== Seeding MLflow ==="
python register_dummy_model.py

echo "=== Seeding OpenMetadata ==="
python seed_sample_tables.py

echo "=== Triggering catalog sync ==="
python trigger_catalog_sync.py

echo "=== Seed complete ==="
