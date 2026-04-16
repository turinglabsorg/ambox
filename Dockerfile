FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ambox ./cmd/ambox

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /ambox /ambox
COPY web/ /web/
EXPOSE 8080
ENTRYPOINT ["/ambox"]
