"""
Workspace file-system tools: Read, Glob, Grep (read-only).

``WorkspaceTools`` is the path-jail logic and pure-Python handlers, with no
agent-SDK coupling. ``build_workspace_descriptors`` wraps those handlers as
runtime-neutral ``ToolDescriptor``s; the Claude runtime adapts them into an
in-process SDK MCP server.

All handlers are path-jailed to the workspace root and are strictly read-only —
there are no write or exec operations (this is the ask deployment). Absolute
paths from the model are treated as relative to the workspace root (so /main.go →
/workspace/main.go).
"""
from __future__ import annotations

import asyncio
import fnmatch
import re
from pathlib import Path
from typing import Any

from .. import observability as obs
from ..models.settings import BaseEngineSettings, default_settings
from ..tools.descriptors import ToolDescriptor

# Directory names to prune from recursive traversal.
_SKIP_DIRS = frozenset({
    ".git", "node_modules", "__pycache__", ".next", ".nuxt",
    "dist", "build", ".build", "vendor", "venv", ".venv",
    ".mypy_cache", ".pytest_cache", ".ruff_cache",
    "target",  # Rust/Maven
})

# Valid workspace tool names — read-only only.
WORKSPACE_TOOL_NAMES = ("Read", "Glob", "Grep")


class WorkspaceTools:
    """
    Read-only file-system tools scoped to a single workspace directory.

    Instantiate with the host path that is mounted at /workspace in the
    container (or /workspace itself when running inside the container).
    Limits (max_read_lines, max_glob_matches, max_grep_matches) come from
    settings rather than module-level constants.
    """

    def __init__(
        self,
        workspace_path: str = "/workspace",
        settings: BaseEngineSettings | None = None,
    ) -> None:
        self._root = Path(workspace_path).resolve()
        self._settings = settings or default_settings()

    # ------------------------------------------------------------------
    # Path safety
    # ------------------------------------------------------------------

    def _safe_path(self, rel_or_abs: str) -> Path | None:
        """
        Resolve a model-supplied path relative to the workspace root.

        Absolute paths (e.g. /src/main.go) are re-rooted to the workspace so
        the model can use them naturally.  Returns ``None`` on traversal attempt.
        """
        raw = Path(rel_or_abs)
        if raw.is_absolute():
            # An absolute path already inside the workspace (e.g. an attachment
            # path-ref "/workspace/<ws>/attachments/...") is used as-is. Otherwise
            # the model is treating the workspace as the filesystem root (e.g.
            # "/src/main.go"), so re-root it by dropping the leading slash.
            resolved = raw.resolve()
            try:
                resolved.relative_to(self._root)
                candidate = resolved
            except ValueError:
                candidate = (self._root / Path(*raw.parts[1:])).resolve()
        else:
            candidate = (self._root / raw).resolve()
        try:
            candidate.relative_to(self._root)
            return candidate
        except ValueError:
            return None

    # ------------------------------------------------------------------
    # Internal walk helper
    # ------------------------------------------------------------------

    def _walk_workspace(self, root: Path):
        """Yield all files under *root*, pruning _SKIP_DIRS."""
        import os

        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]
            base = Path(dirpath)
            for fname in filenames:
                yield base / fname

    # ------------------------------------------------------------------
    # Handlers (pure-Python, returning str)
    # ------------------------------------------------------------------

    def read(self, input: dict[str, Any]) -> str:
        path_str = input.get("path", "")
        resolved = self._safe_path(path_str)
        if resolved is None:
            return f"Error: path '{path_str}' is outside /workspace. Access denied."
        if not resolved.exists():
            return f"Error: '{path_str}' does not exist."
        if resolved.is_dir():
            return f"Error: '{path_str}' is a directory, not a file."

        offset = max(1, int(input.get("offset") or 1))
        limit = int(input.get("limit") or self._settings.max_read_lines)

        try:
            lines = resolved.read_text(encoding="utf-8", errors="replace").splitlines()
        except OSError as e:
            return f"Error reading '{path_str}': {e}"

        selected = lines[offset - 1 : offset - 1 + limit]
        numbered = "\n".join(
            f"{offset + i:6d}\t{line}" for i, line in enumerate(selected)
        )
        return f"Here's the content of {path_str} with line numbers:\n{numbered}"

    def glob(self, input: dict[str, Any]) -> str:
        pattern = input.get("pattern", "")

        paths: list[str] = []
        truncated = False
        try:
            for fpath in self._walk_workspace(self._root):
                try:
                    rel = fpath.relative_to(self._root)
                except ValueError:
                    continue
                if fnmatch.fnmatch(str(rel), pattern) or fnmatch.fnmatch(fpath.name, pattern):
                    paths.append(str(rel))
                    if len(paths) >= self._settings.max_glob_matches:
                        truncated = True
                        break
        except Exception as e:
            return f"Error: invalid glob pattern '{pattern}': {e}"

        if not paths:
            return f"No files matched pattern '{pattern}'"
        result = "\n".join(sorted(paths))
        if truncated:
            result += f"\n... (truncated at {self._settings.max_glob_matches} matches)"
        return result

    def grep(self, input: dict[str, Any]) -> str:
        pattern_str = input.get("pattern", "")
        path_str = input.get("path", "/workspace")
        glob_filter = input.get("glob", "")

        try:
            regex = re.compile(pattern_str)
        except re.error as e:
            return f"Error: invalid regex pattern '{pattern_str}': {e}"

        if not path_str or path_str in ("/workspace", "."):
            search_root = self._root
        else:
            search_root = self._safe_path(path_str)
            if search_root is None:
                return f"Error: path '{path_str}' is outside /workspace. Access denied."

        if not search_root.exists():
            return f"Error: path '{path_str}' does not exist."

        if search_root.is_file():
            file_candidates = [search_root]
        else:
            file_candidates = [
                p
                for p in self._walk_workspace(search_root)
                if p.is_file()
                and (not glob_filter or fnmatch.fnmatch(p.name, glob_filter))
            ]

        results: list[str] = []
        for fpath in sorted(file_candidates):
            try:
                text = fpath.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            for lineno, line in enumerate(text.splitlines(), 1):
                if regex.search(line):
                    try:
                        rel = fpath.relative_to(self._root)
                    except ValueError:
                        rel = fpath
                    results.append(f"{rel}:{lineno}: {line}")
            if len(results) >= self._settings.max_grep_matches:
                results.append(f"... (truncated at {self._settings.max_grep_matches} matches)")
                break

        if not results:
            return f"No matches found for pattern '{pattern_str}'"
        return "\n".join(results)

    def get_handlers(self) -> dict[str, Any]:
        """Return ``{tool_name: handler}`` for all workspace tools."""
        return {
            "Read": self.read,
            "Glob": self.glob,
            "Grep": self.grep,
        }


# ------------------------------------------------------------------
# Descriptor schemas + descriptions
# ------------------------------------------------------------------

_READ_DESC = (
    "Read the contents of a file in the workspace. "
    "Returns file content with 1-indexed line numbers. "
    "Optional offset (default 1) and limit (default 2000 lines)."
)
_READ_SCHEMA = {
    "type": "object",
    "properties": {
        "path": {"type": "string"},
        "offset": {"type": "integer"},
        "limit": {"type": "integer"},
    },
    "required": ["path"],
}

_GLOB_DESC = (
    "Find files in the workspace matching a glob pattern "
    "(e.g. '**/*.go', 'src/**/*.ts'). "
    "Returns a newline-delimited list of paths relative to /workspace."
)
_GLOB_SCHEMA = {
    "type": "object",
    "properties": {"pattern": {"type": "string"}},
    "required": ["pattern"],
}

_GREP_DESC = (
    "Search for a regex pattern in workspace files. "
    "Returns matching lines as 'file:line: content'. "
    "Optional path restricts search to a file or directory. "
    "Optional glob filters by filename pattern (e.g. '*.go')."
)
_GREP_SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string"},
        "path": {"type": "string"},
        "glob": {"type": "string"},
    },
    "required": ["pattern"],
}

_SPECS = {
    "Read": (_READ_DESC, _READ_SCHEMA, "read", "file.path", "path"),
    "Glob": (_GLOB_DESC, _GLOB_SCHEMA, "glob", "glob.pattern", "pattern"),
    "Grep": (_GREP_DESC, _GREP_SCHEMA, "grep", "grep.pattern", "pattern"),
}


def build_workspace_descriptors(
    workspace_path: str,
    allowed_tools: list[str],
    settings: BaseEngineSettings | None = None,
) -> list[ToolDescriptor]:
    """Build runtime-neutral descriptors for the workspace tools named in
    *allowed_tools* (valid values: Read, Glob, Grep)."""
    ws = WorkspaceTools(workspace_path, settings)
    descriptors: list[ToolDescriptor] = []

    for name in WORKSPACE_TOOL_NAMES:
        if name not in allowed_tools:
            continue
        desc, schema, method, span_attr, arg_key = _SPECS[name]
        handler = getattr(ws, method)

        def make(handler=handler, span_name=name, span_attr=span_attr, arg_key=arg_key):
            async def _run(args: dict[str, Any]) -> dict[str, Any]:
                with obs.tool_span(span_name, **{span_attr: args.get(arg_key)}) as span:
                    # Run the (blocking) handler off the event loop so a large
                    # grep doesn't stall the engine serving other jobs.
                    text = await asyncio.to_thread(handler, args)
                    span.set_attribute("result.size", len(text))
                    return ToolDescriptor.text_result(text)
            return _run

        descriptors.append(
            ToolDescriptor(name=name, description=desc, input_schema=schema, handler=make())
        )
    return descriptors
