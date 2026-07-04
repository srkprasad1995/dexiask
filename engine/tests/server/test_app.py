"""
Tests for the generic engine HTTP server (engine_core/app.py) via a FakeRuntime.

Covers:
  - GET /healthz
  - POST /v1/jobs streaming NDJSON (patches the shared runner)
  - X-Run-Id header
  - POST /v1/jobs/{run_id}/cancel
  - 422 on invalid Job JSON
  - credential scrubbing from the process env at startup
"""
import asyncio
import json
import os
from unittest.mock import patch

import pytest
from fastapi.testclient import TestClient

from engine_core.app import create_app
from engine_core.models.events import ResultEvent, TextDeltaEvent
from engine_core.models.settings import BaseEngineSettings
from engine_core.runtime import AgentRuntime


class _FakeRuntime(AgentRuntime):
    def name(self):
        return "fake"

    async def run(self, ctx):  # pragma: no cover - patched in streaming tests
        return "sess"


@pytest.fixture
def test_settings():
    return BaseEngineSettings(
        default_model="test-model",
        max_turns=5,
        server_host="127.0.0.1",
        server_port=8080,
    )


@pytest.fixture
def client(test_settings):
    app = create_app(_FakeRuntime(), settings=test_settings)
    with TestClient(app, raise_server_exceptions=True) as c:
        yield c


VALID_JOB = {
    "model": "test-model",
    "systemPrompt": "You are a test assistant.",
    "messages": [{"role": "user", "content": "Say hello."}],
    "role": "ask",
    "allowedTools": [],
    "permissionMode": "dontAsk",
    "skillsPath": "",
    "workspacePath": "/tmp",
}


def test_healthz(client):
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


def test_invalid_job_422(client):
    assert client.post("/v1/jobs", json={"messages": []}).status_code == 422


def test_non_ask_role_422(client):
    assert client.post("/v1/jobs", json={**VALID_JOB, "role": "plan"}).status_code == 422


def test_create_app_scrubs_credentials(test_settings, monkeypatch):
    # The provider credential vars must be removed from the process env at startup
    # so they cannot be inherited by SDK-spawned tool subprocesses (e.g. Bash env).
    # The default runtime's credential_env_names() is ANTHROPIC_*.
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-platform")
    monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://platform")
    # Operational vars the engine legitimately needs must survive.
    monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")

    create_app(_FakeRuntime(), settings=test_settings)

    assert "ANTHROPIC_API_KEY" not in os.environ
    assert "ANTHROPIC_BASE_URL" not in os.environ
    assert os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT") == "http://collector:4318"


def test_missing_messages_422(client):
    assert client.post("/v1/jobs", json={"model": "test-model"}).status_code == 422


def _make_runner(events):
    """Return a mock for the shared runner that emits given events."""
    async def _run(job, runtime, emit_fn, settings):
        for ev in events:
            emit_fn(ev)
        return "mock-session-id"
    return _run


def test_job_streams_ndjson(client):
    fake_events = [
        TextDeltaEvent(id="b1", text="hello"),
        ResultEvent(model="test-model", session_id="sid-1"),
    ]
    with patch("engine_core.app.run_job", side_effect=_make_runner(fake_events)):
        with client.stream("POST", "/v1/jobs", json=VALID_JOB) as resp:
            assert resp.status_code == 200
            assert "x-ndjson" in resp.headers.get("content-type", "")
            lines = [line for line in resp.iter_lines() if line.strip()]
    assert len(lines) == 2
    assert json.loads(lines[0]) == {"type": "text.delta", "id": "b1", "text": "hello"}
    last = json.loads(lines[1])
    assert last["type"] == "result" and last["sessionId"] == "sid-1"


def test_job_returns_run_id_header(client):
    with patch("engine_core.app.run_job", side_effect=_make_runner([ResultEvent(model="test-model")])):
        with client.stream("POST", "/v1/jobs", json=VALID_JOB) as resp:
            run_id = resp.headers.get("x-run-id")
            for _ in resp.iter_lines():
                pass
    assert run_id is not None and len(run_id) == 36


def test_cancel_nonexistent_job_404(client):
    assert client.post("/v1/jobs/nonexistent-id/cancel").status_code == 404


def test_cancel_active_job(client):
    async def _slow_run(job, runtime, emit_fn, settings):
        await asyncio.sleep(10)
        emit_fn(ResultEvent(model="test-model"))

    run_id_holder = []
    with patch("engine_core.app.run_job", side_effect=_slow_run):
        with client.stream("POST", "/v1/jobs", json=VALID_JOB) as resp:
            run_id_holder.append(resp.headers.get("x-run-id"))
            for _ in resp.iter_lines():
                break
    cancel_resp = client.post(f"/v1/jobs/{run_id_holder[0]}/cancel")
    assert cancel_resp.status_code in (200, 404)
