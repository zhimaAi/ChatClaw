package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// GiteeSource implements selfupdate.Source using Gitee API v5.
// Gitee release download URLs follow the same pattern as GitHub:
//
//	https://gitee.com/{owner}/{repo}/releases/download/{tag}/{filename}
type GiteeSource struct {
	client *http.Client
}

// NewGiteeSource creates a new Gitee source.
func NewGiteeSource() *GiteeSource {
	return &GiteeSource{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// --- Gitee API v5 response types ---

type giteeRelease struct {
	ID         int64        `json:"id"`
	TagName    string       `json:"tag_name"`
	Name       string       `json:"name"`
	Body       string       `json:"body"`
	Prerelease bool         `json:"prerelease"`
	CreatedAt  string       `json:"created_at"`
	Assets     []giteeAsset `json:"assets"`
}

type giteeAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// --- SourceRelease adapter ---

type giteeSourceRelease struct {
	rel giteeRelease
}

func (r *giteeSourceRelease) GetID() int64           { return r.rel.ID }
func (r *giteeSourceRelease) GetTagName() string      { return r.rel.TagName }
func (r *giteeSourceRelease) GetName() string         { return r.rel.Name }
func (r *giteeSourceRelease) GetDraft() bool          { return false }
func (r *giteeSourceRelease) GetPrerelease() bool     { return r.rel.Prerelease }
func (r *giteeSourceRelease) GetReleaseNotes() string { return r.rel.Body }
func (r *giteeSourceRelease) GetURL() string          { return "" }
func (r *giteeSourceRelease) GetPublishedAt() time.Time {
	t, _ := time.Parse(time.RFC3339, r.rel.CreatedAt)
	return t
}
func (r *giteeSourceRelease) GetAssets() []selfupdate.SourceAsset {
	out := make([]selfupdate.SourceAsset, len(r.rel.Assets))
	for i, a := range r.rel.Assets {
		out[i] = &giteeSourceAsset{asset: a, id: int64(i + 1)}
	}
	return out
}

// --- SourceAsset adapter ---

type giteeSourceAsset struct {
	asset giteeAsset
	id    int64
}

func (a *giteeSourceAsset) GetID() int64                  { return a.id }
func (a *giteeSourceAsset) GetName() string               { return a.asset.Name }
func (a *giteeSourceAsset) GetSize() int                  { return 0 }
func (a *giteeSourceAsset) GetBrowserDownloadURL() string { return a.asset.BrowserDownloadURL }

// ListReleases fetches releases from Gitee API v5.
func (s *GiteeSource) ListReleases(ctx context.Context, repository selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	owner, repo, err := repository.GetSlug()
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://gitee.com/api/v5/repos/%s/%s/releases?page=1&per_page=30", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitee: create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitee: list releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitee: unexpected status %d", resp.StatusCode)
	}

	var releases []giteeRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("gitee: decode: %w", err)
	}

	result := make([]selfupdate.SourceRelease, len(releases))
	for i := range releases {
		result[i] = &giteeSourceRelease{rel: releases[i]}
	}
	return result, nil
}

// DownloadReleaseAsset downloads an asset via its browser_download_url (HTTP direct link).
func (s *GiteeSource) DownloadReleaseAsset(ctx context.Context, rel *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	if rel == nil {
		return nil, fmt.Errorf("gitee: release is nil")
	}
	if rel.AssetURL == "" {
		return nil, fmt.Errorf("gitee: asset URL is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.AssetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("gitee: download request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitee: download: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("gitee: download status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

var _ selfupdate.Source = (*GiteeSource)(nil)
