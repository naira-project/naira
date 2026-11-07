# Build context: this directory. The script is stdlib-only (no third-party
# deps): this scenario seeds no data, it only waits until the wikis answer.
FROM python:3.11.17-slim@sha256:e88e9763f943ec1834f992a4b51e0f24500486803e8bc534e5767af9ea65f6ce

WORKDIR /seed

COPY wait_for_wikis.py ./

CMD ["python", "wait_for_wikis.py"]
