FROM golang:1.26-alpine

RUN apk add --no-cache git \
    && go install github.com/air-verse/air@latest

WORKDIR /app

EXPOSE 3000

CMD ["air"]
