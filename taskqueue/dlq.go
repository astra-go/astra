package taskqueue

import (
	"encoding/json"
	"time"

	"github.com/astra-go/astra/mq"
)

// DLQTopic is the default NATS subject for dead-letter messages.
const DLQTopic = "taskqueue.dlq"

// DLQPayload is the standard envelope written to the dead-letter queue when
// a task exhausts all retries or is otherwise discarded.
//
// It preserves the full context of the failed task so downstream consumers
// can inspect, alert, or replay the message.
type DLQPayload struct {
	// OriginalTaskType is the TaskType of the failed task.
	OriginalTaskType TaskType `json:"original_task_type"`

	// OriginalData is the raw task payload (the Data field).
	OriginalData json.RawMessage `json:"original_data"`

	// Error is the error returned by the last handler invocation.
	Error string `json:"error"`

	// Attempts is the total number of delivery attempts made (1 = first attempt).
	Attempts int `json:"attempts"`

	// FailedAt is the ISO-8601 timestamp when the task entered the DLQ.
	FailedAt string `json:"failed_at"`

	// UIN is the optional user identifier, if present in the original payload.
	UIN string `json:"uin,omitempty"`

	// Topic is the original mq.Topic the message was published to.
	Topic string `json:"topic,omitempty"`

	// LastAttemptAt is the ISO-8601 timestamp of the last retry.
	LastAttemptAt string `json:"last_attempt_at,omitempty"`
}

// NewDLQPayload builds a DLQPayload from a failed task.
func NewDLQPayload(tt TaskType, data json.RawMessage, err error, attempts int, uin string) DLQPayload {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return DLQPayload{
		OriginalTaskType: tt,
		OriginalData:     data,
		Error:            errMsg,
		Attempts:         attempts,
		FailedAt:         time.Now().UTC().Format(time.RFC3339),
		UIN:              uin,
	}
}

// ToMessage converts the DLQPayload into a mq.Message ready to publish.
func (p DLQPayload) ToMessage() (*mq.Message, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return &mq.Message{
		Topic:   DLQTopic,
		Payload: payload,
		Headers: map[string]string{
			"x-task-type": string(p.OriginalTaskType),
		},
	}, nil
}
