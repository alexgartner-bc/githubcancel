FROM golang:1.21
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download
COPY main.go /app/
WORKDIR /app/
RUN CGO_ENABLED=0 GOOS=linux go build -o githubcancel .

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=0 /app/githubcancel ./
CMD ["./githubcancel"]  