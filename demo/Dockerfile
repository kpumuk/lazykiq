FROM ruby:4.0.9-alpine@sha256:8ad6909db16ad53b5dc90bb8cc157e5fdd89ba1172798316b95cfd2a61f14b1e

ARG BUNDLE_DIR=.

RUN apk add --no-cache build-base

WORKDIR /app

ENV BUNDLE_GEMFILE=/app/${BUNDLE_DIR}/Gemfile

COPY "${BUNDLE_DIR}/Gemfile" "${BUNDLE_DIR}/Gemfile.lock" /app/${BUNDLE_DIR}/
RUN bundle install

COPY . .

CMD ["ruby", "start.rb"]
