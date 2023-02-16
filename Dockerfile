# Build the manager binary
FROM public.ecr.aws/docker/library/golang:1.19 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

ENV GOPROXY direct

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY webhooks/ webhooks/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o zone-aware-controllers main.go

FROM scratch
WORKDIR /
COPY --from=builder /workspace/zone-aware-controllers .
USER 65532:65532

ENTRYPOINT ["/zone-aware-controllers"]
