# Skills

Each subdirectory here is a **skill pack**: a folder containing a `SKILL.md` file.
On startup the engine globs `/skills/*/SKILL.md` and appends every pack to the
agent's system prompt. This is how you extend or steer Dexiask's behavior without
touching code.

Bundled packs (all read-only-friendly, for ask mode):

- `code-explain/` — how to explain code structure and architecture clearly.
- `helpful/` — bias toward genuinely useful, follow-through answers.
- `voice/` — tone and writing style.

**Add your own:** create `skills/my-skill/SKILL.md` and restart the engine. Start
the file with `# Skill: my-skill` and write plain Markdown guidance.
