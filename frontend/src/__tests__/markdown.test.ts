import { describe, it, expect } from 'vitest'
import { renderMarkdown } from '../utils/markdown'

describe('renderMarkdown', () => {
  it('returns empty string for empty input', () => {
    expect(renderMarkdown('')).toBe('')
  })

  it('escapes raw HTML before applying markdown rules', () => {
    const out = renderMarkdown('<script>alert(1)</script>')
    expect(out).not.toContain('<script>')
  })

  it('renders blockquotes from "> " lines', () => {
    // This is the review-draft disclaimer's own syntax
    // (see internal/app/app_review.go reviewDisclaimer).
    const out = renderMarkdown('> **Disclaimer:** verify everything.')
    expect(out).toContain('<blockquote>')
    expect(out).toContain('</blockquote>')
    expect(out).toContain('Disclaimer')
  })

  it('renders headers at three levels', () => {
    expect(renderMarkdown('# H1')).toContain('<h2>H1</h2>')
    expect(renderMarkdown('## H2')).toContain('<h3>H2</h3>')
    expect(renderMarkdown('### H3')).toContain('<h4>H3</h4>')
  })

  it('renders bold and italic', () => {
    expect(renderMarkdown('**bold**')).toContain('<strong>bold</strong>')
    expect(renderMarkdown('*italic*')).toContain('<em>italic</em>')
  })

  it('renders a bullet list', () => {
    const out = renderMarkdown('- one\n- two')
    expect(out).toContain('<ul>')
    expect(out).toContain('<li>one</li>')
    expect(out).toContain('<li>two</li>')
  })

  it('renders a table without wrapping it in <p>', () => {
    const md = '| A | B |\n|---|---|\n| 1 | 2 |'
    const out = renderMarkdown(md)
    expect(out).toContain('<table>')
    expect(out).toContain('<thead><tr><th>A</th><th>B</th></tr></thead>')
    expect(out).toContain('<td>1</td>')
    expect(out).not.toContain('<p><table>')
  })

  it('renders block and inline math', () => {
    expect(renderMarkdown('$$x^2$$')).toContain('class="math-block"')
    expect(renderMarkdown('inline $x$ math')).toContain('class="math-inline"')
  })

  it('drops empty paragraphs', () => {
    const out = renderMarkdown('text\n\n\n\nmore text')
    expect(out).not.toContain('<p></p>')
  })

  it('does not match inline math across lines', () => {
    // Two unrelated dollar amounts on different lines are not a formula.
    const out = renderMarkdown('costs $5 per unit\n\nor $10 in bulk')
    expect(out).not.toContain('math-inline')
    expect(out).toContain('$5 per unit')
    expect(out).toContain('$10 in bulk')
  })

  it('still matches inline math within a line', () => {
    expect(renderMarkdown('a $x + y$ b\nnext $z$')).toContain('<code class="math-inline">x + y</code>')
    expect(renderMarkdown('a $x + y$ b\nnext $z$')).toContain('<code class="math-inline">z</code>')
  })
})
