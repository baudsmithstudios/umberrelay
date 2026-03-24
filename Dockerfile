FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /scrye ./cmd/scrye/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /scrye /usr/local/bin/scrye
ENTRYPOINT ["scrye"]
