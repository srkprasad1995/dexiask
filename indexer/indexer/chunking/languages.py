"""Extension → language mapping and per-language symbol node types."""
from __future__ import annotations

# Extension (lowercase, no dot) → canonical language name.
EXT_LANG: dict[str, str] = {
    "py": "python",
    "go": "go",
    "ts": "typescript",
    "tsx": "tsx",
    "js": "javascript",
    "jsx": "javascript",
    "java": "java",
    "rs": "rust",
    "rb": "ruby",
    "php": "php",
    "c": "c",
    "h": "c",
    "cc": "cpp",
    "cpp": "cpp",
    "hpp": "cpp",
    "cs": "c_sharp",
    "kt": "kotlin",
    "scala": "scala",
    "swift": "swift",
    "md": "markdown",
}

# Tree-sitter node types that represent a top-level "symbol" worth a chunk.
SYMBOL_NODE_TYPES: dict[str, set[str]] = {
    "python": {"function_definition", "class_definition", "decorated_definition"},
    "go": {"function_declaration", "method_declaration", "type_declaration"},
    "typescript": {"function_declaration", "class_declaration", "method_definition", "interface_declaration"},
    "tsx": {"function_declaration", "class_declaration", "method_definition", "interface_declaration"},
    "javascript": {"function_declaration", "class_declaration", "method_definition"},
    "java": {"method_declaration", "class_declaration", "interface_declaration"},
    "rust": {"function_item", "impl_item", "struct_item", "trait_item", "enum_item"},
    "ruby": {"method", "class", "module"},
    "c": {"function_definition", "struct_specifier"},
    "cpp": {"function_definition", "class_specifier", "struct_specifier"},
    "c_sharp": {"method_declaration", "class_declaration", "interface_declaration"},
}


def language_for_path(path: str) -> str | None:
    ext = path.rsplit(".", 1)[-1].lower() if "." in path else ""
    return EXT_LANG.get(ext)
