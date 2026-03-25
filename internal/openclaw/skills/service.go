package openclawskills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chatclaw/internal/define"
	openclawagents "chatclaw/internal/openclaw/agents"
	openclawruntime "chatclaw/internal/openclaw/runtime"

	"gopkg.in/yaml.v3"
)

// OpenClawSkill is a skill from the OpenClaw Gateway (skills.status) and/or on-disk discovery
// following https://docs.openclaw.ai/tools/skills (managed ~/.openclaw/skills, workspace /skills, bundled, extraDirs).
type OpenClawSkill struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	// Permission summarizes declared access (SKILL.md or gateway metadata).
	Permission string `json:"permission"`
	// Scope is optional frontmatter scope when present on disk.
	Scope string `json:"scope"`
	// Location groups UI filters: "shared" (managed, bundled, extra, gateway-global) vs "workspace".
	Location string `json:"location"`
	// DataSource is where the row came from: gateway, managed, workspace, bundled, extra.
	DataSource string `json:"dataSource"`
	// Eligible is set when the list came from skills.status (nil if unknown / disk-only).
	Eligible *bool `json:"eligible,omitempty"`
	// IneligibleReason from gateway when eligible is false.
	IneligibleReason string `json:"ineligibleReason,omitempty"`
	// AgentID is the OpenClaw agent id for workspace-scoped rows.
	AgentID string `json:"agentId"`
	// AgentName is the ChatClaw display name when known.
	AgentName string `json:"agentName"`
	// SkillRoot is the absolute path to the skill directory when discovered on disk (for file preview).
	SkillRoot string `json:"skillRoot"`
	// Installations lists every on-disk copy (workspace-*/skills, managed, bundled, extraDirs) for this slug.
	Installations []SkillInstallation `json:"installations,omitempty"`
}

// SkillInstallation is one resolved folder for a skill (multi-workspace / multi-layer).
type SkillInstallation struct {
	OpenClawAgentID string `json:"openclawAgentId"`
	AgentName       string `json:"agentName"`
	SkillRoot       string `json:"skillRoot"`
	// Layer matches DataSource on disk: managed, workspace, bundled, extra.
	Layer string `json:"layer"`
	// Location is shared vs workspace (same semantics as OpenClawSkill.Location).
	Location string `json:"location"`
}

// SkillFileInfo mirrors the native skills binding shape for file previews.
type SkillFileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// OpenClawSkillsService lists skills via OpenClaw Gateway skills.status when connected,
// otherwise reads the same directories OpenClaw uses under OPENCLAW_STATE_DIR (ChatClaw: ~/.chatclaw/openclaw).
type OpenClawSkillsService struct {
	agents *openclawagents.OpenClawAgentsService
	mgr    *openclawruntime.Manager
}

func NewOpenClawSkillsService(agents *openclawagents.OpenClawAgentsService, mgr *openclawruntime.Manager) *OpenClawSkillsService {
	return &OpenClawSkillsService{agents: agents, mgr: mgr}
}

// GetSkillsRoot returns the main agent workspace skills directory (…/workspace-main/skills).
// This matches where OpenClaw CLI `skills install` places skills for the default workspace.
func (s *OpenClawSkillsService) GetSkillsRoot() (string, error) {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return "", err
	}
	if err := define.EnsureDataLayout(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, "workspace-"+define.OpenClawMainAgentID, "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// GetManagedSkillsRoot returns the optional managed override directory (…/openclaw/skills),
// equivalent to standalone ~/.openclaw/skills under OPENCLAW_STATE_DIR.
func (s *OpenClawSkillsService) GetManagedSkillsRoot() (string, error) {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return "", err
	}
	if err := define.EnsureDataLayout(); err != nil {
		return "", err
	}
	dir := filepath.Join(root, "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ListSkills prefers Gateway skills.status; falls back to disk layout documented by OpenClaw.
func (s *OpenClawSkillsService) ListSkills() ([]OpenClawSkill, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()

	disk := s.listFromDisk()
	if s.mgr != nil && s.mgr.IsReady() {
		if api := s.collectFromGateway(ctx); api != nil {
			api = dedupeGatewayByCanonicalSlug(api)
			s.applyAgentNames(api)
			return mergeGatewayAndDisk(api, disk), nil
		}
	}
	return mergeGatewayAndDisk(nil, disk), nil
}

func (s *OpenClawSkillsService) collectFromGateway(ctx context.Context) []OpenClawSkill {
	raw, err := s.mgr.SkillsStatus(ctx, "")
	if err != nil {
		return nil
	}
	list, ok := parseSkillsStatusJSON(raw)
	if !ok {
		return nil
	}
	if len(list) > 0 {
		s.applyAgentNames(list)
		return list
	}
	// Global scope returned an empty list — try per-agent (workspace) views.
	var merged []OpenClawSkill
	seen := map[string]struct{}{}
	if s.agents == nil {
		return list
	}
	agents, err := s.agents.ListAgents()
	if err != nil {
		return list
	}
	for _, a := range agents {
		aid := strings.TrimSpace(a.OpenClawAgentID)
		if aid == "" {
			continue
		}
		r2, err2 := s.mgr.SkillsStatus(ctx, aid)
		if err2 != nil {
			continue
		}
		part, ok2 := parseSkillsStatusJSON(r2)
		if !ok2 {
			continue
		}
		for _, sk := range part {
			if strings.TrimSpace(sk.AgentID) == "" {
				sk.AgentID = aid
			}
			if strings.TrimSpace(sk.AgentName) == "" {
				sk.AgentName = strings.TrimSpace(a.Name)
			}
			if sk.Location == "" {
				sk.Location = "workspace"
			}
			k := skillDedupKey(sk)
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			merged = append(merged, sk)
		}
	}
	if len(merged) > 0 {
		return merged
	}
	return list
}

func (s *OpenClawSkillsService) applyAgentNames(list []OpenClawSkill) {
	if s.agents == nil {
		return
	}
	agents, err := s.agents.ListAgents()
	if err != nil {
		return
	}
	byID := map[string]string{}
	for _, a := range agents {
		byID[strings.ToLower(strings.TrimSpace(a.OpenClawAgentID))] = strings.TrimSpace(a.Name)
	}
	for i := range list {
		if list[i].AgentName != "" {
			continue
		}
		aid := strings.TrimSpace(list[i].AgentID)
		if aid == "" {
			continue
		}
		if n, ok := byID[strings.ToLower(aid)]; ok {
			list[i].AgentName = n
		}
	}
}

func skillDedupKey(sk OpenClawSkill) string {
	return strings.ToLower(strings.TrimSpace(sk.AgentID)) + "\x00" + strings.ToLower(strings.TrimSpace(sk.Slug))
}

// canonicalSkillSlug normalizes gateway/disk identifiers so "Quotes", "quotes", and folder names align.
func canonicalSkillSlug(sk OpenClawSkill) string {
	s := strings.TrimSpace(sk.Slug)
	if s == "" {
		s = strings.TrimSpace(sk.Name)
	}
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return strings.Trim(s, "-")
}

func dedupeGatewayByCanonicalSlug(api []OpenClawSkill) []OpenClawSkill {
	groups := map[string][]OpenClawSkill{}
	order := []string{}
	for _, a := range api {
		k := canonicalSkillSlug(a)
		if k == "" {
			continue
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], a)
	}
	out := make([]OpenClawSkill, 0, len(order))
	for _, k := range order {
		out = append(out, bestGatewayRow(groups[k]))
	}
	return out
}

func gatewayRowScore(r OpenClawSkill) int {
	sc := 0
	if r.Eligible != nil && *r.Eligible {
		sc += 100
	}
	sc += len(strings.TrimSpace(r.Description)) / 4
	if strings.TrimSpace(r.Permission) != "" {
		sc += 10
	}
	if strings.TrimSpace(r.Scope) != "" {
		sc += 3
	}
	if strings.TrimSpace(r.AgentID) != "" {
		sc += 1
	}
	return sc
}

func bestGatewayRow(rows []OpenClawSkill) OpenClawSkill {
	if len(rows) == 0 {
		return OpenClawSkill{}
	}
	best := rows[0]
	for _, r := range rows[1:] {
		rs, bs := gatewayRowScore(r), gatewayRowScore(best)
		if rs > bs || (rs == bs && len(strings.TrimSpace(r.Description)) > len(strings.TrimSpace(best.Description))) {
			best = r
		}
	}
	return best
}

func pickPrimarySkillRoot(disk []OpenClawSkill) string {
	main := strings.TrimSpace(define.OpenClawMainAgentID)
	for _, d := range disk {
		if strings.EqualFold(strings.TrimSpace(d.AgentID), main) && strings.TrimSpace(d.SkillRoot) != "" {
			return d.SkillRoot
		}
	}
	for _, d := range disk {
		if strings.TrimSpace(d.SkillRoot) != "" {
			return d.SkillRoot
		}
	}
	return ""
}

func mergeGatewayAndDisk(api []OpenClawSkill, disk []OpenClawSkill) []OpenClawSkill {
	bySlugDisk := map[string][]OpenClawSkill{}
	for _, d := range disk {
		k := canonicalSkillSlug(d)
		if k == "" {
			continue
		}
		bySlugDisk[k] = append(bySlugDisk[k], d)
	}
	bySlugAPI := map[string]OpenClawSkill{}
	for _, a := range api {
		k := canonicalSkillSlug(a)
		if k == "" {
			continue
		}
		bySlugAPI[k] = a
	}
	all := map[string]struct{}{}
	for k := range bySlugDisk {
		all[k] = struct{}{}
	}
	for k := range bySlugAPI {
		all[k] = struct{}{}
	}
	slugs := make([]string, 0, len(all))
	for k := range all {
		slugs = append(slugs, k)
	}
	sort.Strings(slugs)
	out := make([]OpenClawSkill, 0, len(slugs))
	for _, slug := range slugs {
		dlist := bySlugDisk[slug]
		gw, hasGW := bySlugAPI[slug]
		row := buildMergedSkillRow(slug, gw, hasGW, dlist)
		if canonicalSkillSlug(row) != "" {
			out = append(out, row)
		}
	}
	return out
}

func buildMergedSkillRow(slug string, gw OpenClawSkill, hasGW bool, dlist []OpenClawSkill) OpenClawSkill {
	var row OpenClawSkill
	switch {
	case hasGW:
		row = gw
	case len(dlist) > 0:
		row = dlist[0]
	default:
		return OpenClawSkill{}
	}
	if len(dlist) > 0 {
		row.Slug = dlist[0].Slug
	} else if strings.TrimSpace(row.Slug) == "" {
		row.Slug = slug
	}
	if strings.TrimSpace(row.Name) == "" {
		row.Name = row.Slug
	}
	var inst []SkillInstallation
	for _, d := range dlist {
		if strings.TrimSpace(d.SkillRoot) == "" {
			continue
		}
		inst = append(inst, SkillInstallation{
			OpenClawAgentID: d.AgentID,
			AgentName:       d.AgentName,
			SkillRoot:       d.SkillRoot,
			Layer:           d.DataSource,
			Location:        d.Location,
		})
	}
	sort.Slice(inst, func(i, j int) bool {
		ai, aj := inst[i].OpenClawAgentID, inst[j].OpenClawAgentID
		if ai != aj {
			return ai < aj
		}
		return inst[i].SkillRoot < inst[j].SkillRoot
	})
	row.Installations = inst
	row.SkillRoot = pickPrimarySkillRoot(dlist)
	if hasGW {
		for _, d := range dlist {
			if strings.TrimSpace(row.Description) == "" && strings.TrimSpace(d.Description) != "" {
				row.Description = d.Description
			}
			if strings.TrimSpace(row.Permission) == "" && strings.TrimSpace(d.Permission) != "" {
				row.Permission = d.Permission
			}
			if strings.TrimSpace(row.Scope) == "" && strings.TrimSpace(d.Scope) != "" {
				row.Scope = d.Scope
			}
			if strings.TrimSpace(row.Version) == "" && strings.TrimSpace(d.Version) != "" {
				row.Version = d.Version
			}
		}
	}
	return row
}

func parseSkillsStatusJSON(raw json.RawMessage) ([]OpenClawSkill, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err == nil {
		for _, key := range []string{"skills", "items", "entries", "list"} {
			if v, ok := wrap[key]; ok {
				out := mapsToSkillsFromJSONArray(v)
				return out, true
			}
		}
	}
	var rootArr []map[string]any
	if err := json.Unmarshal(raw, &rootArr); err == nil {
		return mapsToSkillsFromMaps(rootArr), true
	}
	return nil, false
}

func mapsToSkillsFromJSONArray(raw json.RawMessage) []OpenClawSkill {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	return mapsToSkillsFromMaps(items)
}

func mapsToSkillsFromMaps(items []map[string]any) []OpenClawSkill {
	var out []OpenClawSkill
	for _, m := range items {
		sk := gatewayMapToSkill(m)
		if sk.Slug == "" && sk.Name == "" {
			continue
		}
		if sk.Slug == "" {
			sk.Slug = sk.Name
		}
		if sk.Name == "" {
			sk.Name = sk.Slug
		}
		out = append(out, sk)
	}
	return out
}

func gatewayMapToSkill(m map[string]any) OpenClawSkill {
	sk := OpenClawSkill{
		DataSource:  "gateway",
		Slug:        firstString(m, "slug", "skillKey", "key", "id"),
		Name:        firstString(m, "name", "title", "label"),
		Description: firstString(m, "description", "desc", "summary"),
		Version:     firstString(m, "version"),
		AgentID:     firstString(m, "agentId", "agent_id"),
	}
	loc := strings.ToLower(firstString(m, "location", "layer", "source"))
	switch loc {
	case "workspace", "ws", "agent":
		sk.Location = "workspace"
	case "managed", "user", "global", "bundled", "shared", "extra", "plugin":
		sk.Location = "shared"
	default:
		if sk.AgentID != "" {
			sk.Location = "workspace"
		} else {
			sk.Location = "shared"
		}
	}
	if v, ok := m["eligible"].(bool); ok {
		sk.Eligible = &v
	}
	sk.IneligibleReason = firstString(m, "reason", "ineligibleReason", "blockedReason", "ineligible", "gateReason")
	sk.Permission = metadataToPermissionString(m["metadata"])
	if sk.Permission == "" {
		sk.Permission = firstString(m, "permission", "permissions")
	}
	sk.Scope = firstString(m, "scope")
	return sk
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func metadataToPermissionString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return fmt.Sprint(t)
	}
}

func (s *OpenClawSkillsService) listFromDisk() []OpenClawSkill {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return nil
	}
	_ = define.EnsureDataLayout()

	agentNames := map[string]string{}
	if s.agents != nil {
		if list, listErr := s.agents.ListAgents(); listErr == nil {
			for _, a := range list {
				agentNames[strings.ToLower(strings.TrimSpace(a.OpenClawAgentID))] = strings.TrimSpace(a.Name)
			}
		}
	}

	var out []OpenClawSkill
	out = append(out, scanSkillsUnder(filepath.Join(root, "skills"), "shared", "", "", "managed", agentNames)...)

	if bundled, err := openclawruntime.BundledSkillsDir(); err == nil {
		out = append(out, scanSkillsUnder(bundled, "shared", "", "", "bundled", agentNames)...)
	}

	for _, dir := range readSkillExtraDirs(filepath.Join(root, "openclaw.json")) {
		abs := expandPath(dir)
		if abs == "" {
			continue
		}
		out = append(out, scanSkillsUnder(abs, "shared", "", "", "extra", agentNames)...)
	}

	matches, _ := filepath.Glob(filepath.Join(root, "workspace-*"))
	for _, ws := range matches {
		base := filepath.Base(ws)
		agentID := strings.TrimSpace(strings.TrimPrefix(base, "workspace-"))
		key := strings.ToLower(agentID)
		agentName := agentNames[key]
		out = append(out, scanSkillsUnder(filepath.Join(ws, "skills"), "workspace", agentID, agentName, "workspace", agentNames)...)
	}

	return out
}

type openclawConfigSnip struct {
	Skills *struct {
		Load *struct {
			ExtraDirs []string `json:"extraDirs"`
		} `json:"load"`
	} `json:"skills"`
}

func readSkillExtraDirs(configPath string) []string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var snip openclawConfigSnip
	if err := json.Unmarshal(data, &snip); err != nil {
		return nil
	}
	if snip.Skills == nil || snip.Skills.Load == nil {
		return nil
	}
	return snip.Skills.Load.ExtraDirs
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	home, herr := os.UserHomeDir()
	if p == "~" {
		if herr != nil {
			return ""
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		if herr != nil {
			return ""
		}
		return filepath.Clean(filepath.Join(home, strings.TrimPrefix(p, "~/")))
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if herr != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(home, p))
}

// ReadSkillMarkdown returns SKILL.md content for the given skill root.
func (s *OpenClawSkillsService) ReadSkillMarkdown(skillRoot string) (string, error) {
	if err := s.mustBeAllowedSkillRoot(skillRoot); err != nil {
		return "", err
	}
	p := filepath.Join(skillRoot, "SKILL.md")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	return string(data), nil
}

// ListSkillFiles lists files under a skill directory (for preview).
func (s *OpenClawSkillsService) ListSkillFiles(skillRoot string) ([]SkillFileInfo, error) {
	if err := s.mustBeAllowedSkillRoot(skillRoot); err != nil {
		return nil, err
	}
	var files []SkillFileInfo
	err := filepath.Walk(skillRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(skillRoot, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, SkillFileInfo{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortSkillFiles(files)
	return files, nil
}

// ReadSkillFile reads a file under the skill root (relative path uses /).
func (s *OpenClawSkillsService) ReadSkillFile(skillRoot, filePath string) (string, error) {
	if err := s.mustBeAllowedSkillRoot(skillRoot); err != nil {
		return "", err
	}
	rel := filepath.FromSlash(strings.TrimPrefix(strings.TrimSpace(filePath), "/"))
	full := filepath.Join(skillRoot, rel)
	absSkill, err := filepath.Abs(skillRoot)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absSkill && !strings.HasPrefix(absFull, absSkill+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal not allowed")
	}
	data, err := os.ReadFile(absFull)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *OpenClawSkillsService) mustBeAllowedSkillRoot(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	roots, err := s.allowedSkillRoots()
	if err != nil {
		return err
	}
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, absPath)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path outside allowed OpenClaw skill roots")
}

func (s *OpenClawSkillsService) allowedSkillRoots() ([]string, error) {
	var roots []string
	if oc, err := define.OpenClawDataRootDir(); err == nil {
		roots = append(roots, oc)
	}
	if b, err := openclawruntime.BundledSkillsDir(); err == nil {
		roots = append(roots, b)
	}
	if oc, err := define.OpenClawDataRootDir(); err == nil {
		for _, dir := range readSkillExtraDirs(filepath.Join(oc, "openclaw.json")) {
			if abs := expandPath(dir); abs != "" {
				roots = append(roots, abs)
			}
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no skill roots configured")
	}
	return roots, nil
}

func scanSkillsUnder(
	baseDir string,
	location string,
	agentID string,
	agentName string,
	dataSource string,
	_ map[string]string,
) []OpenClawSkill {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	var out []OpenClawSkill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillRoot := filepath.Join(baseDir, name)
		skillMd := filepath.Join(skillRoot, "SKILL.md")
		if _, statErr := os.Stat(skillMd); statErr != nil {
			continue
		}
		raw, readErr := os.ReadFile(skillMd)
		if readErr != nil {
			continue
		}
		meta := parseOpenClawSkillFrontmatter(string(raw))
		sk := OpenClawSkill{
			Slug:        name,
			Name:        meta.Name,
			Description: meta.Description,
			Version:     meta.Version,
			Permission:  meta.PermissionSummary(),
			Scope:       meta.Scope,
			Location:    location,
			DataSource:  dataSource,
			AgentID:     agentID,
			AgentName:   agentName,
			SkillRoot:   skillRoot,
		}
		if sk.Name == "" {
			sk.Name = sk.Slug
		}
		out = append(out, sk)
	}
	return out
}

type openClawSkillMeta struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Version     string      `yaml:"version"`
	Permission  string      `yaml:"permission"`
	Permissions interface{} `yaml:"permissions"`
	Scope       string      `yaml:"scope"`
}

func (m *openClawSkillMeta) PermissionSummary() string {
	if strings.TrimSpace(m.Permission) != "" {
		return strings.TrimSpace(m.Permission)
	}
	if m.Permissions == nil {
		return ""
	}
	switch v := m.Permissions.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		var parts []string
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(v, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func parseOpenClawSkillFrontmatter(data string) openClawSkillMeta {
	data = strings.TrimSpace(data)
	const delim = "---"
	if !strings.HasPrefix(data, delim) {
		return openClawSkillMeta{}
	}
	rest := data[len(delim):]
	endIdx := strings.Index(rest, "\n"+delim)
	if endIdx == -1 {
		return openClawSkillMeta{}
	}
	front := strings.TrimSpace(rest[:endIdx])
	var meta openClawSkillMeta
	if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
		return openClawSkillMeta{}
	}
	return meta
}

func sortSkillFiles(files []SkillFileInfo) {
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if skillFileLess(files[i].Path, files[j].Path) > 0 {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

func skillFileLess(a, b string) int {
	if a == "SKILL.md" {
		return -1
	}
	if b == "SKILL.md" {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
