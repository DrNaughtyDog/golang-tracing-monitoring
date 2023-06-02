# based on https://medium.com/@chemidy/create-the-smallest-and-secured-golang-docker-image-based-on-scratch-4752223b7324
############################
# STEP 1 build executable binary
############################
FROM golang:1.19-alpine AS builder
ARG GO_FILES=.
ARG GO_MAIN=./cmd/main.go
LABEL maintainer="Vincent Welker <welker@atix.de>"
# Install git.
# Git is required for fetching the dependencies.
RUN apk update && apk add --no-cache git
# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache ca-certificates && update-ca-certificates

# Set the Current Working Directory inside the container
WORKDIR /app

# Create appuser.
ENV USER=appuser
ENV UID=10001
# See https://stackoverflow.com/a/55757473/12429735RUN
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

# Fetch dependencies.
COPY ${GO_FILES}/go.mod .
COPY ${GO_FILES}/go.sum .
RUN go mod download
# Using go get.
RUN go get -v all

# get go files
COPY ${GO_FILES} .
# Build the binary.
ARG GOOS=linux
ARG GOARCH=amd64
ARG CGO_ENABLED=0
RUN GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=${CGO_ENABLED} go build -ldflags="-w -s" -o /app/main ${GO_MAIN}

# create tmp folder for use in scratch
RUN mkdir /my_tmp
RUN chown -R ${USER}:${USER} /my_tmp


############################
# STEP 2 build a small image
############################
FROM scratch
LABEL maintainer="Vincent Welker <welker@atix.de>"
ARG PROJECT_NAME=jager-tracing
# import certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Import the user and group files from the builder.
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
# Copy static executable.
COPY --from=builder /app/main /main
# create tmp directory
COPY --from=builder /my_tmp /tmp



# Use an unprivileged user.
USER ${UID}:${UID}


# Run the binary.
ENTRYPOINT ["/main"]
