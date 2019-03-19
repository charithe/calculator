FROM golang:1.12-alpine as base
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOOS=linux
RUN apk --no-cache add --update git libc6-compat 

FROM base as build
ADD . /calculator
WORKDIR /calculator
RUN go test ./...
RUN go build -a -ldflags '-s -w' -o calculator cmd/server/main.go
RUN go build -a -ldflags '-s -w' -o cli cmd/cli/main.go

FROM gcr.io/distroless/static
COPY --from=build /calculator/calculator /calculator
COPY --from=build /calculator/cli /cli
ENTRYPOINT ["/calculator"]
CMD ["--listen_addr=:8080"]
	
