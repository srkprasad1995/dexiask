"""Tests for models/settings.py — env-driven config."""

from claude_engine.models.settings import Settings, get_settings


def test_defaults():
    s = Settings(_env_file=None)
    assert s.default_model == "claude-sonnet-5"
    assert s.max_turns == 20
    assert s.max_tokens == 8192
    assert s.server_port == 8080
    assert s.max_read_lines == 2000


def test_env_override(monkeypatch):
    monkeypatch.setenv("CLAUDE_ENGINE_MAX_TURNS", "50")
    monkeypatch.setenv("CLAUDE_ENGINE_SERVER_PORT", "9090")
    s = Settings(_env_file=None)
    assert s.max_turns == 50
    assert s.server_port == 9090


def test_default_model_from_claude_engine_model_env(monkeypatch):
    # The pinned env var is the bare CLAUDE_ENGINE_MODEL.
    monkeypatch.setenv("CLAUDE_ENGINE_MODEL", "claude-opus-4-8")
    s = Settings(_env_file=None)
    assert s.default_model == "claude-opus-4-8"


def test_anthropic_key_via_bare_env(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-test-123")
    s = Settings(_env_file=None)
    assert s.anthropic_api_key == "sk-test-123"
    # The runner reads it through the engine_api_key accessor.
    assert s.engine_api_key == "sk-test-123"


def test_get_settings_returns_singleton():
    get_settings.cache_clear()
    a = get_settings()
    b = get_settings()
    assert a is b


def test_get_settings_cache_clear(monkeypatch):
    get_settings.cache_clear()
    monkeypatch.setenv("CLAUDE_ENGINE_MAX_TURNS", "99")
    s1 = get_settings()
    assert s1.max_turns == 99
    get_settings.cache_clear()  # reset for other tests
