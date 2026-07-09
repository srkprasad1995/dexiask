# Ollama sidecar for local (no-API-key) mode.
#
# Models are pulled at BUILD time and live in plain image layers, so nothing is
# downloaded after the stack is up. The engine's local runtime drives the
# native /api/chat with tool calling; pin the base version for reproducibility.
ARG OLLAMA_VERSION=0.31.2
FROM ollama/ollama:${OLLAMA_VERSION}

# A small NON-reasoning model with tool support: reasoning models (qwen3) burn
# the whole turn "thinking" on CPU, and the Anthropic-compat API can't turn
# that off.
ARG TEXT_MODEL=qwen2.5:1.5b
ARG EMBED_MODEL=qwen3-embedding:0.6b
# Tuning baked into the TEXT model only (via a derived Modelfile) — not global
# env — so the tiny embedding model keeps its small default context instead of
# reserving a huge KV cache.
#  - num_ctx: Ollama's 4096 default would truncate the agent's system prompt;
#    16K fits it while keeping the KV cache modest on small machines.
#  - num_thread: llama.cpp defaults to every core, but on laptops (and Docker
#    VMs exposing efficiency cores) thread contention makes generation 10-300x
#    SLOWER than a few pinned threads. 4 is a safe fast default everywhere;
#    raise it on real many-core servers.
ARG TEXT_CONTEXT=16384
ARG TEXT_THREADS=4

# The base image declares VOLUME /root/.ollama — anything pulled there during
# build is shadowed by an anonymous volume at runtime. Store models in /models
# instead so the build-time pulls survive into the running container.
ENV OLLAMA_MODELS=/models

# Pull each model in its own layer (embed ~640 MB, text ~1 GB) so a text-model
# change doesn't invalidate the embed layer in the build cache. `ollama pull`
# needs a running server; start one for the duration of the RUN step.
RUN ollama serve & \
    until ollama list >/dev/null 2>&1; do sleep 1; done; \
    ollama pull "${EMBED_MODEL}"
# Pull the text model, then re-tag it with the tuning baked in so the Anthropic
# API (which sends neither num_ctx nor num_thread) gets them without global env.
RUN ollama serve & \
    until ollama list >/dev/null 2>&1; do sleep 1; done; \
    ollama pull "${TEXT_MODEL}"; \
    printf 'FROM %s\nPARAMETER num_ctx %s\nPARAMETER num_thread %s\n' \
        "${TEXT_MODEL}" "${TEXT_CONTEXT}" "${TEXT_THREADS}" > /tmp/Modelfile; \
    ollama create "${TEXT_MODEL}" -f /tmp/Modelfile; \
    rm -f /tmp/Modelfile

# Pre-load the text model into RAM on startup (background, keep-alive forever) so
# the first chat turn doesn't pay the cold model-load latency inside the engine's
# no-output watchdog. The entrypoint keeps Ollama's own startup behaviour.
ENV TEXT_MODEL=${TEXT_MODEL}
COPY entrypoint.sh /usr/local/bin/dexiask-entrypoint.sh
RUN chmod +x /usr/local/bin/dexiask-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/dexiask-entrypoint.sh"]
CMD ["serve"]
