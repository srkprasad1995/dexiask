#!/bin/sh
# Start Ollama, then warm the text model into RAM in the background so the first
# chat turn skips the cold model-load (which otherwise exceeds the engine's
# no-output watchdog on slower hosts). Warming is best-effort — the server is
# usable for embeddings and other models regardless.
set -e

if [ -n "${TEXT_MODEL}" ]; then
    (
        until ollama list >/dev/null 2>&1; do sleep 1; done
        echo "[dexiask] pre-loading text model ${TEXT_MODEL} into RAM..."
        # A one-shot prompt triggers the load; the server keeps it resident per
        # OLLAMA_KEEP_ALIVE (=-1 in compose → forever).
        ollama run "${TEXT_MODEL}" "hi" >/dev/null 2>&1 \
            && echo "[dexiask] text model ${TEXT_MODEL} resident" \
            || echo "[dexiask] warm-up of ${TEXT_MODEL} failed (non-fatal)"
    ) &
fi

exec /bin/ollama "$@"
