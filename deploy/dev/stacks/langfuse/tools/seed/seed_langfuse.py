"""Seed the local Langfuse project with traces, generations and scores.

The plugin reads aggregates, so it needs something to aggregate. This writes a
spread of LLM activity for the two demo applications, then replays the exact
queries the plugin builds against the real metrics API. That second step is
the point: the plugin's view, measure and column names came from the docs, and
this proves them against a running Langfuse.

Two ingestion paths, because v4 defaults to
LANGFUSE_MIGRATION_V4_WRITE_MODE=events_only: /api/public/ingestion then takes
score events only, and traces and generations must arrive over OTLP. Writing
the OTLP payload by hand keeps the dependencies at `requests` and the
attributes Langfuse reads visible in one place. The `dual` mode would accept
the old shapes, but it is a documented-temporary bridge for v3 upgrades.

Usage:
    LANGFUSE_HOST=http://127.0.0.1:3003 python seed_langfuse.py
"""

import json
import os
import random
import sys
import time
import uuid
from datetime import datetime, timedelta, timezone

import requests

HOST = os.environ.get("LANGFUSE_HOST", "http://127.0.0.1:3003").rstrip("/")
PUBLIC_KEY = os.environ.get("LANGFUSE_PUBLIC_KEY", "pk-lf-naira-dev")
SECRET_KEY = os.environ.get("LANGFUSE_SECRET_KEY", "sk-lf-naira-dev")

AUTH = (PUBLIC_KEY, SECRET_KEY)
TIMEOUT = 60

# Ingestion is async: the API accepts spans, a worker moves them into
# ClickHouse.
INGESTION_SETTLE_ATTEMPTS = 30
INGESTION_SETTLE_INTERVAL = 10

# Deterministic so a reseed reports comparable numbers.
random.seed(20260930)


class App:
    """One demo application: a trace name, its window, and its score."""

    def __init__(self, trace_name, window_hours, traces, score_name, score_values, models):
        self.trace_name = trace_name
        self.window_hours = window_hours
        self.traces = traces
        self.score_name = score_name
        self.score_values = score_values
        self.models = models


# The windows mirror the plugin config so the seeded spread and the aggregated
# window agree; seeding 7 days of data for a 24h window would hide most of it.
APPS = [
    App(
        trace_name="rag-assistant",
        window_hours=24,
        traces=120,
        # A thumbs up/down ratio: the aggregate mean is the share of positives.
        score_name="user_feedback",
        score_values=[0, 1],
        models=[
            ("gpt-4o-mini", 0.00000015, 0.0000006),
            ("gpt-4o", 0.0000025, 0.00001),
        ],
    ),
    App(
        trace_name="triage-agent",
        window_hours=168,
        traces=200,
        # A 1-5 rating, to show the UI does not assume a 0-1 scale.
        score_name="helpfulness",
        score_values=[1, 2, 3, 4, 5],
        models=[
            ("claude-haiku-4-5", 0.000001, 0.000005),
        ],
    ),
]


def iso(moment):
    return moment.astimezone(timezone.utc).isoformat()


def unix_nano(moment):
    """OTLP timestamps: nanoseconds since the epoch, as strings.

    Scaling float seconds straight to nanoseconds exceeds float64's exact
    range, so microseconds are taken as an int first.
    """
    return str(int(moment.timestamp() * 1_000_000) * 1000)


def attribute(key, value):
    """One OTLP key/value. bool is checked first, being a subclass of int."""
    if isinstance(value, bool):
        typed = {"boolValue": value}
    elif isinstance(value, int):
        typed = {"intValue": str(value)}
    elif isinstance(value, float):
        typed = {"doubleValue": value}
    else:
        typed = {"stringValue": str(value)}
    return {"key": key, "value": typed}


def build_spans(app, now):
    """Builds the OTLP spans for one app, plus its score events.

    Each trace is a root span with generation children. Langfuse's own
    attribute names are used over the gen_ai.* ones: it reads both, but its
    own are unambiguous about observation type and cost.
    """
    spans = []
    scores = []

    for _ in range(app.traces):
        # OTLP ids are hex bytes: 16 for a trace, 8 for a span. Langfuse
        # reuses the OTel trace id, so the scores below attach by traceId.
        trace_id = uuid.uuid4().hex
        root_span_id = uuid.uuid4().hex[:16]

        # Headroom at both ends: a trace starting at `now` would have
        # children ending past toTimestamp, and one at the far edge would
        # lose children off fromTimestamp. Either way counts stop matching.
        window_seconds = app.window_hours * 3600
        started = now - timedelta(
            seconds=random.uniform(window_seconds * 0.03, window_seconds * 0.97)
        )

        generations = []
        cursor = started
        for step in range(random.randint(1, 3)):
            model, input_rate, output_rate = random.choice(app.models)
            input_tokens = random.randint(200, 3000)
            output_tokens = random.randint(50, 800)
            # Skewed so the averages are not uniform noise and a p95-style tail
            # exists if anyone looks for one.
            latency_seconds = random.lognormvariate(0.2, 0.6)
            ended = cursor + timedelta(seconds=latency_seconds)

            input_cost = input_tokens * input_rate
            output_cost = output_tokens * output_rate

            generations.append(
                {
                    "traceId": trace_id,
                    "spanId": uuid.uuid4().hex[:16],
                    "parentSpanId": root_span_id,
                    "name": f"llm-call-{step + 1}",
                    "kind": 3,  # SPAN_KIND_CLIENT
                    # Latency is the difference between these two, so they are
                    # what the plugin's avg_latency ultimately measures.
                    "startTimeUnixNano": unix_nano(cursor),
                    "endTimeUnixNano": unix_nano(ended),
                    "attributes": [
                        # Without this it lands as a plain span, uncounted
                        # by the observations view.
                        attribute("langfuse.observation.type", "generation"),
                        # On every span: the traceName column the plugin
                        # filters on.
                        attribute("langfuse.trace.name", app.trace_name),
                        attribute("langfuse.observation.model.name", model),
                        attribute(
                            "langfuse.observation.usage_details",
                            json.dumps(
                                {
                                    "input": input_tokens,
                                    "output": output_tokens,
                                    "total": input_tokens + output_tokens,
                                }
                            ),
                        ),
                        # Stated, not left to Langfuse's price table, so
                        # totals do not shift when it updates — or come out
                        # zero for a model it does not know.
                        attribute(
                            "langfuse.observation.cost_details",
                            json.dumps(
                                {
                                    "input": input_cost,
                                    "output": output_cost,
                                    "total": input_cost + output_cost,
                                }
                            ),
                        ),
                    ],
                }
            )
            cursor = ended

        spans.append(
            {
                "traceId": trace_id,
                "spanId": root_span_id,
                "name": app.trace_name,
                "kind": 1,  # SPAN_KIND_INTERNAL
                "startTimeUnixNano": unix_nano(started),
                "endTimeUnixNano": unix_nano(cursor),
                "attributes": [
                    attribute("langfuse.trace.name", app.trace_name),
                    attribute("langfuse.user.id", f"user-{random.randint(1, 25)}"),
                    attribute("langfuse.session.id", f"session-{random.randint(1, 60)}"),
                    attribute("langfuse.trace.tags", json.dumps(["naira-demo"])),
                ],
            }
        )
        spans.extend(generations)

        # Not every trace is rated, so the score count differs from the
        # observation count on the tiles.
        if random.random() < 0.7:
            scores.append(
                {
                    "id": str(uuid.uuid4()),
                    "type": "score-create",
                    "timestamp": iso(cursor),
                    "body": {
                        "id": str(uuid.uuid4()),
                        "traceId": trace_id,
                        "name": app.score_name,
                        "value": random.choice(app.score_values),
                        "dataType": "NUMERIC",
                    },
                }
            )

    return spans, scores


def ingest_spans(spans):
    """Posts spans to the OTLP endpoint in batches."""
    batch_size = 200
    sent = 0

    for start in range(0, len(spans), batch_size):
        batch = spans[start : start + batch_size]
        payload = {
            "resourceSpans": [
                {
                    "resource": {
                        "attributes": [attribute("service.name", "naira-langfuse-seed")]
                    },
                    "scopeSpans": [
                        {"scope": {"name": "naira-seed"}, "spans": batch},
                    ],
                }
            ]
        }

        resp = requests.post(
            f"{HOST}/api/public/otel/v1/traces",
            json=payload,
            auth=AUTH,
            timeout=TIMEOUT,
        )
        if resp.status_code >= 400:
            raise SystemExit(
                f"OTLP ingestion failed: {resp.status_code} {resp.text[:800]}"
            )

        # OTLP answers 200 with partialSuccess when it drops spans.
        body = resp.json() if resp.content else {}
        partial = body.get("partialSuccess") or {}
        if partial.get("rejectedSpans"):
            raise SystemExit(
                f"OTLP rejected {partial['rejectedSpans']} spans: "
                f"{partial.get('errorMessage', '')}"
            )

        sent += len(batch)
        print(f"  sent {sent}/{len(spans)} spans", flush=True)

    return sent


def ingest_scores(events):
    """Posts score events, the one event type v4 still takes on this endpoint."""
    batch_size = 100
    accepted = 0

    for start in range(0, len(events), batch_size):
        batch = events[start : start + batch_size]
        resp = requests.post(
            f"{HOST}/api/public/ingestion",
            json={"batch": batch},
            auth=AUTH,
            timeout=TIMEOUT,
        )
        # Ingestion answers 207: each event succeeds or fails on its own, so a
        # 2xx status alone does not mean the data landed.
        if resp.status_code >= 400:
            raise SystemExit(f"score ingestion failed: {resp.status_code} {resp.text[:500]}")

        payload = resp.json() if resp.content else {}
        errors = payload.get("errors") or []
        if errors:
            raise SystemExit(f"score ingestion rejected events: {json.dumps(errors[:3], indent=2)}")

        accepted += len(payload.get("successes") or batch)
        print(f"  ingested {accepted}/{len(events)} scores", flush=True)

    return accepted


def metrics_query(view, metrics, filters, from_time, to_time):
    """Runs one metrics query the same way the plugin does.

    Mirrors plugins/cmd/langfuse/client.go: a GET with the query object JSON
    encoded into the `query` parameter, authenticated with the key pair as HTTP
    Basic credentials.
    """
    query = {
        "view": view,
        "metrics": metrics,
        "dimensions": [],
        "filters": filters,
        "fromTimestamp": iso(from_time),
        "toTimestamp": iso(to_time),
    }

    resp = requests.get(
        f"{HOST}/api/public/v2/metrics",
        params={"query": json.dumps(query)},
        auth=AUTH,
        timeout=TIMEOUT,
    )
    if resp.status_code >= 400:
        raise SystemExit(
            f"metrics query failed: {resp.status_code} {resp.text[:500]}\n"
            f"query was: {json.dumps(query, indent=2)}"
        )
    return resp.json().get("data", [])


def trace_name_filter(trace_name):
    return [
        {
            "column": "traceName",
            "operator": "=",
            "value": trace_name,
            "type": "string",
        }
    ]


def score_name_filter(score_name):
    return {
        "column": "name",
        "operator": "=",
        "value": score_name,
        "type": "string",
    }


def row_count(rows):
    """Reads count_count out of a metrics row, treating absent as zero."""
    if not rows:
        return 0
    return int(rows[0].get("count_count") or 0)


def observation_count(app, now):
    return row_count(
        metrics_query(
            "observations",
            [{"measure": "count", "aggregation": "count"}],
            trace_name_filter(app.trace_name),
            now - timedelta(hours=app.window_hours),
            now,
        )
    )


def score_count(app, now):
    return row_count(
        metrics_query(
            "scores-numeric",
            [{"measure": "count", "aggregation": "count"}],
            trace_name_filter(app.trace_name) + [score_name_filter(app.score_name)],
            now - timedelta(hours=app.window_hours),
            now,
        )
    )


def wait_for_ingestion(app, now, expected):
    """Polls until everything seeded for this app is queryable.

    Waiting for the first row is not enough: ingestion streams in, and a
    half-filled window yields aggregates that look plausible and are wrong.
    This waits for the counts sent, or for them to stop moving.
    """
    expected_observations, expected_scores = expected
    previous = None

    for attempt in range(1, INGESTION_SETTLE_ATTEMPTS + 1):
        observations = observation_count(app, now)
        scores = score_count(app, now)

        if observations >= expected_observations and scores >= expected_scores:
            print(
                f"  {app.trace_name}: {observations} observations, "
                f"{scores} scores — complete",
                flush=True,
            )
            return True

        # Stopped moving means ingestion is done and something is missing.
        current = (observations, scores)
        if current == previous and attempt > 2:
            print(
                f"  {app.trace_name}: stalled at {observations}/"
                f"{expected_observations} observations, {scores}/"
                f"{expected_scores} scores",
                flush=True,
            )
            return False
        previous = current

        print(
            f"  {app.trace_name}: {observations}/{expected_observations} "
            f"observations, {scores}/{expected_scores} scores "
            f"({attempt}/{INGESTION_SETTLE_ATTEMPTS})",
            flush=True,
        )
        time.sleep(INGESTION_SETTLE_INTERVAL)

    return False


def verify(app, now, expected):
    """Replays the plugin's two queries and prints the aggregates."""
    expected_observations, expected_scores = expected
    from_time = now - timedelta(hours=app.window_hours)

    observations = metrics_query(
        "observations",
        [
            {"measure": "totalCost", "aggregation": "sum"},
            {"measure": "latency", "aggregation": "avg"},
            {"measure": "count", "aggregation": "count"},
        ],
        trace_name_filter(app.trace_name),
        from_time,
        now,
    )

    scores = metrics_query(
        "scores-numeric",
        [
            {"measure": "value", "aggregation": "avg"},
            {"measure": "count", "aggregation": "count"},
        ],
        trace_name_filter(app.trace_name) + [score_name_filter(app.score_name)],
        from_time,
        now,
    )

    print(f"\n  {app.trace_name} (last {app.window_hours}h)")
    print(f"    observations -> {json.dumps(observations)}")
    print(f"    scores       -> {json.dumps(scores)}")
    print(
        f"    expected {expected_observations} observations, "
        f"{expected_scores} scores"
    )

    # The keys the plugin reads. Naming them here means a Langfuse rename is
    # caught by the seeder, not by a blank tile in the Naira UI.
    checks = {
        "observations": (observations, ["sum_totalCost", "avg_latency", "count_count"]),
        "scores-numeric": (scores, ["avg_value", "count_count"]),
    }

    ok = True
    for view, (rows, keys) in checks.items():
        if not rows:
            print(f"    WARNING: {view} returned no rows")
            ok = False
            continue
        missing = [key for key in keys if key not in rows[0]]
        if missing:
            print(f"    WARNING: {view} row is missing {missing}")
            print(f"             keys present: {sorted(rows[0].keys())}")
            ok = False

    return ok


def main():
    now = datetime.now(timezone.utc)
    expected = {}

    print(f"Seeding Langfuse at {HOST}")
    for app in APPS:
        spans, scores = build_spans(app, now)
        # Only generations are observations; the root span is the trace.
        generations = sum(1 for span in spans if "parentSpanId" in span)

        # Measured before ingesting, so a reseed onto existing data still
        # knows what "done" means. Otherwise the earlier rows alone satisfy
        # the target and the wait returns mid-stream.
        baseline_observations = observation_count(app, now)
        baseline_scores = score_count(app, now)
        expected[app.trace_name] = (
            baseline_observations + generations,
            baseline_scores + len(scores),
        )

        print(
            f"\n{app.trace_name}: {len(spans)} spans "
            f"({generations} generations) over {app.window_hours}h"
        )
        if baseline_observations or baseline_scores:
            print(
                f"  already present: {baseline_observations} observations, "
                f"{baseline_scores} scores — adding to them"
            )
        ingest_spans(spans)
        ingest_scores(scores)

    print("\nWaiting for ingestion to reach ClickHouse...")
    settled = [wait_for_ingestion(app, now, expected[app.trace_name]) for app in APPS]

    # Runs either way: the aggregates show whether it is a shortfall or a
    # broken query.
    print("\nReplaying the plugin's metrics queries against the real API:")
    verified = [verify(app, now, expected[app.trace_name]) for app in APPS]

    if not all(settled):
        print(
            "\nNot everything that was accepted became queryable. The counts "
            "above say how far it got. If they stalled short, the worker "
            "dropped events rather than lagging — check its log:\n"
            "  kubectl -n langfuse logs deploy/langfuse-worker --tail=100",
            file=sys.stderr,
        )
        return 1

    if not all(verified):
        print(
            "\nThe metrics API answered, but not with the keys the plugin "
            "expects. Reconcile plugins/cmd/langfuse/client.go with the output "
            "above before trusting the UI tiles.",
            file=sys.stderr,
        )
        return 1

    print(
        "\nAll queries returned the keys the plugin reads, over the full "
        "seeded dataset. Trigger the langfuse plugin from the UI's Plugins & "
        "Ingestion dialog to pull these into the catalog."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
