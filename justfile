#!/usr/bin/env just --justfile

set dotenv-load := true

export CLICOLOR_FORCE := "1"

default:
  just --list

update:
  cd web && bun install

build-server:
  go build -buildvcs=false -o ./.cache/server.exe ./server/cmd/deepdoc/.

build-web:
  cd web && bun run build
  rm -r ./server/api/webbuild
  cp -r ./web/build ./server/api/webbuild
  touch ./server/api/webbuild/.gitkeep

dev:
  ./web/node_modules/.bin/concurrently \
    --names "API,WEB" \
    --prefix-colors "bgBlue.bold,bgMagenta.bold" \
    '{{just_executable()}} --justfile {{justfile()}} dev-server' \
    '{{just_executable()}} --justfile {{justfile()}} dev-web'

dataservice:
    cd server/cmd/dataservice && nohup go run *.go > dataservice.log 2>&1 &

dev-server:
  go tool air \
      --build.cmd '{{just_executable()}} --justfile {{justfile()}} build-server' \
      --build.bin './.cache/server.exe' \
      --build.exclude_dir 'web,.cache' \
      -tmp_dir './.cache'

dev-web:
  cd web && bun dev

debug-server:
  cd server/cmd/deepdoc && dlv debug

publish:
  {{just_executable()}} --justfile {{justfile()}} build-web
  KO_DOCKER_REPO=chendingplano/deepdoc go tool ko build ./server/cmd/deepdoc --platform=linux/amd64 --bare --sbom=none
