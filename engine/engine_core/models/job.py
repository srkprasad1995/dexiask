"""
Agent Job Protocol — Job model.

JSON field names are camelCase (matching what the Go orchestrator sends); Python
attribute names are snake_case.

This is the ``ask`` deployment of the engine: read-only assistant turns only, so
the role set is a single ``ask`` role and the permission modes are trimmed to the
two the engine actually uses.
"""
from __future__ import annotations

from enum import Enum

from pydantic import BaseModel, ConfigDict, Field, model_validator

# ---------------------------------------------------------------------------
# Enums
# ---------------------------------------------------------------------------

class Role(str, Enum):
    """Role this job is performing. This engine only runs ``ask``."""
    ASK = "ask"


class PermissionMode(str, Enum):
    """Tool permission mode passed to the Agent SDK.

    ``ask`` turns are read-only, so only two modes are used: ``dontAsk``
    (auto-approve the pre-allowed read tools, deny all others) and ``default``
    (standard prompting). The other SDK modes (acceptEdits / bypassPermissions)
    are write-oriented and never sent to this deployment.
    """
    DONT_ASK = "dontAsk"    # auto-approve pre-allowed tools, deny all others
    DEFAULT = "default"      # standard prompting


# ---------------------------------------------------------------------------
# Models
# ---------------------------------------------------------------------------

class Attachment(BaseModel):
    """A file attached to a message turn."""

    model_config = ConfigDict(populate_by_name=True)

    kind: str = Field(..., description="'image' or 'file'")
    path: str = Field(..., description="Absolute path inside the container workspace.")
    media_type: str = Field(..., alias="mediaType", description="MIME type.")
    filename: str = Field(..., description="Original filename.")


class McpServerConfig(BaseModel):
    """A remote MCP server attached to a job.

    Maps directly onto the Claude Agent SDK's McpHttpServerConfig /
    McpSSEServerConfig. The orchestrator resolves which servers apply and
    decrypts headers before sending.
    """

    model_config = ConfigDict(populate_by_name=True)

    name: str = Field(..., description="Server name; becomes the mcp__<name> tool namespace.")
    type: str = Field(..., description="'http' or 'sse'.")
    url: str = Field(..., description="Remote server URL.")
    description: str = Field(
        default="",
        description="High-level summary of the server, surfaced to the agent for routing.",
    )
    headers: dict[str, str] = Field(default_factory=dict, description="HTTP headers (e.g. auth).")
    defer_loading: bool = Field(
        default=False,
        alias="deferLoading",
        description=(
            "When true the server is lazy-loaded: it is not attached to the SDK; "
            "its tools are discovered/invoked on demand through the in-process "
            "'mcp_router' tool-search proxy so their definitions stay out of context "
            "until the agent searches for them. When false the server attaches natively."
        ),
    )


class Message(BaseModel):
    """A single conversation turn."""

    model_config = ConfigDict(populate_by_name=True)

    role: str = Field(..., description="'user' or 'assistant'")
    content: str = Field(default="", description="Message text content. May be empty when attachments are present.")
    attachments: list[Attachment] = Field(default_factory=list, description="Files attached to this turn.")

    @model_validator(mode="after")
    def content_or_attachments(self) -> Message:
        if not self.content.strip() and not self.attachments:
            raise ValueError("Message content must not be whitespace-only when no attachments are present.")
        return self


class ThinkingConfig(BaseModel):
    """Extended (adaptive) thinking configuration."""

    model_config = ConfigDict(populate_by_name=True)

    type: str = Field(default="adaptive", description="Always 'adaptive' when present.")
    display: str = Field(
        default="summarized",
        description="'summarized' returns a reasoning summary; 'omitted' hides it.",
    )


class Job(BaseModel):
    """
    A single agent execution request.

    JSON keys use camelCase (Go convention); Python attributes use snake_case.
    Validation rejects jobs where all messages have empty content and no attachments.
    """

    model_config = ConfigDict(populate_by_name=True)

    # Required fields from the orchestrator.
    model: str = Field(
        default="",
        description="Model identifier.  Falls back to Settings.default_model when empty.",
    )
    system_prompt: str = Field(
        default="",
        alias="systemPrompt",
        description="System prompt composed by the Go orchestrator.",
    )
    messages: list[Message] = Field(
        ...,
        min_length=1,
        description="Conversation history.  At least one non-empty message required.",
    )
    role: Role = Field(
        default=Role.ASK,
        description="Role determining tools and system prompt.",
    )
    allowed_tools: list[str] = Field(
        default_factory=list,
        alias="allowedTools",
        description="Tool names the model may call without a permission prompt.",
    )
    permission_mode: PermissionMode = Field(
        default=PermissionMode.DONT_ASK,
        alias="permissionMode",
        description="How to handle tool-use permission requests.",
    )

    # Mount paths — default to container mount points.
    skills_path: str = Field(
        default="/skills",
        alias="skillsPath",
        description="Container path where skill packs (*/SKILL.md) are mounted.",
    )
    workspace_path: str = Field(
        default="/workspace",
        alias="workspacePath",
        description="Container path of the code workspace.",
    )

    # Session management.
    session_id: str | None = Field(
        default=None,
        alias="sessionId",
        description=(
            "Engine session ID.  When present, the agent resumes "
            "the prior conversation instead of starting fresh."
        ),
    )
    session_store_path: str | None = Field(
        default=None,
        alias="sessionStorePath",
        description=(
            "Per-conversation directory the engine should point its session store "
            "at (e.g. '/workspace/<ws>/conversations/<id>/session'). "
            "Empty → the runtime keeps the SDK's default location."
        ),
    )

    # Remote MCP servers.
    mcp_servers: list[McpServerConfig] = Field(
        default_factory=list,
        alias="mcpServers",
        description="Remote MCP servers to attach for this turn.",
    )

    # Provider credentials supplied per-(workspace, runtime) from the UI. When
    # empty, the engine falls back to its own env-configured key/base URL.
    api_key: str = Field(
        default="",
        alias="apiKey",
        description="ANTHROPIC_API_KEY override. Empty → fall back to the engine env key.",
    )
    base_url: str = Field(
        default="",
        alias="baseUrl",
        description="ANTHROPIC_BASE_URL override. Empty → fall back to the engine env base URL.",
    )

    # Generation knobs resolved per (workspace, role) by the orchestrator.
    # None/empty → fall back to engine Settings defaults.
    max_tokens: int | None = Field(
        default=None,
        alias="maxTokens",
        description="Max output tokens. None → Settings.max_tokens.",
    )
    max_turns: int | None = Field(
        default=None,
        alias="maxTurns",
        description="Max agent loop iterations. None → Settings.max_turns.",
    )
    effort: str | None = Field(
        default=None,
        description="Effort level: low|medium|high|xhigh|max. None → SDK default.",
    )
    thinking: ThinkingConfig | None = Field(
        default=None,
        description="Extended thinking config. None → thinking off.",
    )
    fallback_model: str | None = Field(
        default=None,
        alias="fallbackModel",
        description="Model to fall back to if the primary is unavailable.",
    )
    betas: list[str] = Field(
        default_factory=list,
        description="Claude Agent SDK beta flags (e.g. the 1M-context flag).",
    )

    @model_validator(mode="after")
    def at_least_one_message(self) -> Job:
        """All messages must not be whitespace-only with no attachments."""
        valid = [m for m in self.messages if m.content.strip() or m.attachments]
        if not valid:
            raise ValueError("All messages have empty content and no attachments.")
        self.messages = valid
        return self

    def resolved_model(self, default: str) -> str:
        """Return the job model, falling back to *default* when blank."""
        return self.model or default
