// Client-side SVG → PNG export helpers. Papeer has no charting library, so
// the Year Distribution chart and the Citation Graph are hand-rolled SVG —
// exporting them to PNG (for the report "insert" flow and the Analysis
// Export Bundle) has to happen in the browser: draw the serialized SVG onto
// an offscreen canvas, then either trigger a download or hand back base64
// for SaveAnalysisBundle.

// The app is dark-theme only (see theme/tokens.css --surface-bg), so PNG
// exports use the same background rather than a transparent/white one that
// would make the light-colored edges and labels unreadable.
const EXPORT_BACKGROUND = '#0f172a'

function svgToDataUrl(svg: SVGSVGElement, width: number, height: number): string {
  const clone = svg.cloneNode(true) as SVGSVGElement
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  clone.setAttribute('width', String(width))
  clone.setAttribute('height', String(height))
  const source = new XMLSerializer().serializeToString(clone)
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(source)
}

async function svgToCanvas(svg: SVGSVGElement, scale = 2): Promise<HTMLCanvasElement> {
  const width = svg.clientWidth || svg.viewBox?.baseVal?.width || 800
  const height = svg.clientHeight || svg.viewBox?.baseVal?.height || 600
  const url = svgToDataUrl(svg, width, height)

  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(width * scale))
  canvas.height = Math.max(1, Math.round(height * scale))
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas 2d context unavailable')
  ctx.fillStyle = EXPORT_BACKGROUND
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.scale(scale, scale)

  const img = new Image()
  await new Promise<void>((resolve, reject) => {
    img.onload = () => {
      ctx.drawImage(img, 0, 0, width, height)
      resolve()
    }
    img.onerror = () => reject(new Error('failed to rasterize svg'))
    img.src = url
  })
  return canvas
}

/** Renders an SVG element to a PNG and returns it as base64 (no data: prefix),
 * or null if rasterization failed — callers should treat that as "skip the
 * image" rather than a hard error. */
export async function svgToPngBase64(svg: SVGSVGElement, scale = 2): Promise<string | null> {
  try {
    const canvas = await svgToCanvas(svg, scale)
    const dataUrl = canvas.toDataURL('image/png')
    return dataUrl.split(',')[1] ?? null
  } catch {
    return null
  }
}

/** Renders an SVG element to a PNG and triggers a browser download. */
export async function downloadSvgAsPng(svg: SVGSVGElement, filename: string, scale = 2): Promise<void> {
  const canvas = await svgToCanvas(svg, scale)
  const dataUrl = canvas.toDataURL('image/png')
  const a = document.createElement('a')
  a.href = dataUrl
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
}
