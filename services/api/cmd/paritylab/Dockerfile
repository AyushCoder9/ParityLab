FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/paritylab ./services/api/cmd/paritylab

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/paritylab /usr/local/bin/paritylab
COPY --from=build /src/db/migrations /app/db/migrations
WORKDIR /app
ENV API_ADDRESS=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/paritylab"]
