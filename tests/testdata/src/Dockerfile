# syntax=docker/dockerfile:1
# Dockerfile: each instruction is a node. Searching the image returns the
# FROM block.
FROM registry.example.com/billing:1.2.3 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/billing ./cmd/billing

FROM gcr.io/distroless/static
COPY --from=builder /out/billing /billing
ENTRYPOINT ["/billing"]
