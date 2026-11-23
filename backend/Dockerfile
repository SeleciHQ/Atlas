# syntax=docker/dockerfile:1
# One image definition for every Go process. Choose the command with:
#   docker build --build-arg SERVICE=atlas-api -t atlas-api .
#   SERVICE: atlas-api | atlas-mcp | atlas-worker | atlas-scheduler |
#            atlas-migrate | atlas-provision
ARG GO_VERSION=1.27
FROM golang:${GO_VERSION}-alpine AS build
ARG SERVICE=atlas-api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/service /atlas/service
COPY migrations /app/migrations
COPY scripts /app/scripts
ENV MIGRATIONS_DIR=/app/migrations
USER nonroot:nonroot
ENTRYPOINT ["/atlas/service"]
