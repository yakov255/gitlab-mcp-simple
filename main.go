package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

type GetMRFailedJobsArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=GitLab project path or numeric ID, e.g. 'raketa/raketa' or 335"`
	MRIID     int    `json:"mr_iid" jsonschema:"required,description=Merge Request internal ID (IID), e.g. 35063"`
}

type GetJobLogArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=GitLab project path or numeric ID"`
	JobID     int    `json:"job_id" jsonschema:"required,description=Job ID to fetch logs for"`
	Offset    int    `json:"offset" jsonschema:"description=Line number to start from (0-based), default 0"`
	Limit     int    `json:"limit" jsonschema:"description=Number of lines to return, default 100, max 500"`
}

type mrAPI struct {
	SourceBranch string `json:"source_branch"`
	Title        string `json:"title"`
	WebURL       string `json:"web_url"`
}

type pipelineAPI struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	WebURL string `json:"web_url"`
}

type jobAPI struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
	WebURL        string `json:"web_url"`
}

func config() (string, string, error) {
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return "", "", fmt.Errorf("GITLAB_TOKEN environment variable is required")
	}
	baseURL := os.Getenv("GITLAB_BASE_URL")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	return baseURL, token, nil
}

func gitLabAPI(path string) ([]byte, error) {
	baseURL, token, err := config()
	if err != nil {
		return nil, err
	}

	u := baseURL + "/api/v4/" + path
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func gitLabAPIPaginated(path string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	page := 1
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for {
		body, err := gitLabAPI(fmt.Sprintf("%s%sper_page=20&page=%d", path, sep, page))
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("failed to decode list: %w", err)
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if len(items) < 20 {
			break
		}
		page++
	}
	return all, nil
}

func fetchTrace(project string, jobID int) ([]byte, error) {
	baseURL, token, err := config()
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/api/v4/projects/%s/jobs/%d/trace", baseURL, project, jobID)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned %d", resp.StatusCode)
	}
	return body, nil
}

func stripANSICodes(s string) string {
	var b strings.Builder
	in := false
	for _, c := range s {
		if c == '\x1b' {
			in = true
			continue
		}
		if in {
			if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
				in = false
			}
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

func filterLogLine(line string) string {
	clean := stripANSICodes(line)
	clean = strings.TrimRight(clean, " \t\r")
	if clean == "" {
		return ""
	}
	if strings.Contains(clean, " section_start:") || strings.Contains(clean, " section_end:") {
		return ""
	}
	return clean
}

func main() {
	server := mcp_golang.NewServer(stdio.NewStdioServerTransport())

	err := server.RegisterTool("get_mr_failed_jobs", "Lists failed jobs from the latest pipeline for a GitLab merge request", func(arguments GetMRFailedJobsArgs) (*mcp_golang.ToolResponse, error) {
		project := url.PathEscape(arguments.ProjectID)
		mrIID := arguments.MRIID

		mrBody, err := gitLabAPI(fmt.Sprintf("projects/%s/merge_requests/%d", project, mrIID))
		if err != nil {
			return nil, err
		}
		var mr mrAPI
		if err := json.Unmarshal(mrBody, &mr); err != nil {
			return nil, fmt.Errorf("failed to decode MR: %w", err)
		}

		pipelinesRaw, err := gitLabAPIPaginated(fmt.Sprintf("projects/%s/merge_requests/%d/pipelines?order_by=id&sort=desc", project, mrIID))
		if err != nil {
			return nil, err
		}
		if len(pipelinesRaw) == 0 {
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("No pipelines found for this merge request.")), nil
		}
		var latestPipeline pipelineAPI
		if err := json.Unmarshal(pipelinesRaw[0], &latestPipeline); err != nil {
			return nil, fmt.Errorf("failed to decode pipeline: %w", err)
		}

		jobsRaw, err := gitLabAPIPaginated(fmt.Sprintf("projects/%s/pipelines/%d/jobs?scope[]=failed", project, latestPipeline.ID))
		if err != nil {
			return nil, err
		}

		var parts []string
		parts = append(parts, fmt.Sprintf("MR: %s", mr.Title))
		parts = append(parts, fmt.Sprintf("Branch: %s", mr.SourceBranch))
		parts = append(parts, fmt.Sprintf("Pipeline #%d — %s", latestPipeline.ID, latestPipeline.Status))
		parts = append(parts, fmt.Sprintf("Pipeline URL: %s", latestPipeline.WebURL))

		if len(jobsRaw) == 0 {
			parts = append(parts, "\nNo failed jobs found in the latest pipeline.")
		} else {
			parts = append(parts, fmt.Sprintf("\nFailed jobs (%d):", len(jobsRaw)))
			for _, jr := range jobsRaw {
				var job jobAPI
				if err := json.Unmarshal(jr, &job); err != nil {
					continue
				}
				parts = append(parts, fmt.Sprintf("\n  [%d] %s (%s)", job.ID, job.Name, job.Stage))
				parts = append(parts, fmt.Sprintf("       Status: %s", job.Status))
				parts = append(parts, fmt.Sprintf("       Reason: %s", job.FailureReason))
				parts = append(parts, fmt.Sprintf("       URL: %s", job.WebURL))
			}
			parts = append(parts, "\nUse get_job_log with a job_id to see the logs.")
		}

		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(strings.Join(parts, "\n"))), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterTool("get_job_log", "Gets a portion of a job log. Returns the requested lines and total line count.", func(arguments GetJobLogArgs) (*mcp_golang.ToolResponse, error) {
		project := url.PathEscape(arguments.ProjectID)

		limit := arguments.Limit
		if limit <= 0 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}
		offset := arguments.Offset
		if offset < 0 {
			offset = 0
		}

		trace, err := fetchTrace(project, arguments.JobID)
		if err != nil {
			return nil, err
		}

		rawLines := strings.Split(string(trace), "\n")
		totalLines := len(rawLines)

		var cleanLines []string
		for _, l := range rawLines {
			if filtered := filterLogLine(l); filtered != "" {
				cleanLines = append(cleanLines, filtered)
			}
		}

		if offset >= len(cleanLines) {
			text := fmt.Sprintf("Job %d\nTotal lines: %d\nFiltered lines: %d\nRequested offset %d exceeds log length.",
				arguments.JobID, totalLines, len(cleanLines), offset)
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(text)), nil
		}

		end := offset + limit
		if end > len(cleanLines) {
			end = len(cleanLines)
		}
		selected := cleanLines[offset:end]

		var parts []string
		parts = append(parts, fmt.Sprintf("Job %d — showing lines %d-%d (filtered) of %d raw lines",
			arguments.JobID, offset, end-1, totalLines))
		parts = append(parts, fmt.Sprintf("Total filtered lines: %d\n", len(cleanLines)))
		parts = append(parts, selected...)

		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(strings.Join(parts, "\n"))), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.Serve()
	if err != nil {
		panic(err)
	}

	select {}
}
