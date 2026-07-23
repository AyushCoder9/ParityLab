FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./services/api/cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/worker /usr/local/bin/worker
WORKDIR /app
ENTRYPOINT ["/usr/local/bin/worker"]
