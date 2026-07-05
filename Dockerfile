FROM golang:1.25.0-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE=gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /out/app /app

EXPOSE 50051 8080

USER nonroot:nonroot

ENTRYPOINT ["/app"]