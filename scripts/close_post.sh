#!/usr/bin/env bash
set -o xtrace
curl https://$1.zendesk.com/api/v2/community/posts/$4/comments.json \
  -d '{"comment": {"body": "<p>This discussion moved to <a href=\"https://feedback.pleexy.com\" target=\"_blank\" rel=\"nofollow noreferrer\">https://feedback.pleexy.com</a></p>", "official": true}, "notify_subscribers": false}' \
  -u "$2:$3" --basic -H "Content-Type: application/json"

curl https://$1.zendesk.com/api/v2/community/posts/$4.json \
  -d '{"post": {"closed":true}}' \
  -u $2:$3 -X PUT -H "Content-Type: application/json"
