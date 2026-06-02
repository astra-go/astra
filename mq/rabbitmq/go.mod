module github.com/astra-go/astra/mq/rabbitmq

go 1.25.1

require (
	github.com/astra-go/astra/mq v0.1.0
	github.com/rabbitmq/amqp091-go v1.10.0
)

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ./..
