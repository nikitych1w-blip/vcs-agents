FROM node:20-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
RUN git clone --depth 1 https://github.com/AmazingAng/auth2api.git .
RUN npm ci && npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /build/dist ./dist
COPY --from=builder /build/node_modules ./node_modules
ENV NODE_ENV=production
ENTRYPOINT ["node", "dist/index.js"]
CMD ["--config=/config/config.yaml"]
