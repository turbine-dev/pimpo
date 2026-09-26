import { chromium } from '@playwright/test'
// Renders an icon SVG to PNG: node scripts/render-icon.mjs in.svg out.png size (absolute paths).
import { readFileSync } from 'node:fs'
// node render.mjs in.svg out.png size
const [svg, out, size] = process.argv.slice(2)
const data = 'data:image/svg+xml;base64,' + readFileSync(svg).toString('base64')
const b = await chromium.launch()
const p = await b.newPage({ viewport: { width: +size, height: +size }, deviceScaleFactor: 1 })
await p.setContent(`<html><body style="margin:0;background:transparent"><img src="${data}" width="${size}" height="${size}" style="display:block"></body></html>`)
await p.waitForTimeout(300)
await p.screenshot({ path: out, omitBackground: true })
await b.close()
