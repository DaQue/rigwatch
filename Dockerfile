FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY .git ./.git

ARG GIT_TAG=v0.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
  -ldflags "-s -w -X 'github.com/allisonhere/rigwatch/internal.Version=${GIT_TAG#v}' -X 'github.com/allisonhere/rigwatch/internal.GitCommit=${GIT_COMMIT}' -X 'github.com/allisonhere/rigwatch/internal.BuildDate=${BUILD_DATE}' -X 'github.com/allisonhere/rigwatch/internal.GitTag=${GIT_TAG}'" \
  -o /rigwatch ./cmd/rigwatch

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /rigwatch /usr/local/bin/rigwatch

VOLUME ["/root/.ssh"]

EXPOSE 8081

ENTRYPOINT ["rigwatch"]
CMD ["--web", "--port", "8081"]
