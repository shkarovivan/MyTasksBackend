FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /mytasks-backend .

FROM alpine:3.20
COPY --from=build /mytasks-backend /mytasks-backend
EXPOSE 8080
ENTRYPOINT ["/mytasks-backend"]
