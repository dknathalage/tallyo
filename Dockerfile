FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create non-root user
RUN addgroup -g 1001 -S nodejs && adduser -S nodejs -u 1001

# Create data directory
RUN mkdir -p /data && chown nodejs:nodejs /data

COPY --from=builder --chown=nodejs:nodejs /app/build ./build
COPY --from=builder --chown=nodejs:nodejs /app/package*.json ./
RUN npm ci --omit=dev

USER nodejs

ENV PORT=3000
ENV HOST=0.0.0.0
ENV NODE_ENV=production
ENV DB_PATH=/data/invoices.db

EXPOSE 3000
VOLUME ["/data"]

ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "build/index.js"]
