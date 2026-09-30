# syntax=docker/dockerfile:1

# 1. Build the frontend
FROM node:22-alpine AS web
WORKDIR /app
COPY app/package.json app/package-lock.json ./
RUN npm ci
COPY app/ ./
RUN npm run build

# 2. Build the Go server
FROM golang:1.24-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY parse/ ./parse/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /go-diagram .

# 3. Minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /go-diagram /go-diagram
COPY --from=web /app/dist /web
EXPOSE 8080
# Mount the Go project to diagram at /project, e.g.:
#   docker run --rm -p 8080:8080 -v "$PWD":/project go-diagram
ENTRYPOINT ["/go-diagram", "-addr", ":8080", "-static", "/web", "-no-browser"]
CMD ["/project"]
