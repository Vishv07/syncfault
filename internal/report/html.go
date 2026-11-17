package report

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
)

const htmlScorecardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>SyncFault Correctness Scorecard</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 1040px; margin: 2rem auto; padding: 0 1rem; color: #1f2937; line-height: 1.5; }
  h1, h2 { color: #111827; border-bottom: 1px solid #e5e7eb; padding-bottom: 0.4rem; }
  table { border-collapse: collapse; width: 100%; margin: 1rem 0 2rem; font-size: 0.95rem; }
  th, td { border: 1px solid #d1d5db; padding: 0.55rem 0.75rem; text-align: left; }
  th { background: #f3f4f6; font-weight: 600; }
  code { background: #f3f4f6; padding: 0.15rem 0.35rem; border-radius: 4px; font-size: 0.9em; }
  .badge-loss { color: #991b1b; font-weight: 700; }
  .badge-clean { color: #065f46; font-weight: 600; }
  blockquote { background: #f9fafb; border-left: 4px solid #6b7280; margin: 1rem 0; padding: 0.75rem 1rem; }
</style>
</head>
<body>
  <h1>SyncFault Correctness Scorecard</h1>
  <p><strong>Total Seeded Runs:</strong> {{.TotalRuns}} &nbsp;|&nbsp; <strong>Finding Reproducibility:</strong> {{.Reproducibility}} &nbsp;|&nbsp; <strong>Severity Order:</strong> <code>DATA_LOSS &gt; WRONG_BYTES &gt; DUPLICATE &gt; STALL &gt; CLEAN</code></p>

  <h2>1. Target &times; Verdict Matrix</h2>
  <table>
    <thead>
      <tr>
        <th>Target</th><th>Version</th><th>Runs</th><th>DATA_LOSS</th><th>WRONG_BYTES</th><th>DUPLICATE</th><th>STALL</th><th>CLEAN</th><th>FLAKY</th>
      </tr>
    </thead>
    <tbody>
      {{range .Targets}}
      <tr>
        <td><code>{{.Adapter}}</code></td>
        <td>{{.Version}}</td>
        <td>{{.Total}}</td>
        <td class="badge-loss">{{.DataLoss}}</td>
        <td>{{.WrongBytes}}</td>
        <td>{{.Duplicate}}</td>
        <td>{{.Stall}}</td>
        <td class="badge-clean">{{.Clean}}</td>
        <td>{{.Flaky}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>

  <h2>2. Per-Fault-Class Breakdown</h2>
  <table>
    <thead>
      <tr>
        <th>Target</th><th>Fault</th><th>Runs</th><th>CLEAN</th><th>Non-CLEAN</th><th>Avg Convergence (ms)</th><th>Bytes Transferred</th><th>Reproducibility</th>
      </tr>
    </thead>
    <tbody>
      {{range .FaultBreakdown}}
      <tr>
        <td><code>{{.Adapter}}</code></td>
        <td><code>{{.FaultType}}</code></td>
        <td>{{.Runs}}</td>
        <td>{{.Clean}}</td>
        <td>{{.NonClean}}</td>
        <td>{{.AvgDurationMS}}</td>
        <td>{{.TotalBytes}}</td>
        <td>{{.Reproducibility}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>

  <h2>3. Quiescence Rule</h2>
  <blockquote>{{.QuiescenceRule}}</blockquote>

  <h2>4. Limitations</h2>
  <ul>
    <li><strong>No VM/power-loss page-cache destruction:</strong> v1 injects process (<code>sigkill</code>), network (<code>netcut</code>/<code>netslow</code>), and filesystem quota (<code>enospc</code>) faults.</li>
    <li><strong>Two-replica topology only:</strong> Multi-node mesh graphs are scheduled for v2.</li>
    <li><strong>Not a speed benchmark:</strong> Convergence time is a diagnostic secondary metric only.</li>
  </ul>
</body>
</html>
`

// RenderHTML renders a self-contained HTML scorecard report with zero external assets (§7.10).
func RenderHTML(data ScorecardData) (string, error) {
	tmpl, err := template.New("scorecard").Parse(htmlScorecardTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// GenerateAll writes both scorecard.md and index.html into outDir.
func GenerateAll(outDir string, runsData ScorecardData) error {
	if _, err := WriteMarkdown(outDir, runsData); err != nil {
		return err
	}
	htmlStr, err := RenderHTML(runsData)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "index.html"), []byte(htmlStr), 0o644)
}
