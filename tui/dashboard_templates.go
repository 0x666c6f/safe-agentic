package main

// dashboardHTML contains the Go HTML templates for the web dashboard.
// Two templates: "index" (agent grid) and "detail" (single agent with live logs).
const dashboardHTML = `
{{define "index"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>safe-agentic dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #0d1117; color: #c9d1d9; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; font-size: 14px; padding: 24px; }
  h1 { font-size: 20px; font-weight: 600; margin-bottom: 16px; color: #f0f6fc; }
  h1 span { color: #8b949e; font-weight: 400; font-size: 14px; margin-left: 8px; }
  a { color: #58a6ff; text-decoration: none; }
  a:hover { text-decoration: underline; }
  table { width: 100%; border-collapse: collapse; }
  th { text-align: left; padding: 8px 12px; border-bottom: 1px solid #30363d; color: #8b949e; font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; }
  td { padding: 8px 12px; border-bottom: 1px solid #21262d; }
  tr:hover td { background: #161b22; }
  .status-running { color: #3fb950; }
  .status-exited { color: #f85149; }
  .activity-working { color: #3fb950; }
  .activity-idle { color: #d29922; }
  .activity-stopped { color: #f85149; }
  .btn-stop { background: #da3633; color: #fff; border: none; padding: 4px 12px; border-radius: 6px; cursor: pointer; font-size: 12px; }
  .btn-stop:hover { background: #f85149; }
  .btn-stop:disabled { opacity: 0.5; cursor: not-allowed; }
  .empty { color: #8b949e; padding: 40px; text-align: center; }
  .refresh-note { color: #484f58; font-size: 12px; margin-top: 12px; }
</style>
</head>
<body>
<h1>safe-agentic <span>dashboard</span></h1>
{{if .}}
<table>
<thead>
<tr>
  <th>Name</th>
  <th>Type</th>
  <th>Repo</th>
  <th>Status</th>
  <th>Activity</th>
  <th>CPU</th>
  <th>Memory</th>
  <th>PIDs</th>
  <th></th>
</tr>
</thead>
<tbody>
{{range .}}
<tr>
  <td><a href="/agents/{{.Name}}">{{.Name}}</a></td>
  <td>{{.Type}}</td>
  <td>{{.Repo}}</td>
  <td class="{{if .Running}}status-running{{else}}status-exited{{end}}">{{.Status}}</td>
  <td class="{{if eq .Activity "Working"}}activity-working{{else if eq .Activity "Idle"}}activity-idle{{else}}activity-stopped{{end}}">{{.Activity}}</td>
  <td>{{.CPU}}</td>
  <td>{{.Memory}}</td>
  <td>{{.PIDs}}</td>
  <td>{{if .Running}}<button class="btn-stop" onclick="stopAgent('{{.Name}}', this)">Stop</button>{{end}}</td>
</tr>
{{end}}
</tbody>
</table>
{{else}}
<div class="empty">No agents running. Use <code>agent spawn</code> to start one.</div>
{{end}}
<p class="refresh-note">Auto-refreshes every 5 seconds</p>
<script>
setTimeout(function(){ location.reload(); }, 5000);
function stopAgent(name, btn) {
  btn.disabled = true;
  btn.textContent = '...';
  fetch('/api/agents/stop/' + encodeURIComponent(name), {method: 'POST'})
    .then(function(){ location.reload(); })
    .catch(function(){ btn.disabled = false; btn.textContent = 'Stop'; });
}
</script>
</body>
</html>
{{end}}

{{define "detail"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Name}} - safe-agentic</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #0d1117; color: #c9d1d9; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; font-size: 14px; padding: 24px; }
  a { color: #58a6ff; text-decoration: none; }
  a:hover { text-decoration: underline; }
  h1 { font-size: 20px; font-weight: 600; margin-bottom: 4px; color: #f0f6fc; }
  .breadcrumb { margin-bottom: 16px; font-size: 13px; color: #8b949e; }
  .meta { display: flex; gap: 24px; margin-bottom: 16px; flex-wrap: wrap; }
  .meta-item { font-size: 13px; }
  .meta-label { color: #8b949e; }
  .status-running { color: #3fb950; }
  .status-exited { color: #f85149; }
  #log-output { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace; font-size: 13px; line-height: 1.5; white-space: pre-wrap; word-wrap: break-word; min-height: 300px; max-height: 70vh; overflow-y: auto; color: #e6edf3; }
  .log-status { color: #484f58; font-size: 12px; margin-top: 8px; }
</style>
</head>
<body>
<div class="breadcrumb"><a href="/">dashboard</a> / {{.Name}}</div>
<h1>{{.Name}}</h1>
<div class="meta">
  <div class="meta-item"><span class="meta-label">Type:</span> {{.Type}}</div>
  <div class="meta-item"><span class="meta-label">Repo:</span> {{.Repo}}</div>
  <div class="meta-item"><span class="meta-label">Status:</span> <span class="{{if .Running}}status-running{{else}}status-exited{{end}}">{{.Status}}</span></div>
  <div class="meta-item"><span class="meta-label">CPU:</span> {{.CPU}}</div>
  <div class="meta-item"><span class="meta-label">Memory:</span> {{.Memory}}</div>
  <div class="meta-item"><span class="meta-label">SSH:</span> {{.SSH}}</div>
  <div class="meta-item"><span class="meta-label">Network:</span> {{.NetworkMode}}</div>
</div>
<div id="log-output">Connecting to log stream...</div>
<p class="log-status" id="log-status">Streaming logs via SSE</p>
<script>
(function() {
  var output = document.getElementById('log-output');
  var status = document.getElementById('log-status');
  var es = new EventSource('/agents/{{.Name}}/logs');
  es.onmessage = function(e) {
    var text = e.data.replace(/\\n/g, '\n');
    output.textContent = text;
    output.scrollTop = output.scrollHeight;
    status.textContent = 'Last update: ' + new Date().toLocaleTimeString();
  };
  es.onerror = function() {
    status.textContent = 'Stream disconnected. Retrying...';
  };
})();
</script>
</body>
</html>
{{end}}
`
