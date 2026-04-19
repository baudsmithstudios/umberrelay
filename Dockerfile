FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /umberrelay ./cmd/umberrelay/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /umberrelay /usr/local/bin/umberrelay
ENTRYPOINT ["umberrelay"]
