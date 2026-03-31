package openclawruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BuiltinToolCatalog struct {
	RuntimeVersion string               `json:"runtime_version"`
	Sections       []BuiltinToolSection `json:"sections"`
	Profiles       []BuiltinToolProfile `json:"profiles"`
	Groups         []BuiltinToolGroup   `json:"groups"`
}

type BuiltinToolSection struct {
	ID    string            `json:"id"`
	Label string            `json:"label"`
	Tools []BuiltinToolInfo `json:"tools"`
}

type BuiltinToolInfo struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Profiles    []string `json:"profiles"`
}

type BuiltinToolProfile struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	ToolIDs []string `json:"tool_ids"`
}

type BuiltinToolGroup struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	ToolIDs []string `json:"tool_ids"`
}

func (s *OpenClawRuntimeService) GetBuiltinToolCatalog() (*BuiltinToolCatalog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return loadBuiltinToolCatalog(ctx)
}

func loadBuiltinToolCatalog(ctx context.Context) (*BuiltinToolCatalog, error) {
	bundle, err := resolveBundledRuntime()
	if err != nil {
		return nil, fmt.Errorf("resolve bundled runtime: %w", err)
	}

	nodePath := filepath.Join(bundle.Root, "tools", "node", "bin", "node")
	if _, err := os.Stat(nodePath); err != nil {
		return nil, fmt.Errorf("bundled node missing at %s: %w", nodePath, err)
	}

	packageRoot := filepath.Join(bundle.Root, "lib", "node_modules", "openclaw")
	if stat, err := os.Stat(packageRoot); err != nil || !stat.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return nil, fmt.Errorf("openclaw package root missing at %s: %w", packageRoot, err)
	}

	distDir := filepath.Join(packageRoot, "dist")
	if stat, err := os.Stat(distDir); err != nil || !stat.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return nil, fmt.Errorf("openclaw dist directory missing at %s: %w", distDir, err)
	}

	script := strings.TrimSpace(`
import fs from 'node:fs/promises'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

const distDir = process.env.OPENCLAW_DIST_DIR
const runtimeVersion = process.env.OPENCLAW_RUNTIME_VERSION || ''

if (!distDir) {
  throw new Error('OPENCLAW_DIST_DIR is not set')
}

const entries = await fs.readdir(distDir)
const toolPolicyFile = entries.find((name) => /^tool-policy-.*\.js$/.test(name))
if (!toolPolicyFile) {
  throw new Error('tool-policy dist module not found')
}

const modulePath = path.join(distDir, toolPolicyFile)
const source = await fs.readFile(modulePath, 'utf8')
const exportMatch = source.match(/export\s*\{([^}]*)\}/s)
if (!exportMatch) {
  throw new Error('tool-policy export map not found')
}

const exportMap = Object.fromEntries(
  exportMatch[1]
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => {
      const match = part.match(/^([A-Za-z0-9_$]+)\s+as\s+([A-Za-z0-9_$]+)$/)
      return match ? [match[1], match[2]] : [part, part]
    })
)

const mod = await import(pathToFileURL(modulePath).href)

const listCoreToolSections = mod[exportMap.listCoreToolSections]
const resolveCoreToolProfiles = mod[exportMap.resolveCoreToolProfiles]
const expandToolGroups = mod[exportMap.expandToolGroups]
const resolveToolProfilePolicy = mod[exportMap.resolveToolProfilePolicy]
const profileOptions = mod[exportMap.PROFILE_OPTIONS]

if (typeof listCoreToolSections !== 'function') {
  throw new Error('listCoreToolSections export is unavailable')
}
if (typeof resolveCoreToolProfiles !== 'function') {
  throw new Error('resolveCoreToolProfiles export is unavailable')
}
if (typeof expandToolGroups !== 'function') {
  throw new Error('expandToolGroups export is unavailable')
}
if (!Array.isArray(profileOptions)) {
  throw new Error('PROFILE_OPTIONS export is unavailable')
}

const sections = listCoreToolSections().map((section) => ({
  id: section.id,
  label: section.label,
  tools: section.tools.map((tool) => ({
    id: tool.id,
    label: tool.label,
    description: tool.description ?? '',
    profiles: resolveCoreToolProfiles(tool.id),
  })),
}))

const allToolIds = Array.from(
  new Set(sections.flatMap((section) => section.tools.map((tool) => tool.id)))
)

const profileToolIds = (profileId) => {
  const resolved =
    typeof resolveToolProfilePolicy === 'function' ? resolveToolProfilePolicy(profileId) : undefined
  const allow = Array.isArray(resolved?.allow) ? resolved.allow : []
  return profileId === 'full' ? allToolIds : Array.from(new Set(allow))
}

const groups = [
  {
    id: 'group:openclaw',
    label: 'OpenClaw',
    tool_ids: expandToolGroups(['group:openclaw']),
  },
  ...sections.map((section) => ({
    id: 'group:' + section.id,
    label: section.label,
    tool_ids: expandToolGroups(['group:' + section.id]),
  })),
]

const profiles = profileOptions.map((profile) => ({
  id: profile.id,
  label: profile.label,
  tool_ids: profileToolIds(profile.id),
}))

process.stdout.write(
  JSON.stringify({
    runtime_version: runtimeVersion,
    sections,
    profiles,
    groups,
  })
)
`)

	cmd := exec.CommandContext(ctx, nodePath, "--input-type=module", "-e", script)
	cmd.Dir = packageRoot
	cmd.Env = append(os.Environ(),
		"OPENCLAW_DIST_DIR="+distDir,
		"OPENCLAW_RUNTIME_VERSION="+bundle.Manifest.OpenClawVersion,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("load builtin tool catalog via bundled node: %w\n%s", err, string(out))
	}

	var catalog BuiltinToolCatalog
	if err := json.Unmarshal(out, &catalog); err != nil {
		return nil, fmt.Errorf("decode builtin tool catalog: %w", err)
	}
	return &catalog, nil
}
