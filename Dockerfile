# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o kflashback ./cmd/kflashback/

# UI build stage
FROM node:20-alpine AS ui-builder

WORKDIR /ui

COPY ui/package.json ui/package-lock.json* ui/.npmrc* ./
RUN npm ci

COPY ui/ .
RUN npm run build

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/kflashback /kflashback
COPY --from=ui-builder /ui/dist /ui

USER 65532:65532

ENTRYPOINT ["/kflashback"]
