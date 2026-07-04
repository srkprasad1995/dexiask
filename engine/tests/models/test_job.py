"""Tests for models/job.py — typed Job and Message models (ask deployment)."""
import pytest
from pydantic import ValidationError

from engine_core.models.job import Attachment, Job, Message, PermissionMode, Role

# ---------------------------------------------------------------------------
# Message
# ---------------------------------------------------------------------------

def test_message_valid():
    m = Message(role="user", content="hello")
    assert m.role == "user"
    assert m.content == "hello"


def test_message_whitespace_only_rejected():
    with pytest.raises(ValidationError):
        Message(role="user", content="   ")


def test_message_empty_rejected():
    with pytest.raises(ValidationError):
        Message(role="user", content="")


# ---------------------------------------------------------------------------
# Job defaults
# ---------------------------------------------------------------------------

def test_job_minimal():
    job = Job.model_validate({"messages": [{"role": "user", "content": "hi"}]})
    assert job.role == Role.ASK
    assert job.model == ""  # filled at runtime from settings
    assert job.permission_mode == PermissionMode.DONT_ASK
    assert job.allowed_tools == []
    assert job.session_id is None
    assert job.mcp_servers == []


def test_job_parses_mcp_servers():
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "mcpServers": [
            {
                "name": "indexer",
                "type": "http",
                "url": "https://mcp.example.com",
                "headers": {"Authorization": "Bearer x"},
                "deferLoading": True,
            },
            {"name": "slack", "type": "sse", "url": "https://slack.example.com"},
        ],
    })
    assert len(job.mcp_servers) == 2
    gh = job.mcp_servers[0]
    assert gh.name == "indexer"
    assert gh.type == "http"
    assert gh.headers == {"Authorization": "Bearer x"}
    assert gh.defer_loading is True
    # Defaults when omitted.
    assert job.mcp_servers[1].headers == {}
    assert job.mcp_servers[1].defer_loading is False


def test_job_default_model_resolved():
    job = Job.model_validate({"messages": [{"role": "user", "content": "hi"}]})
    assert job.resolved_model("claude-sonnet-5") == "claude-sonnet-5"


def test_job_explicit_model_resolved():
    job = Job.model_validate({
        "model": "claude-opus-4-8",
        "messages": [{"role": "user", "content": "hi"}],
    })
    assert job.resolved_model("claude-sonnet-5") == "claude-opus-4-8"


def test_job_camel_case_aliases():
    job = Job.model_validate({
        "systemPrompt": "Be concise.",
        "allowedTools": ["Read", "Grep"],
        "permissionMode": "dontAsk",
        "skillsPath": "/custom/skills",
        "workspacePath": "/custom/ws",
        "messages": [{"role": "user", "content": "hi"}],
    })
    assert job.system_prompt == "Be concise."
    assert job.allowed_tools == ["Read", "Grep"]
    assert job.permission_mode == PermissionMode.DONT_ASK
    assert job.skills_path == "/custom/skills"
    assert job.workspace_path == "/custom/ws"


def test_job_session_id():
    job = Job.model_validate({
        "sessionId": "abc-123",
        "sessionStorePath": "/workspace/w/conversations/c/session",
        "messages": [{"role": "user", "content": "hi"}],
    })
    assert job.session_id == "abc-123"
    assert job.session_store_path == "/workspace/w/conversations/c/session"


# ---------------------------------------------------------------------------
# Role enum — this deployment only runs ask.
# ---------------------------------------------------------------------------

def test_ask_role_default_and_explicit():
    assert Role("ask") is Role.ASK
    job = Job.model_validate({
        "role": "ask",
        "messages": [{"role": "user", "content": "x"}],
    })
    assert job.role is Role.ASK


@pytest.mark.parametrize("value", ["plan", "code", "test", "deploy", "execute", "project.brd"])
def test_non_ask_roles_rejected(value):
    """Only the ask role is supported; every other role 422s at validation."""
    with pytest.raises(ValidationError):
        Job.model_validate({
            "role": value,
            "messages": [{"role": "user", "content": "x"}],
        })


def test_invalid_role_rejected():
    with pytest.raises(ValidationError):
        Job.model_validate({
            "role": "unknown",
            "messages": [{"role": "user", "content": "x"}],
        })


# ---------------------------------------------------------------------------
# PermissionMode enum — trimmed to the two modes ask turns use.
# ---------------------------------------------------------------------------

@pytest.mark.parametrize("mode", ["dontAsk", "default"])
def test_permission_mode_values(mode):
    job = Job.model_validate({
        "permissionMode": mode,
        "messages": [{"role": "user", "content": "x"}],
    })
    assert job.permission_mode.value == mode


@pytest.mark.parametrize("mode", ["acceptEdits", "bypassPermissions"])
def test_write_permission_modes_rejected(mode):
    """Write-oriented permission modes are not accepted by the read-only engine."""
    with pytest.raises(ValidationError):
        Job.model_validate({
            "permissionMode": mode,
            "messages": [{"role": "user", "content": "x"}],
        })


# ---------------------------------------------------------------------------
# Extra keys tolerated (forward-compat with orchestrator fields)
# ---------------------------------------------------------------------------

def test_leftover_unknown_key_is_ignored():
    job = Job.model_validate({
        "messages": [{"role": "user", "content": "hi"}],
        "projectPath": "/projects/p1",  # unused by this deployment
    })
    assert not hasattr(job, "project_path")


# ---------------------------------------------------------------------------
# Message filtering
# ---------------------------------------------------------------------------

def test_whitespace_messages_rejected_at_message_level():
    with pytest.raises(ValidationError) as exc_info:
        Job.model_validate({
            "messages": [
                {"role": "user", "content": "hello"},
                {"role": "assistant", "content": "  "},
                {"role": "user", "content": "follow up"},
            ],
        })
    assert "whitespace" in str(exc_info.value).lower()


def test_no_messages_rejected():
    with pytest.raises(ValidationError):
        Job.model_validate({"messages": []})


# ---------------------------------------------------------------------------
# Attachment support
# ---------------------------------------------------------------------------

def test_attachment_parsed():
    msg = Message.model_validate({
        "role": "user",
        "content": "Look at this screenshot.",
        "attachments": [
            {
                "kind": "image",
                "path": "/workspace/attachments/conv1/abc-shot.png",
                "mediaType": "image/png",
                "filename": "shot.png",
            }
        ],
    })
    assert len(msg.attachments) == 1
    assert msg.attachments[0].kind == "image"
    assert msg.attachments[0].media_type == "image/png"


def test_attachment_camel_case_alias():
    att = Attachment.model_validate({
        "kind": "file",
        "path": "/workspace/attachments/conv1/abc-app.yaml",
        "mediaType": "text/yaml",
        "filename": "app.yaml",
    })
    assert att.media_type == "text/yaml"


def test_image_only_message_allowed():
    msg = Message.model_validate({
        "role": "user",
        "content": "",
        "attachments": [
            {
                "kind": "image",
                "path": "/workspace/attachments/c/x.png",
                "mediaType": "image/png",
                "filename": "x.png",
            }
        ],
    })
    assert msg.content == ""
    assert len(msg.attachments) == 1


def test_empty_message_without_attachments_still_rejected():
    with pytest.raises(ValidationError):
        Message.model_validate({"role": "user", "content": "  "})
