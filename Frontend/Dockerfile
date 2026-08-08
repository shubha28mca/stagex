# --- Build stage -------------------------------------------------------------
# Builds the static SPA. VITE_API_BASE_URL is baked in at build time — this is
# the single place the backend location is set for a container deployment
# (docker-compose passes it as a build arg). To re-target the API, change this
# one value; nothing else in the app hard-codes a URL.
FROM node:22-alpine AS build
WORKDIR /app

ARG VITE_API_BASE_URL=http://localhost:8080
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

# --- Runtime stage -----------------------------------------------------------
# Serves the built assets with nginx and an SPA fallback for client routing.
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
