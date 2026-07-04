# Sample workspace

This directory is bind-mounted at `/workspace` inside the engine (read/write, for
uploaded attachments under `.dexiask/`) and the indexer (read-only, as the source
for its git mirrors).

Point `DEXIASK_WORKSPACE_PATH` in your `.env` at the codebase you actually want the
agent to read and the indexer to index — any local git repository works. This
placeholder just keeps the mount non-empty on a fresh clone.
