# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

ARG BUILD_COMMIT=dev
ARG BUILD_TIME=

# Copy package files
COPY package.json ./

# Install dependencies
RUN npm install

# Copy source
COPY . .

# Inject build info and build
RUN echo "window.BUILD_INFO = { commit: '${BUILD_COMMIT}', time: '${BUILD_TIME}' };" > public/build-info.js || true
RUN npm run build

# Production stage
FROM nginx:1.27-alpine

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx config for SPA routing
RUN echo 'server { \
    listen 80; \
    root /usr/share/nginx/html; \
    index index.html; \
    location / { \
        try_files $uri $uri/ /index.html; \
    } \
}' > /etc/nginx/conf.d/default.conf

EXPOSE 80
