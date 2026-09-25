package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// Canvas clients must continue polling immediately after a successful submit,
// before the gateway's first upstream poll changes NOT_START to IN_PROGRESS.
func TestVideoPollingLifecycle(t *testing.T) {
	for _, tc := range []struct {
		state    model.TaskStatus
		want     string
		finished int64
	}{
		{model.TaskStatusNotStart, dto.VideoStatusQueued, 0},
		{model.TaskStatusSubmitted, dto.VideoStatusQueued, 0},
		{model.TaskStatusQueued, dto.VideoStatusQueued, 0},
		{model.TaskStatusInProgress, dto.VideoStatusInProgress, 0},
		{model.TaskStatusSuccess, dto.VideoStatusCompleted, 120},
		{model.TaskStatusFailure, dto.VideoStatusFailed, 120},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			task := &model.Task{TaskID: "task-existing", Status: tc.state,
				CreatedAt: 100, UpdatedAt: 110, FinishTime: tc.finished,
				Data: []byte(`{"id":"upstream-existing"}`)}
			body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
			if err != nil {
				t.Fatal(err)
			}
			var got dto.OpenAIVideo
			if err := common.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q", got.Status, tc.want)
			}
			if got.CompletedAt != tc.finished {
				t.Errorf("completed_at = %d, want %d", got.CompletedAt, tc.finished)
			}
		})
	}
}
