FROM ruby:3.4-alpine

ARG BUNDLE_GEMFILE=Gemfile

RUN apk add --no-cache build-base

WORKDIR /app

ENV BUNDLE_GEMFILE=${BUNDLE_GEMFILE}

COPY "${BUNDLE_GEMFILE}" "${BUNDLE_GEMFILE}.lock" ./
RUN bundle install

COPY . .

CMD ["ruby", "start.rb"]
