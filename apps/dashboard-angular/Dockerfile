# syntax=docker/dockerfile:1.7

# --- build stage ----------------------------------------------------------
FROM node:20-alpine AS build
WORKDIR /src

COPY package.json ./
RUN npm install --no-audit --no-fund

COPY angular.json tsconfig.json tsconfig.app.json ./
COPY src ./src
RUN npx ng build --configuration production

# --- runtime stage --------------------------------------------------------
FROM nginx:1.27-alpine
COPY --from=build /src/dist/dashboard-angular/browser /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
HEALTHCHECK CMD wget -q -O- http://localhost:8080/healthz || exit 1
