# dexiask-web — production image for the Next.js BFF.
#
# Multi-stage: Node 22 (alpine) + pnpm via corepack. Builds with `pnpm build`
# using Next.js "standalone" output and serves the self-contained server on
# :3000 as a non-root user.
#
#   docker build -t dexiask-web:dev .
#   docker run -p 3000:3000 -e GO_BACKEND_URL=http://host.docker.internal:8080 dexiask-web:dev

# syntax=docker/dockerfile:1

# ---- Base: Node 22 + pnpm via corepack ----
FROM node:22-alpine AS base
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
ENV NEXT_TELEMETRY_DISABLED=1
RUN corepack enable
WORKDIR /app

# ---- Dependencies ----
FROM base AS deps
# Disable pnpm's minimum-release-age supply-chain gate for the build: we install
# from a committed, in-sync lockfile, and some CI/registry policies would
# otherwise reject legitimately-recent package versions. Vanilla pnpm does not
# gate by default, so this is a no-op outside such environments.
ENV npm_config_minimum_release_age=0
COPY package.json pnpm-workspace.yaml .npmrc ./
COPY pnpm-lock.yaml* ./
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile

# ---- Build (produces .next/standalone) ----
FROM base AS build
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

# ---- Runtime (self-contained standalone server) ----
FROM node:22-alpine AS runtime
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
WORKDIR /app

# The standalone output bundles a minimal node_modules + server.js.
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public

USER node
EXPOSE 3000

# LB / k8s liveness+readiness probe target: GET /api/healthz -> 200 {"status":"ok"}
CMD ["node", "server.js"]
