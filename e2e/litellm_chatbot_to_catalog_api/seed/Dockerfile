# Build context: this directory. Both scripts are stdlib-only (no
# third-party deps) — this scenario doesn't seed any external system's bulk
# data, it only provisions one scoped LiteLLM key and triggers a catalog
# plugin run.
FROM python:3.11.17-slim@sha256:e88e9763f943ec1834f992a4b51e0f24500486803e8bc534e5767af9ea65f6ce

WORKDIR /seed

COPY provision_chatbot1_key.py trigger_catalog_sync.py ./

CMD ["sh", "-c", "python provision_chatbot1_key.py && python trigger_catalog_sync.py"]
