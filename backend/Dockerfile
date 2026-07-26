# Go 服务镜像,SERVICE 指定构建哪个入口(当前仅 workflow):
#   docker build --build-arg SERVICE=workflow -t phoenix-workflow backend
FROM golang:1.26 AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app-bin ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /app-bin /app/service
COPY configs /app/configs
ENTRYPOINT ["/app/service"]
