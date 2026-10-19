#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"action":"cutover","domain":"creator.example","name":"media","target":"edge.new.example","previous_target":"edge.old.example","ttl":60,"digital_assets_ready":true,"subscriber_updates_ready":true,"content_processing_ready":true}' \
  http://localhost:8080/cutover
