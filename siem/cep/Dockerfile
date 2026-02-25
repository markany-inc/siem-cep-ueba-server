FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY SafePC/ .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 go build -o siem .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Seoul
WORKDIR /app
COPY --from=builder /build/siem .
CMD ["./siem", "cep"]
