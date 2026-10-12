FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /diary ./cmd/server

FROM alpine:3.22
RUN addgroup -S diary && adduser -S -G diary diary && mkdir /data && chown diary:diary /data
COPY --from=build /diary /usr/local/bin/diary
USER diary
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["diary"]
CMD ["-addr", "0.0.0.0:8080", "-data", "/data/trips.json"]
