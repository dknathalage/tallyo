FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
COPY apps/app/package.json apps/app/package.json
RUN npm ci
COPY . .
RUN npx turbo run build --filter=@tallyo/app

FROM node:22-alpine AS runner
WORKDIR /app

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create non-root user
RUN addgroup -g 1001 -S nodejs && adduser -S nodejs -u 1001

COPY --from=builder --chown=nodejs:nodejs /app/apps/app/build ./build
COPY --from=builder --chown=nodejs:nodejs /app/apps/app/drizzle ./drizzle
COPY --from=builder --chown=nodejs:nodejs /app/apps/app/package.json ./
COPY --from=builder --chown=nodejs:nodejs /app/package-lock.json ./
RUN npm ci --omit=dev

USER nodejs

ENV PORT=3000
ENV HOST=0.0.0.0
ENV NODE_ENV=production
ENV DATABASE_URL=postgresql://localhost:5432/tallyo

EXPOSE 3000

ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "build/index.js"]
