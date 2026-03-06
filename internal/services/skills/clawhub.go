package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Global rate limiter for ClawHub API: max 2 requests per second (one every 500ms).
// Shared across all SkillsService instances, all goroutines.
var clawHubThrottle = &throttle{minInterval: 500 * time.Millisecond}

type throttle struct {
	mu          sync.Mutex
	lastReq     time.Time
	minInterval time.Duration
}

func (t *throttle) wait() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if elapsed := time.Since(t.lastReq); elapsed < t.minInterval {
		time.Sleep(t.minInterval - elapsed)
	}
	t.lastReq = time.Now()
}

const clawHubBaseURL = "https://clawhub.ai/api/v1"

// RemoteSkill represents a skill from the ClawHub marketplace.
type RemoteSkill struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
	Version     string `json:"version"`
	Downloads   int    `json:"downloads"`
	Installs    int    `json:"installs"`
	Stars       int    `json:"stars"`
	Installed   bool   `json:"installed"`
}

type ExploreResult struct {
	Items      []RemoteSkill `json:"items"`
	NextCursor string        `json:"nextCursor"`
}

type SkillDetail struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
	Version     string `json:"version"`
	Downloads   int    `json:"downloads"`
	Installs    int    `json:"installs"`
	Stars       int    `json:"stars"`
	Changelog   string `json:"changelog"`
	Installed   bool   `json:"installed"`
	OwnerName   string `json:"ownerName"`
	OwnerImage  string `json:"ownerImage"`
}

// --- ClawHub API response types (internal) ---

type clawHubExploreResp struct {
	Items      []clawHubSkillItem `json:"items"`
	NextCursor string             `json:"nextCursor"`
}

type clawHubSkillItem struct {
	Slug        string            `json:"slug"`
	DisplayName string            `json:"displayName"`
	Summary     string            `json:"summary"`
	Tags        map[string]string `json:"tags"`
	Stats       clawHubStats      `json:"stats"`
}

type clawHubStats struct {
	Downloads       int `json:"downloads"`
	InstallsAllTime int `json:"installsAllTime"`
	Stars           int `json:"stars"`
}

type clawHubSearchResp struct {
	Results []clawHubSearchItem `json:"results"`
}

type clawHubSearchItem struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
	Version     *string `json:"version"`
}

type clawHubDetailResp struct {
	Skill         clawHubSkillItem   `json:"skill"`
	LatestVersion *clawHubVersionObj `json:"latestVersion"`
	Owner         *clawHubOwner      `json:"owner"`
}

type clawHubOwner struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Image       string `json:"image"`
}

type clawHubVersionObj struct {
	Version   string            `json:"version"`
	Changelog string            `json:"changelog"`
	Files     []clawHubFileInfo `json:"files"`
}

type clawHubVersionResp struct {
	Skill   clawHubSkillItem  `json:"skill"`
	Version clawHubVersionObj `json:"version"`
}

type clawHubFileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// --- API methods ---

func (s *SkillsService) SearchSkills(query string, limit int) ([]RemoteSkill, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/search?q=%s&limit=%d", clawHubBaseURL, url.QueryEscape(query), limit)
	body, err := s.httpGet(u)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var resp clawHubSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("search response parse failed: %w", err)
	}

	installed := s.installedSet()
	out := make([]RemoteSkill, 0, len(resp.Results))
	for _, r := range resp.Results {
		ver := ""
		if r.Version != nil {
			ver = *r.Version
		}
		out = append(out, RemoteSkill{
			Slug:        r.Slug,
			DisplayName: r.DisplayName,
			Summary:     r.Summary,
			Version:     ver,
			Installed:   installed[r.Slug],
		})
	}
	return out, nil
}

func (s *SkillsService) ExploreSkills(limit int, sort, cursor string) (*ExploreResult, error) {
	if limit <= 0 {
		limit = 25
	}
	if sort == "" {
		sort = "trending"
	}

	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))
	params.Set("sort", sort)
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	u := fmt.Sprintf("%s/skills?%s", clawHubBaseURL, params.Encode())

	body, err := s.httpGet(u)
	if err != nil {
		return nil, fmt.Errorf("explore request failed: %w", err)
	}

	var resp clawHubExploreResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("explore response parse failed: %w", err)
	}

	installed := s.installedSet()
	items := make([]RemoteSkill, 0, len(resp.Items))
	for _, it := range resp.Items {
		items = append(items, RemoteSkill{
			Slug:        it.Slug,
			DisplayName: it.DisplayName,
			Summary:     it.Summary,
			Version:     it.Tags["latest"],
			Downloads:   it.Stats.Downloads,
			Installs:    it.Stats.InstallsAllTime,
			Stars:       it.Stats.Stars,
			Installed:   installed[it.Slug],
		})
	}
	return &ExploreResult{Items: items, NextCursor: resp.NextCursor}, nil
}

func (s *SkillsService) GetSkillDetail(slug string) (*SkillDetail, error) {
	u := fmt.Sprintf("%s/skills/%s", clawHubBaseURL, url.PathEscape(slug))
	body, err := s.httpGet(u)
	if err != nil {
		return nil, fmt.Errorf("detail request failed: %w", err)
	}

	var resp clawHubDetailResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("detail response parse failed: %w", err)
	}

	ver := ""
	changelog := ""
	if resp.LatestVersion != nil {
		ver = resp.LatestVersion.Version
		changelog = resp.LatestVersion.Changelog
	}

	installed := s.installedSet()
	detail := &SkillDetail{
		Slug:        resp.Skill.Slug,
		DisplayName: resp.Skill.DisplayName,
		Summary:     resp.Skill.Summary,
		Version:     ver,
		Downloads:   resp.Skill.Stats.Downloads,
		Installs:    resp.Skill.Stats.InstallsAllTime,
		Stars:       resp.Skill.Stats.Stars,
		Changelog:   changelog,
		Installed:   installed[resp.Skill.Slug],
	}
	if resp.Owner != nil {
		detail.OwnerName = resp.Owner.DisplayName
		if detail.OwnerName == "" {
			detail.OwnerName = resp.Owner.Handle
		}
		detail.OwnerImage = resp.Owner.Image
	}
	return detail, nil
}

// getVersionFiles fetches the file list for a specific skill version.
func (s *SkillsService) getVersionFiles(slug, version string) ([]clawHubFileInfo, error) {
	u := fmt.Sprintf("%s/skills/%s/versions/%s", clawHubBaseURL, url.PathEscape(slug), url.PathEscape(version))
	body, err := s.httpGet(u)
	if err != nil {
		return nil, fmt.Errorf("version request failed: %w", err)
	}

	var resp clawHubVersionResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("version response parse failed: %w", err)
	}
	return resp.Version.Files, nil
}

// getFileContent downloads a single file from ClawHub.
func (s *SkillsService) getFileContent(slug, version, path string) ([]byte, error) {
	params := url.Values{}
	params.Set("path", path)
	params.Set("version", version)
	u := fmt.Sprintf("%s/skills/%s/file?%s", clawHubBaseURL, url.PathEscape(slug), params.Encode())
	return s.httpGet(u)
}

func (s *SkillsService) httpGet(rawURL string) ([]byte, error) {
	const maxRetries = 5
	const maxBackoff = 60 * time.Second
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		clawHubThrottle.wait()

		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "WillChat/1.0")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt < maxRetries {
				wait := retryAfterFromHeaders(resp.Header, backoff)
				jitter := time.Duration(rand.Int63n(int64(wait/4) + 1))
				time.Sleep(wait + jitter)
				backoff = min(backoff*2, maxBackoff)
				continue
			}
			return nil, fmt.Errorf("rate limited (HTTP 429) after %d retries", maxRetries)
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}
		return io.ReadAll(resp.Body)
	}
	return nil, fmt.Errorf("unexpected retry loop exit")
}

// retryAfterFromHeaders extracts the wait duration from rate-limit response
// headers. It prefers Retry-After, then falls back to RateLimit-Reset (delay
// seconds), then X-RateLimit-Reset (absolute epoch). If none are usable it
// returns the provided fallback.
func retryAfterFromHeaders(h http.Header, fallback time.Duration) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if v := h.Get("RateLimit-Reset"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil {
			if d := time.Until(time.Unix(epoch, 0)); d > 0 {
				return d
			}
		}
	}
	return fallback
}

// GetRemoteSkillMD fetches the SKILL.md content from ClawHub for a given skill.
func (s *SkillsService) GetRemoteSkillMD(slug, version string) (string, error) {
	if version == "" {
		version = "latest"
	}

	files, err := s.getVersionFiles(slug, version)
	if err != nil {
		return "", fmt.Errorf("failed to get version files: %w", err)
	}

	hasSkillMD := false
	for _, f := range files {
		if f.Path == "SKILL.md" {
			hasSkillMD = true
			break
		}
	}
	if !hasSkillMD {
		return "", fmt.Errorf("SKILL.md not found in version %s", version)
	}

	content, err := s.getFileContent(slug, version, "SKILL.md")
	if err != nil {
		return "", fmt.Errorf("failed to fetch SKILL.md: %w", err)
	}
	return string(content), nil
}

// GetRemoteSkillFiles returns the file list for a remote skill version.
func (s *SkillsService) GetRemoteSkillFiles(slug, version string) ([]SkillFileInfo, error) {
	if version == "" {
		version = "latest"
	}
	files, err := s.getVersionFiles(slug, version)
	if err != nil {
		return nil, err
	}
	out := make([]SkillFileInfo, 0, len(files))
	for _, f := range files {
		out = append(out, SkillFileInfo{Path: f.Path, Size: f.Size})
	}
	sortSkillFiles(out)
	return out, nil
}

// GetRemoteSkillFile fetches a single file's content from ClawHub.
func (s *SkillsService) GetRemoteSkillFile(slug, version, path string) (string, error) {
	if version == "" {
		version = "latest"
	}
	content, err := s.getFileContent(slug, version, path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// installedSet returns a set of installed skill slugs for quick lookup.
func (s *SkillsService) installedSet() map[string]bool {
	set := make(map[string]bool)
	rows, err := s.ListInstalledSkills("all")
	if err != nil {
		return set
	}
	for _, r := range rows {
		set[r.Slug] = true
	}
	return set
}
