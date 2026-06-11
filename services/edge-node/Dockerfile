# syntax=docker/dockerfile:1.7

# --- build stage ----------------------------------------------------------
FROM node:20-alpine AS build
WORKDIR /src

COPY package.json ./
RUN npm install --no-audit --no-fund

COPY tsconfig.json ./
COPY src ./src
RUN npm run build && npm prune --omit=dev

# --- runtime stage --------------------------------------------------------
FROM node:20-alpine
RUN adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /src/node_modules ./node_modules
COPY --from=build /src/dist ./dist
COPY --from=build /src/package.json ./package.json

USER app
EXPOSE 8080
CMD ["node", "dist/server.js"]
