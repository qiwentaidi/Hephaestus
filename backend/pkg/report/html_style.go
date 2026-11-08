package report

// reportHTMLCoreStyle is the shared visual language for every HTML report.
// Task-specific templates append their own layout classes instead of
// re-declaring fonts, colors, hero, stat cards, and result tables.
const reportHTMLCoreStyle = `body{margin:0;font-family:'Segoe UI',PingFang SC,Microsoft YaHei,sans-serif;background:linear-gradient(180deg,#f3f7ff 0,#f8fafc 100%);color:#111827;line-height:1.6}` +
	`.container{max-width:1320px;margin:0 auto;padding:28px 18px 40px}` +
	`.hero{background:#0f172a;color:#e2e8f0;border-radius:16px;padding:20px 22px;box-shadow:0 14px 28px rgba(2,6,23,.16)}` +
	`.hero h1{margin:0;font-size:26px}` +
	`.hero p{margin:8px 0 0;color:#cbd5e1;font-size:13px}` +
	`.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(128px,1fr));gap:10px;margin:14px 0 18px}` +
	`.stat{background:#fff;border:1px solid #e2e8f0;border-radius:12px;padding:10px 12px}` +
	`.stat .k{font-size:12px;color:#64748b}` +
	`.stat .v{font-size:22px;font-weight:700;color:#0f172a}` +
	`table{width:100%;border-collapse:collapse;font-size:13px}` +
	`th,td{border:1px solid #e2e8f0;padding:8px 9px;vertical-align:top}` +
	`th{background:#f8fafc;text-align:left;color:#334155}` +
	`.mono{font-family:ui-monospace,Consolas,Menlo,monospace;word-break:break-all}` +
	`details{border:1px solid #e2e8f0;border-radius:10px;background:#f8fafc;margin-top:6px}` +
	`summary{cursor:pointer;padding:8px 10px;color:#334155;font-weight:600}` +
	`pre{margin:0;padding:10px;border-top:1px solid #e2e8f0;background:#fff;white-space:pre-wrap;word-break:break-all;max-height:360px;overflow:auto}`

// reportHTMLMarkdownBodyStyle renders delivered Markdown inside the same card
// and typography system used by the HTML reports generated for scan tasks.
const reportHTMLMarkdownBodyStyle = `.report-body{background:#fff;border:1px solid #e2e8f0;border-radius:14px;padding:24px 28px;margin-top:14px}` +
	`.report-body>:first-child{margin-top:0}` +
	`.report-body h1{margin:0 0 12px;font-size:24px;line-height:1.35}` +
	`.report-body h2{margin:24px 0 10px;padding-bottom:8px;border-bottom:1px solid #e2e8f0;font-size:20px;line-height:1.4}` +
	`.report-body h3{margin:20px 0 8px;font-size:17px;line-height:1.4}` +
	`.report-body h4{margin:16px 0 6px;font-size:15px;line-height:1.4}` +
	`.report-body p{margin:10px 0}` +
	`.report-body ul,.report-body ol{margin:10px 0;padding-left:1.6em}` +
	`.report-body li+li{margin-top:4px}` +
	`.report-body strong{color:#0f172a}` +
	`.report-body code{padding:2px 5px;border-radius:5px;background:#f1f5f9;font-family:ui-monospace,Consolas,Menlo,monospace;font-size:12px;word-break:break-word}` +
	`.report-body pre{max-height:none;border:1px solid #e2e8f0;border-radius:10px;background:#0f172a;color:#e2e8f0;overflow:auto}` +
	`.report-body pre code{padding:0;background:transparent;color:inherit;font-size:12px}` +
	`.report-body blockquote{margin:12px 0;padding:6px 14px;border-left:3px solid #cbd5e1;background:#f8fafc;color:#475569}` +
	`.report-body hr{margin:24px 0;border:0;border-top:1px solid #e2e8f0}` +
	`.report-body table{margin:12px 0}` +
	`.report-body a{color:#1d4ed8}` +
	`.report-body img{max-width:100%}` +
	`@media (max-width:720px){.container{padding:16px 10px 28px}.hero{padding:16px 16px}.hero h1{font-size:22px}.report-body{padding:16px 14px}}` +
	`@media print{body{background:#fff}.container{max-width:none;padding:0}.hero{box-shadow:none}.report-body pre,.report-body table{break-inside:avoid}}`
