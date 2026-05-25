package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/metoro-io/mcp-golang"
)

func testClient(srv *httptest.Server) *Client {
	return NewClient(srv.URL, "test-token")
}

func testText(t *testing.T, resp *mcp_golang.ToolResponse) string {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if len(resp.Content) == 0 {
		t.Fatal("response has no content")
	}
	if resp.Content[0].TextContent == nil {
		t.Fatal("content[0] is not TextContent")
	}
	return resp.Content[0].TextContent.Text
}

// ---------- get_mr_failed_jobs ----------

func TestHandleGetMRFailedJobs_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Test MR","source_branch":"feature-branch","web_url":"https://gitlab.com/test-project/-/mr/42"}`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42/pipelines", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":100,"status":"failed","ref":"feature-branch","web_url":"https://gitlab.com/test-project/-/pipelines/100"}]`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/pipelines/100/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":1,"name":"test:unit","stage":"test","status":"failed","failure_reason":"script_failure","web_url":"https://gitlab.com/-/jobs/1"},
			{"id":2,"name":"build:image","stage":"build","status":"failed","failure_reason":"exit_code_1","web_url":"https://gitlab.com/-/jobs/2"}
		]`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "Test MR") {
		t.Errorf("missing MR title, got: %s", text)
	}
	if !strings.Contains(text, "feature-branch") {
		t.Errorf("missing branch, got: %s", text)
	}
	if !strings.Contains(text, "Pipeline #100") {
		t.Errorf("missing pipeline info, got: %s", text)
	}
	if !strings.Contains(text, "test:unit") {
		t.Errorf("missing job name, got: %s", text)
	}
	if !strings.Contains(text, "build:image") {
		t.Errorf("missing job name, got: %s", text)
	}
	if !strings.Contains(text, "script_failure") {
		t.Errorf("missing failure reason, got: %s", text)
	}
}

func TestHandleGetMRFailedJobs_NoPipelines(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Test MR","source_branch":"feature-branch","web_url":"https://gitlab.com/-/mr/42"}`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42/pipelines", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "No pipelines found") {
		t.Errorf("expected 'No pipelines found', got: %s", text)
	}
}

func TestHandleGetMRFailedJobs_NoFailedJobs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Test MR","source_branch":"feature-branch","web_url":"https://gitlab.com/-/mr/42"}`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42/pipelines", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":100,"status":"success","ref":"feature-branch","web_url":"https://gitlab.com/-/pipelines/100"}]`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/pipelines/100/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "No failed jobs found") {
		t.Errorf("expected 'No failed jobs found', got: %s", text)
	}
}

func TestHandleGetMRFailedJobs_MalformedMR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestHandleGetMRFailedJobs_APIFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`Internal Server Error`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestHandleGetMRFailedJobs_PaginatedResults(t *testing.T) {
	// Simulate 21 pipelines: first page 20, second page 1.
	// The first pipeline (page1[0]) has ID 210, so jobs request goes to /pipelines/210.
	page1 := make([]pipelineAPI, 20)
	for i := 0; i < 20; i++ {
		page1[i] = pipelineAPI{ID: 210 - i, Status: "failed", Ref: "ref", WebURL: "https://gitlab.com/-/pipelines/210"}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Test MR","source_branch":"feature","web_url":"https://gitlab.com/-/mr/42"}`))
	})
	mux.HandleFunc("/api/v4/projects/test-project/merge_requests/42/pipelines", func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		switch pageStr {
		case "1":
			json.NewEncoder(w).Encode(page1)
		case "2":
			json.NewEncoder(w).Encode([]pipelineAPI{{ID: 100, Status: "failed", Ref: "ref", WebURL: "https://gitlab.com/-/pipelines/100"}})
		default:
			json.NewEncoder(w).Encode([]pipelineAPI{})
		}
	})
	mux.HandleFunc("/api/v4/projects/test-project/pipelines/210/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":1,"name":"test:unit","stage":"test","status":"failed","failure_reason":"script_failure","web_url":"https://gitlab.com/-/jobs/1"}]`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "test-project", MRIID: 42})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "Pipeline #210") {
		t.Errorf("expected latest pipeline ID 210, got: %s", text)
	}
}

// ---------- get_job_log ----------

func TestHandleGetJobLog_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("line 1\nline 2\nline 3\nline 4\nline 5\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{ProjectID: "test-project", JobID: 99, Offset: 1, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "showing lines 1-2") {
		t.Errorf("expected range info, got: %s", text)
	}
	if !strings.Contains(text, "line 2") {
		t.Errorf("expected line 2, got: %s", text)
	}
	if !strings.Contains(text, "line 3") {
		t.Errorf("expected line 3, got: %s", text)
	}
	if strings.Contains(text, "line 1") {
		t.Errorf("line 1 should not appear (offset=1), got: %s", text)
	}
}

func TestHandleGetJobLog_OffsetExceedsLength(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("line 1\nline 2\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{ProjectID: "test-project", JobID: 99, Offset: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "offset 100 exceeds log length") {
		t.Errorf("expected offset exceeds message, got: %s", text)
	}
}

func TestHandleGetJobLog_Tail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("line 1\nline 2\nline 3\nline 4\nline 5\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Tail: true, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "showing last 2 lines") {
		t.Errorf("expected 'showing last 2 lines', got: %s", text)
	}
	if !strings.Contains(text, "line 4") {
		t.Errorf("expected line 4, got: %s", text)
	}
	if !strings.Contains(text, "line 5") {
		t.Errorf("expected line 5, got: %s", text)
	}
	if strings.Contains(text, "line 1") {
		t.Errorf("line 1 should not appear in tail, got: %s", text)
	}
}

func TestHandleGetJobLog_TailFewerLines(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("only line\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Tail: true, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "showing last 1 lines") {
		t.Errorf("expected 'showing last 1 lines', got: %s", text)
	}
	if !strings.Contains(text, "only line") {
		t.Errorf("expected 'only line', got: %s", text)
	}
}

func TestHandleGetJobLog_StripANSICodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("\x1b[31mred\x1b[0m\nnormal\n\x1b[2Kclear\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "red") {
		t.Errorf("expected 'red' (without ANSI codes), got: %s", text)
	}
	if !strings.Contains(text, "normal") {
		t.Errorf("expected 'normal', got: %s", text)
	}
	if !strings.Contains(text, "clear") {
		t.Errorf("expected 'clear', got: %s", text)
	}
	// ANSI escape sequences themselves should not appear
	if strings.Contains(text, "\x1b[31m") {
		t.Errorf("ANSI codes should be stripped")
	}
}

func TestHandleGetJobLog_FilterSectionMarkers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("section_start:123\nvisible line\nsection_end:123\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if strings.Contains(text, "section_start") {
		t.Errorf("section_start should be filtered out, got: %s", text)
	}
	if strings.Contains(text, "section_end") {
		t.Errorf("section_end should be filtered out, got: %s", text)
	}
	if !strings.Contains(text, "visible line") {
		t.Errorf("expected 'visible line' to remain, got: %s", text)
	}
}

func TestHandleGetJobLog_LimitClamped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		var lines []string
		for i := 0; i < 1000; i++ {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		w.Write([]byte(strings.Join(lines, "\n")))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Limit: 1000000,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "showing lines 0-499") {
		t.Errorf("expected 500 lines max, got: %s", text)
	}
}

func TestHandleGetJobLog_DefaultLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		var lines []string
		for i := 0; i < 50; i++ {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
		w.Write([]byte(strings.Join(lines, "\n")))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Limit: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "showing lines 0-49") {
		t.Errorf("expected 50 lines (default), got: %s", text)
	}
}

func TestHandleGetJobLog_NegativeOffset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("line 1\nline 2\nline 3\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
		Offset: -5, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	// offset -5 should become 0
	if !strings.Contains(text, "showing lines 0-1") {
		t.Errorf("expected offset 0, got: %s", text)
	}
}

func TestHandleGetJobLog_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestHandleGetJobLog_NumericProjectID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/335/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("numeric project log line\n"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "335", JobID: 99,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "numeric project log line") {
		t.Errorf("expected log content, got: %s", text)
	}
}

func TestHandleGetJobLog_EmptyTrace(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/test-project/jobs/99/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetJobLog(testClient(srv), GetJobLogArgs{
		ProjectID: "test-project", JobID: 99,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "offset 0 exceeds log length") {
		t.Errorf("expected 'offset 0 exceeds log length', got: %s", text)
	}
}

// ---------- URL-encoded project path ----------

func TestHandleGetMRFailedJobs_URLEncodedProject(t *testing.T) {
	encoded := url.PathEscape("group/subgroup/project")
	mux := http.NewServeMux()
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/42", encoded)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"Encoded MR","source_branch":"feature","web_url":"https://gitlab.com/-/mr/42"}`))
	})
	pipPath := fmt.Sprintf("/api/v4/projects/%s/merge_requests/42/pipelines", encoded)
	mux.HandleFunc(pipPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":10,"status":"failed","ref":"feature","web_url":"https://gitlab.com/-/pipelines/10"}]`))
	})
	jobPath := fmt.Sprintf("/api/v4/projects/%s/pipelines/10/jobs", encoded)
	mux.HandleFunc(jobPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":1,"name":"test","stage":"test","status":"failed","failure_reason":"error","web_url":"https://gitlab.com/-/jobs/1"}]`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := HandleGetMRFailedJobs(testClient(srv), GetMRFailedJobsArgs{ProjectID: "group/subgroup/project", MRIID: 42})
	if err != nil {
		t.Fatal(err)
	}

	text := testText(t, resp)
	if !strings.Contains(text, "Encoded MR") {
		t.Errorf("expected 'Encoded MR', got: %s", text)
	}
}
