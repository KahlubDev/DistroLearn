# Web image: Next.js 16 in standalone mode.
#
# Build from the repository root. This is a pnpm workspace, so pnpm install needs the
# root pnpm-lock.yaml and the sibling package manifests. An apps/web-only context cannot
# satisfy --frozen-lockfile.
#   docker build -f apps/web/Dockerfile -t distrolearn-web:sha .

# Pinned by digest. Verified 2026-10-09 against the registry index. The image config sets
# no USER, so this base runs as root and the runtime stage must set USER node itself.
ARG NODE_IMAGE=node:24.21.0@sha256:3d27e5c11e5786e309ec3e03f93ae536eb36e6e5eb3714d5eb3300a36157add0

FROM ${NODE_IMAGE} AS deps
WORKDIR /app
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable

# Manifests first so a source-only change reuses the cached install layer.
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY apps/web/package.json apps/web/package.json
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --filter @distrolearn/web...

FROM ${NODE_IMAGE} AS build
WORKDIR /app
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
ENV NEXT_TELEMETRY_DISABLED=1
RUN corepack enable

COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/web/node_modules ./apps/web/node_modules
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web ./apps/web

RUN pnpm --filter @distrolearn/web build

# The standalone server resolves its own dependency tree relative to this directory.
# Static assets and public/ are not folded in by Next.js, so they are copied alongside.
FROM ${NODE_IMAGE} AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# The base image ships a `node` account (uid 1000). Reused rather than created, so the
# uid matches whatever the runtime image already expects.
COPY --from=build --chown=node:node /app/apps/web/.next/standalone ./
COPY --from=build --chown=node:node /app/apps/web/.next/static ./apps/web/.next/static
COPY --from=build --chown=node:node /app/apps/web/public ./apps/web/public

USER node
EXPOSE 3000
# Next.js preserves the monorepo layout inside standalone, so the entrypoint sits at
# apps/web/server.js rather than at the root. Verified against a real build on
# 2026-10-09: the only server.js is .next/standalone/apps/web/server.js.
CMD ["node", "apps/web/server.js"]