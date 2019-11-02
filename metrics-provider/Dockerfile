# Use base golang image from Docker Hub
FROM golang:1.13.3 as build

WORKDIR /src/github.com/balchua/artemis-ext-scaler

# Copy go.mod and go.sum 
ADD ./go.mod /src/github.com/balchua/artemis-ext-scaler/
ADD ./go.sum /src/github.com/balchua/artemis-ext-scaler/
# Install dependencies in go.mod and go.sum
RUN go mod download

# Copy application source code
COPY ./server.go /src/github.com/balchua/artemis-ext-scaler/
COPY ./externalscaler/ /src/github.com/balchua/artemis-ext-scaler/externalscaler

# Compile the application to /app.
RUN go build -o /app -v .

# Now create separate deployment image
FROM gcr.io/distroless/base
COPY --from=build /app /app
# Cause full tracebacks; also serves to identify this image as a Go image for `skaffold debug`
ENV GOTRACEBACK=all
ENTRYPOINT ["/app"]
