import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const root = dirname(dirname(fileURLToPath(import.meta.url)))
const monacoSource = join(root, "node_modules", "monaco-editor", "min", "vs")
const monacoTarget = join(root, "public", "monaco-editor", "vs")
const mapLibreSource = join(root, "node_modules", "maplibre-gl", "dist")
const mapLibreTarget = join(root, "public", "maplibre-gl")

if (!existsSync(monacoSource)) {
  throw new Error(`Monaco assets not found at ${monacoSource}. Run npm install first.`)
}
if (!existsSync(mapLibreSource)) {
  throw new Error(`MapLibre assets not found at ${mapLibreSource}. Run npm install first.`)
}

rmSync(monacoTarget, { force: true, recursive: true })
mkdirSync(dirname(monacoTarget), { recursive: true })
cpSync(monacoSource, monacoTarget, { recursive: true })

rmSync(mapLibreTarget, { force: true, recursive: true })
mkdirSync(mapLibreTarget, { recursive: true })
for (const file of ["maplibre-gl.css", "maplibre-gl-csp.js", "maplibre-gl-csp-worker.js", "LICENSE.txt"]) {
  cpSync(join(mapLibreSource, file), join(mapLibreTarget, file))
}

patchWgslLanguage()
stripSourceMapComments(monacoTarget)

console.log(`Copied Monaco assets to ${monacoTarget}`)
console.log(`Copied MapLibre assets to ${mapLibreTarget}`)

function patchWgslLanguage() {
  const files = readdirSync(monacoTarget).filter((file) => /^wgsl-.*\.js$/.test(file))

  for (const file of files) {
    const path = join(monacoTarget, file)
    const content = readFileSync(path, "utf8")
    const patched = content.replace(
      'new RegExp("[_\\\\p{XID_Start}]\\\\p{XID_Continue}*","u")',
      "/[_A-Za-z][_0-9A-Za-z]*/",
    )

    if (patched !== content) {
      writeFileSync(path, patched)
    }
  }
}

function stripSourceMapComments(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    const stat = statSync(path)

    if (stat.isDirectory()) {
      stripSourceMapComments(path)
      continue
    }

    if (!entry.endsWith(".js")) {
      continue
    }

    const content = readFileSync(path, "utf8")
    const patched = content.replace(/\n?\/\/# sourceMappingURL=.*\.map\s*$/u, "")

    if (patched !== content) {
      writeFileSync(path, patched)
    }
  }
}
