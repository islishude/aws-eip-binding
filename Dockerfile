# syntax=docker/dockerfile:1
FROM golang:1.26.2 AS compiler
WORKDIR /app
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go install

FROM gcr.io/distroless/base-debian13:latest
COPY --from=compiler /go/bin/aws-eip-binding /usr/local/bin/
ENTRYPOINT [ "aws-eip-binding" ]
