FROM ruby:4.0.2-alpine@sha256:e973e9aa7573233432565798734a891d648b8f9f6ae20d57db7338b63c35db02

ARG BUNDLE_DIR=.

RUN apk add --no-cache build-base

WORKDIR /app

ENV BUNDLE_GEMFILE=/app/${BUNDLE_DIR}/Gemfile

COPY "${BUNDLE_DIR}/Gemfile" "${BUNDLE_DIR}/Gemfile.lock" /app/${BUNDLE_DIR}/
RUN bundle install

COPY . .

CMD ["ruby", "start.rb"]
