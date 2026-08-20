FROM node:22-alpine AS build

WORKDIR /app
ENV ASTRO_TELEMETRY_DISABLED=1

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# PUBLIC_API_URL se incrusta en el bundle del navegador durante el build.
ARG PUBLIC_API_URL=http://localhost:8080
ENV PUBLIC_API_URL=$PUBLIC_API_URL
RUN npm run build

FROM node:22-alpine

WORKDIR /app
ENV ASTRO_TELEMETRY_DISABLED=1
ENV HOST=0.0.0.0
ENV PORT=4321

COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY --from=build /app/package.json ./package.json

EXPOSE 4321
# Servidor SSR del adaptador de Node: renderiza el catálogo y el detalle en cada
# petición con los datos actuales del backend.
CMD ["node", "./dist/server/entry.mjs"]
