# The runtime as one image: Node, the source, and its dependencies. No build
# step — node runs the TypeScript directly, which is one fewer thing between
# what is reviewed and what runs.
#
#   docker build -t cuckoo-runtime:$(git describe --always) runtime/
#
# The SDK is a file: dependency on ../sdk/typescript, so this image is built
# from the repository root with runtime/Dockerfile as the file:
#
#   docker build -f runtime/Dockerfile -t cuckoo-runtime:TAG .

FROM node:22-alpine AS deps
WORKDIR /app

# The SDK first: it is a local dependency, and npm needs it present to link.
COPY sdk/typescript/package.json sdk/typescript/tsconfig.json ./sdk/typescript/
COPY sdk/typescript/src ./sdk/typescript/src
RUN cd sdk/typescript && npm install --omit=dev --no-audit --no-fund --ignore-scripts \
    && npm install --no-audit --no-fund typescript@5.7.2 && npx tsc -p tsconfig.json

COPY runtime/package.json ./runtime/
RUN cd runtime && npm install --omit=dev --no-audit --no-fund --ignore-scripts

FROM node:22-alpine
WORKDIR /app/runtime

RUN apk add --no-cache tzdata \
    && addgroup -g 10001 cuckoo && adduser -D -u 10001 -G cuckoo cuckoo

COPY --from=deps /app/sdk/typescript /app/sdk/typescript
COPY --from=deps /app/runtime/node_modules ./node_modules
COPY runtime/package.json ./
COPY runtime/src ./src
COPY runtime/scripts ./scripts

ARG VERSION=dev
ENV RUNTIME_VERSION=${VERSION}
ENV NODE_ENV=production

USER cuckoo
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8081/healthz >/dev/null || exit 1
ENTRYPOINT ["node", "--experimental-strip-types", "src/main.ts"]
