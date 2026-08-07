import DOMPurify from 'dompurify'

/**
 * Renders a subset of Markdown to sanitized HTML.
 * Supports: headers, bold, italic, lists, tables, math blocks.
 */
export function renderMarkdown(text: string): string {
  if (!text) return ''

  // Escape HTML first
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

  // Block math $$...$$ -> centered block
  html = html.replace(/\$\$([^$]+)\$\$/g, '<div class="math-block">$1</div>')
  // Inline math $...$ -> code-styled span
  html = html.replace(/\$([^$]+)\$/g, '<code class="math-inline">$1</code>')

  // Tables: convert | ... | blocks to HTML tables
  html = html.replace(/((?:^\|.+\|$\n?)+)/gm, (block: string) => {
    const lines = block.trim().split('\n').filter(l => l.includes('|'))
    if (lines.length < 2) return block
    // Skip separator line (|---|---|)
    const rows = lines.filter(l => !/^\|[\s\-:|]+\|$/.test(l))
    if (rows.length < 1) return block
    const makeCells = (row: string, tag: string) =>
      row.split('|').filter((_, i, a) => i > 0 && i < a.length - 1)
        .map(c => '<' + tag + '>' + c.trim() + '</' + tag + '>').join('')
    const head = '<thead><tr>' + makeCells(rows[0], 'th') + '</tr></thead>'
    const body = '<tbody>' + rows.slice(1).map(r => '<tr>' + makeCells(r, 'td') + '</tr>').join('') + '</tbody>'
    return '<table>' + head + body + '</table>'
  })

  // Headers, blockquotes, bold, italic, lists, paragraphs
  html = html.replace(/^### (.+)$/gm, '<h4>$1</h4>')
  html = html.replace(/^## (.+)$/gm, '<h3>$1</h3>')
  html = html.replace(/^# (.+)$/gm, '<h2>$1</h2>')
  html = html.replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
  html = html.replace(/^- (.+)$/gm, '<li>$1</li>')
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, '<ul>$1</ul>')
  html = html.replace(/\n\n/g, '</p><p>')
  html = '<p>' + html + '</p>'
  html = html.replace(/<p>\s*<\/p>/g, '')
  // Don't wrap tables in <p>
  html = html.replace(/<p><table>/g, '<table>')
  html = html.replace(/<\/table><\/p>/g, '</table>')

  return DOMPurify.sanitize(html)
}
