import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const repoRoot = path.resolve(import.meta.dirname, '..', '..')
const frontendLocalesDir = path.join(repoRoot, 'frontend', 'src', 'locales')
const backendLocalesDir = path.join(repoRoot, 'internal', 'services', 'i18n', 'locales')
const writeMissing = process.argv.includes('--write-missing')

function flatten(value, prefix = '', out = {}) {
  for (const [key, nested] of Object.entries(value)) {
    const nextKey = prefix ? `${prefix}.${key}` : key
    if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
      flatten(nested, nextKey, out)
      continue
    }
    out[nextKey] = nested
  }
  return out
}

function readJSON(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'))
}

function writeJSON(filePath, value) {
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`)
}

function readFrontendLocale(filePath) {
  const source = fs.readFileSync(filePath, 'utf8')
  const expression = source
    .replace(/^export default\s*/, '')
    .trim()
    .replace(/;?\s*$/, '')
  return Function(`return (${expression})`)()
}

function writeFrontendLocale(filePath, value) {
  fs.writeFileSync(filePath, `export default ${JSON.stringify(value, null, 2)}\n`)
}

function clone(value) {
  return JSON.parse(JSON.stringify(value))
}

function fallbackBaseFile(file) {
  return file === 'zh-TW.ts' || file === 'zh-TW.json' ? 'zh-CN' : 'en-US'
}

function mergeMissingWithFallback(target, template, fallback) {
  const out = clone(target)
  for (const [key, templateValue] of Object.entries(template)) {
    const targetValue = out[key]
    const fallbackValue = fallback?.[key]
    if (templateValue && typeof templateValue === 'object' && !Array.isArray(templateValue)) {
      out[key] = mergeMissingWithFallback(
        targetValue && typeof targetValue === 'object' && !Array.isArray(targetValue)
          ? targetValue
          : {},
        templateValue,
        fallbackValue && typeof fallbackValue === 'object' && !Array.isArray(fallbackValue)
          ? fallbackValue
          : {}
      )
      continue
    }
    if (!(key in out)) {
      out[key] = key in (fallback ?? {}) ? clone(fallbackValue) : clone(templateValue)
    }
  }
  return out
}

function orderLikeTemplate(template, value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return value
  }

  const ordered = {}
  for (const key of Object.keys(template)) {
    if (!(key in value)) {
      continue
    }
    const templateValue = template[key]
    const valueItem = value[key]
    if (
      templateValue &&
      typeof templateValue === 'object' &&
      !Array.isArray(templateValue) &&
      valueItem &&
      typeof valueItem === 'object' &&
      !Array.isArray(valueItem)
    ) {
      ordered[key] = orderLikeTemplate(templateValue, valueItem)
    } else {
      ordered[key] = valueItem
    }
  }

  for (const key of Object.keys(value).sort()) {
    if (key in ordered) {
      continue
    }
    const valueItem = value[key]
    ordered[key] =
      valueItem && typeof valueItem === 'object' && !Array.isArray(valueItem)
        ? orderLikeTemplate({}, valueItem)
        : valueItem
  }

  return ordered
}

function collectLocaleDiffs({ dir, files, baseFile, loader }) {
  const base = flatten(loader(path.join(dir, baseFile)))
  const baseKeys = new Set(Object.keys(base))

  return files.map((file) => {
    const current = flatten(loader(path.join(dir, file)))
    const currentKeys = new Set(Object.keys(current))
    const missing = [...baseKeys].filter((key) => !currentKeys.has(key)).sort()
    const extra = [...currentKeys].filter((key) => !baseKeys.has(key)).sort()
    return { file, missing, extra }
  })
}

function syncFrontendLocales() {
  const template = readFrontendLocale(path.join(frontendLocalesDir, 'zh-CN.ts'))
  const english = readFrontendLocale(path.join(frontendLocalesDir, 'en-US.ts'))
  for (const file of frontendFiles) {
    if (file === 'zh-CN.ts' || file === 'en-US.ts') {
      continue
    }
    const current = readFrontendLocale(path.join(frontendLocalesDir, file))
    const fallback = fallbackBaseFile(file) === 'zh-CN' ? template : english
    const merged = mergeMissingWithFallback(current, template, fallback)
    const ordered = orderLikeTemplate(template, merged)
    writeFrontendLocale(path.join(frontendLocalesDir, file), ordered)
  }
}

function syncBackendLocales() {
  const template = readJSON(path.join(backendLocalesDir, 'zh-CN.json'))
  const english = readJSON(path.join(backendLocalesDir, 'en-US.json'))
  for (const file of backendFiles) {
    if (file === 'zh-CN.json' || file === 'en-US.json') {
      continue
    }
    const current = readJSON(path.join(backendLocalesDir, file))
    const fallback = fallbackBaseFile(file) === 'zh-CN' ? template : english
    const merged = { ...current }
    for (const [key, value] of Object.entries(template)) {
      if (!(key in merged)) {
        merged[key] = key in fallback ? fallback[key] : value
      }
    }
    const ordered = orderLikeTemplate(template, merged)
    writeJSON(path.join(backendLocalesDir, file), ordered)
  }
}

function printSection(title, diffs) {
  console.log(`\n${title}`)
  let hasMissing = false
  let hasExtra = false
  for (const diff of diffs) {
    if (diff.missing.length === 0 && diff.extra.length === 0) {
      continue
    }
    if (diff.missing.length > 0) {
      hasMissing = true
    }
    if (diff.extra.length > 0) {
      hasExtra = true
    }
    console.log(`- ${diff.file}: missing=${diff.missing.length}, extra=${diff.extra.length}`)
    if (diff.missing.length > 0) {
      console.log(`  missing: ${diff.missing.slice(0, 12).join(', ')}`)
    }
    if (diff.extra.length > 0) {
      console.log(`  extra: ${diff.extra.slice(0, 12).join(', ')}`)
    }
  }

  if (!hasMissing && !hasExtra) {
    console.log('- all locale files are in sync')
  }
  return { hasMissing, hasExtra }
}

const frontendFiles = fs
  .readdirSync(frontendLocalesDir)
  .filter((file) => file.endsWith('.ts') && file !== 'index.ts')
  .sort()
const backendFiles = fs
  .readdirSync(backendLocalesDir)
  .filter((file) => file.endsWith('.json'))
  .sort()

if (writeMissing) {
  syncFrontendLocales()
  syncBackendLocales()
}

const frontendDiffs = collectLocaleDiffs({
  dir: frontendLocalesDir,
  files: frontendFiles,
  baseFile: 'zh-CN.ts',
  loader: readFrontendLocale,
})
const backendDiffs = collectLocaleDiffs({
  dir: backendLocalesDir,
  files: backendFiles,
  baseFile: 'zh-CN.json',
  loader: readJSON,
})

const frontendStatus = printSection('Frontend locales', frontendDiffs)
const backendStatus = printSection('Backend locales', backendDiffs)

const hasMissing = frontendStatus.hasMissing || backendStatus.hasMissing
const hasExtra = frontendStatus.hasExtra || backendStatus.hasExtra

if ((!writeMissing && (hasMissing || hasExtra)) || (writeMissing && hasMissing)) {
  process.exitCode = 1
} else if (!hasMissing && !hasExtra) {
  console.log('\nLocale coverage check passed.')
}
