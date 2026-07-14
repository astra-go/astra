#!/usr/bin/env bash
# Initialize RocketMQ topics required by the Astra mq integration tests.
#
# Usage (after `docker compose up -d` in this directory):
#   ./init-topics.sh
#
# Creates:
#   astra-integration-test        NORMAL      pub/sub smoke test
#   astra-integration-test-tx     TRANSACTION transaction commit path
#   astra-integration-test-tx-rb  TRANSACTION transaction rollback path
#
# Note: RocketMQ 5.x proxies (incl. 5.3.1) expose no CreateTopic RPC, so the
# TRANSACTION-typed topics must be created via the broker's mqadmin. The
# `-a +message.type=TRANSACTION` attribute is what makes a transactional
# publish succeed (otherwise the broker rejects it with
# "current message type not match with topic accept message types").
set -euo pipefail

BROKER="${ROCKETMQ_BROKER:-astra-rmq-broker}"
MQADMIN="/home/rocketmq/rocketmq-5.3.1/bin/mqadmin"
CLUSTER="${ROCKETMQ_CLUSTER:-DefaultCluster}"
NAMESRV="${ROCKETMQ_NAMESRV:-namesrv:9876}"

echo "Waiting for broker $BROKER to accept admin commands..."
for _ in $(seq 1 30); do
  if docker exec "$BROKER" sh -c "$MQADMIN clusterList -n $NAMESRV" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

create_topic() {
  local topic="$1"; local attrs="${2:-}"
  echo "Creating topic: $topic ${attrs:+(attrs=$attrs)}"
  # shellcheck disable=SC2086
  docker exec "$BROKER" sh -c "$MQADMIN updateTopic -n $NAMESRV -c $CLUSTER -t $topic -w 8 -r 8 $attrs" 2>&1 | tail -1
}

create_topic "astra-integration-test" ""
create_topic "astra-integration-test-tx" "-a +message.type=TRANSACTION"
create_topic "astra-integration-test-tx-rb" "-a +message.type=TRANSACTION"

echo "RocketMQ topics initialized."
