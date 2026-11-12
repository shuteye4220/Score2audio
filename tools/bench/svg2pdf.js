// Prints engraved SVG pages to one PDF with Chromium, which draws the
// music font embedded in the SVG. usage: node svg2pdf.js OUT.pdf PAGE1.svg PAGE2.svg ...
const { chromium } = require('playwright');
const fs = require('fs');
(async () => {
  const [out, ...pages] = process.argv.slice(2);
  const body = pages.map((p) => `<div class="page">${fs.readFileSync(p, 'utf8')}</div>`).join('');
  const html = `<!doctype html><meta charset="utf-8"><style>@page{size:A4;margin:0}body{margin:0}
    .page{width:210mm;height:297mm;page-break-after:always;overflow:hidden}.page svg{width:210mm;height:297mm}</style>${body}`;
  const b = await chromium.launch();
  const page = await b.newPage();
  await page.setContent(html, { waitUntil: 'load' });
  await page.pdf({ path: out, format: 'A4', printBackground: true, margin: { top: 0, bottom: 0, left: 0, right: 0 } });
  await b.close();
})();
