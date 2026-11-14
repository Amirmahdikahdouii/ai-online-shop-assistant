FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /service ./cmd/service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
