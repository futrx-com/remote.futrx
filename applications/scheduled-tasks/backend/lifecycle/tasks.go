package lifecycle

import (
	"encoding/json"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type TaskEvents interface {
	Due(taskID, runID string) error
}
type Tasks struct{ events applications.EventEmitter }

func NewTasks(events applications.EventEmitter) *Tasks { return &Tasks{events: events} }
func (t *Tasks) Due(taskID, runID string) error {
	payload, err := json.Marshal(map[string]string{"taskId": taskID, "runId": runID})
	if err != nil {
		return err
	}
	return t.events.Emit(applications.Publication{Publisher: "tasks", Event: "due", Version: 1, Payload: payload})
}
