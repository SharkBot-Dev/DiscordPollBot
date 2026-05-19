FROM alpine:latest

WORKDIR /app
COPY . /app

RUN apk --no-cache add tzdata \
    && cp /usr/share/zoneinfo/Asia/Tokyo /etc/localtime \
    && echo "Asia/Tokyo" > /etc/timezone

CMD ["./DiscordPollBot"]