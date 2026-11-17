# Both build stages run on the build host and cross-compile, so a multi-arch
# build doesn't run npm and Go under emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web/ui
COPY web/ui/package.json web/ui/package-lock.json ./
RUN npm ci
COPY web/ui/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /homepaper ./cmd/homepaper

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /homepaper /homepaper
ENV HOMEPAPER_DATA=/data HOMEPAPER_LISTEN=:8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/homepaper"]
