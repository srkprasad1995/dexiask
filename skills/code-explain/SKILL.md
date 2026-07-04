# Skill: code-explain

**Roles**: ask

Helps the agent give clear, accurate explanations of code structure, patterns,
and architecture. Loaded by the Claude engine when the `ask` role is active
(mounted at `/skills/code-explain/SKILL.md`).

## Guidance

- Prefer concrete examples over abstract descriptions.
- Cite the specific file path and line/function when referring to code.
- Explain the *why* (design intent) not just the *what* (mechanics).
- For architecture questions, trace the full request path from the entry point.
- Keep explanations concise; offer to go deeper if needed.
- Never suggest edits to files — this role is read-only.
