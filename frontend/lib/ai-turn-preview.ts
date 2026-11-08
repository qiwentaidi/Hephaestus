/** Plain text excerpts for the conversation rail, never rendered as HTML. */
export function turnPreviewText(value: string, limit = 220): string {
  const text = value
    .replace(/<think\b[^>]*>[\s\S]*?(?:<\/think>|$)/gi, '')
    .replace(/```[^\n]*\n([\s\S]*?)(?:```|$)/g, '$1')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]*>/g, '')
    .replace(/^\s{0,3}(?:#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+)/gm, '')
    .replace(/(\*\*|__|~~|`)/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  const characters = Array.from(text)
  return characters.length <= limit ? text : `${characters.slice(0, limit - 1).join('').trimEnd()}…`
}

export function turnResultPreview(messages: Array<{ content: string }>, fallback: string): string {
  for (let index = messages.length - 1; index >= 0; index--) {
    const text = turnPreviewText(messages[index].content)
    if (text) return text
  }
  return turnPreviewText(fallback)
}
