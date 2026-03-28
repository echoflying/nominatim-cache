FROM node:20-alpine AS builder

WORKDIR /app/server

COPY server/package*.json ./
RUN npm ci

COPY server/ ./
RUN npm run build

FROM node:20-alpine AS runtime

WORKDIR /app

RUN apk add --no-cache curl

COPY server/package*.json ./server/
WORKDIR /app/server
RUN npm ci --omit=dev

COPY --from=builder /app/server/dist ./dist
COPY --from=builder /app/server/ecosystem.config.js ./ecosystem.config.js
COPY --from=builder /app/server/src/scripts ./src/scripts
COPY --from=builder /app/server/logs ./logs
COPY frontend /app/frontend

EXPOSE 3000

CMD ["node", "dist/index.js"]
