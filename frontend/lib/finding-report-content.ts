export type ReportBlock = { kind: 'markdown'; content: string } | { kind: 'code'; content: string; language: string; recorded: boolean }

const requestLine = /^(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|CONNECT|TRACE)\s+\S+/
const jsonLine = /^\s*(?:\{(?=\s*(?:"|\}|$))|\[(?=\s*(?:["{\[\]0-9-]|true\b|false\b|null\b|$))|[\]}][,;]?\s*$|"[^"\n]+"\s*:)/
const formLine = /^[\w.%-]+=[^\s]+(?:&[\w.%-]+=)/

/** Presentation only: recognize explicit fences and unambiguous recorded snippets.
 * Never generate a request, infer missing content, or rewrite stored evidence.
 */
export function findingReportBlocks(source: string): ReportBlock[] {
  const lines = source.replace(/\r\n/g, '\n').split('\n')
  const blocks: ReportBlock[] = []
  let prose: string[] = []
  const flush = () => {
    if (prose.some(line => line.trim())) blocks.push({ kind: 'markdown', content: prose.join('\n') })
    prose = []
  }
  for (let i = 0; i < lines.length; i++) {
    let line = lines[i]
    const fence = line.match(/^\s*(`{3,}|~{3,})\s*([^\s]*)[^\n]*$/)
    if (fence) {
      flush()
      const content: string[] = []
      const close = new RegExp(`^\\s*${fence[1][0]}{${fence[1].length},}\\s*$`)
      while (++i < lines.length && !close.test(lines[i])) content.push(lines[i])
      blocks.push({ kind: 'code', content: content.join('\n'), language: fence[2] || 'text', recorded: false })
      continue
    }
    // Old records use "1) 操作：GET ..." without Markdown structure.
    const step = line.match(/^(\d+)[)）]\s*(.*)$/)
    if (step) {
      flush()
      const request = step[2].search(/(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+(?:https?:\/\/|\/)/)
      if (request > 0) {
        prose.push(`### ${step[1]}. ${step[2].slice(0, request).replace(/[：:\s]+$/, '')}`)
        flush()
        line = step[2].slice(request)
      } else {
        prose.push(`**${step[1]}.** ${step[2]}`)
        continue
      }
    }
    const labeledJSON = line.match(/^([^{}\[\]\n]{1,40}[：:]\s*)([\[{].*)$/)
    if (labeledJSON && jsonLine.test(labeledJSON[2])) {
      prose.push(labeledJSON[1])
      flush()
      line = labeledJSON[2]
    }
    const language = requestLine.test(line.trim()) || /^HTTP\/\d/.test(line.trim()) ? 'http' : jsonLine.test(line) ? 'json' : formLine.test(line.trim()) ? 'text' : ''
    if (language) {
      flush()
      const content = [line]
      if (language === 'json') {
        // Keep excerpts (including ellipses and masking) verbatim, not JSON.parse/stringify.
        while (i + 1 < lines.length && jsonLine.test(lines[i + 1])) content.push(lines[++i])
      } else if (language === 'http') {
        while (i + 1 < lines.length && /^[A-Za-z][A-Za-z0-9-]*:\s*\S/.test(lines[i + 1])) content.push(lines[++i])
      }
      blocks.push({ kind: 'code', content: content.join('\n'), language, recorded: true })
    } else prose.push(line)
  }
  flush()
  return blocks
}
