package apimart

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func TestConvertImageRequest(t *testing.T) {
	a := &Adaptor{}
	n := uint(1)
	req := dto.ImageRequest{Model: "gpt-image-2", Prompt: "a blue cube", N: &n, Size: "1024x1024"}
	out, err := a.ConvertImageRequest(nil, nil, req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	b, _ := common.Marshal(out)
	var got SubmitRequest
	if err := common.Unmarshal(b, &got); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got.Model != "gpt-image-2" || got.Prompt != "a blue cube" || got.Size != "1024x1024" || got.N == nil || *got.N != 1 {
		t.Fatalf("bad mapping: %+v", got)
	}
}

func TestConvertImageRequestPreservesImageURLs(t *testing.T) {
	a := &Adaptor{}
	var req dto.ImageRequest
	if err := common.Unmarshal([]byte(`{
		"model":"gpt-image-2",
		"prompt":"make this more cinematic",
		"image_urls":["https://cdn.example/ref-a.png","https://cdn.example/ref-b.png"]
	}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := a.ConvertImageRequest(nil, nil, req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	b, _ := common.Marshal(out)
	var got SubmitRequest
	if err := common.Unmarshal(b, &got); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(got.ImageURLs) != 2 || got.ImageURLs[0] != "https://cdn.example/ref-a.png" || got.ImageURLs[1] != "https://cdn.example/ref-b.png" {
		t.Fatalf("image_urls not preserved: %+v", got.ImageURLs)
	}
}

func TestConvertImageRequestOfficialParams(t *testing.T) {
	a := &Adaptor{}
	var req dto.ImageRequest
	if err := common.Unmarshal([]byte(`{
		"model":"gpt-image-2-official",
		"prompt":"星空下的古老城堡",
		"size":"16:9",
		"quality":"high",
		"resolution":"2k",
		"output_format":"webp",
		"mask_url":"https://cdn.example/mask.png"
	}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := a.ConvertImageRequest(nil, nil, req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	b, _ := common.Marshal(out)
	var got SubmitRequest
	if err := common.Unmarshal(b, &got); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got.Model != "gpt-image-2-official" || got.Size != "16:9" || got.Quality != "high" ||
		got.Resolution != "2k" || got.OutputFormat != "webp" || got.MaskURL != "https://cdn.example/mask.png" {
		t.Fatalf("official params not mapped: %+v", got)
	}
}

func TestPollTaskCompletes(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, `{"code":200,"data":{"status":"pending","progress":50}}`)
			return
		}
		io.WriteString(w, `{"code":200,"data":{"status":"completed","progress":100,"result":{"images":[{"url":["https://cdn/x.png"]}]}}}`)
	}))
	defer srv.Close()

	url, err := pollTask(context.Background(), srv.Client(), srv.URL, "k", "task_1", 10*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if url != "https://cdn/x.png" {
		t.Fatalf("got url %q", url)
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 polls, got %d", calls)
	}
}

func TestPollTaskFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":200,"data":{"status":"failed","error":"moderation"}}`)
	}))
	defer srv.Close()
	_, err := pollTask(context.Background(), srv.Client(), srv.URL, "k", "task_1", 10*time.Millisecond, 5*time.Second)
	if err == nil {
		t.Fatalf("expected error on failed status")
	}
}

func TestTaskAdaptorParseTaskResultCompleted(t *testing.T) {
	a := &TaskAdaptor{}
	got, err := a.ParseTaskResult([]byte(`{"code":200,"data":{"status":"completed","progress":100,"result":{"images":[{"url":["https://cdn.example/out.png"]}]}}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Status != model.TaskStatusSuccess {
		t.Fatalf("status = %q, want %q", got.Status, model.TaskStatusSuccess)
	}
	if got.Url != "https://cdn.example/out.png" {
		t.Fatalf("url = %q", got.Url)
	}
}

func TestTaskAdaptorParseTaskResultFailedObjectError(t *testing.T) {
	a := &TaskAdaptor{}
	got, err := a.ParseTaskResult([]byte(`{"code":200,"data":{"status":"failed","progress":100,"error":{"code":"task_failed","message":"moderation blocked"}}}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Status != model.TaskStatusFailure {
		t.Fatalf("status = %q, want %q", got.Status, model.TaskStatusFailure)
	}
	if got.Reason == "" || got.Reason == "{}" {
		t.Fatalf("empty reason: %q", got.Reason)
	}
}
