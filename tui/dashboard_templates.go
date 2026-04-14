package main

// dashboardHTML contains the Go HTML template for the browser dashboard.
const dashboardHTML = `
{{define "app"}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>safe-agentic dashboard</title>
<style>
  @font-face {
    font-family: "FKGrotesk";
    src: url("/dashboard-assets/fonts/FKGrotesk.ttf") format("truetype");
    font-display: swap;
  }
  @font-face {
    font-family: "FKGrotesk SemiMono";
    src: url("/dashboard-assets/fonts/FKGroteskSemiMono-Regular.ttf") format("truetype");
    font-display: swap;
  }
  @font-face {
    font-family: "FKGrotesk SemiMono";
    src: url("/dashboard-assets/fonts/FKGroteskSemiMono-Medium.ttf") format("truetype");
    font-display: swap;
    font-weight: 500;
  }
  :root {
    --morpho-neutral-900: #15181A;
    --morpho-neutral-700: #222529;
    --morpho-neutral-500: #383B3E;
    --morpho-neutral-300: #6F7174;
    --morpho-neutral-100: #9C9D9F;
    --morpho-white: #FFFFFF;
    --morpho-blue-dark: #2973FF;
    --morpho-blue-medium: #5792FF;
    --morpho-blue-light: #C4DAFF;
    --bg: var(--morpho-neutral-900);
    --panel: rgba(34, 37, 41, 0.82);
    --panel-2: rgba(44, 49, 54, 0.94);
    --border: rgba(255,255,255,0.09);
    --text: rgba(255,255,255,0.92);
    --muted: rgba(255,255,255,0.68);
    --accent: var(--morpho-blue-medium);
    --accent-2: var(--morpho-blue-dark);
    --warn: #f5b66f;
    --danger: #ff7b8c;
    --mono: "SFMono-Regular", ui-monospace, Menlo, Consolas, monospace;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    background:
      radial-gradient(circle at top left, rgba(87, 146, 255, 0.14), transparent 28%),
      radial-gradient(circle at top right, rgba(41, 115, 255, 0.14), transparent 32%),
      linear-gradient(180deg, rgba(34, 37, 41, 0.98), rgba(21, 24, 26, 0.98));
    color: var(--text);
    font: 14px/1.45 "FKGrotesk", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    min-height: 100vh;
  }
  button, input, select, textarea {
    font: inherit;
  }
  .shell {
    display: grid;
    grid-template-rows: auto 1fr;
    min-height: 100vh;
  }
  .topbar {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 16px;
    padding: 20px 24px 16px;
    border-bottom: 1px solid rgba(255,255,255,0.06);
    backdrop-filter: blur(12px);
    position: sticky;
    top: 0;
    z-index: 5;
    background: rgba(21, 24, 26, 0.92);
  }
  .title h1 {
    margin: 0;
    font-size: 28px;
    font-weight: 300;
    letter-spacing: -0.03em;
  }
  .title p {
    margin: 6px 0 0;
    color: var(--muted);
  }
  .top-actions, .button-row, .tab-row, .quick-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .layout {
    display: grid;
    grid-template-columns: 260px minmax(0, 1fr);
    gap: 20px;
    padding: 20px 24px 24px;
  }
  .sidebar {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .menu {
    display: grid;
    gap: 8px;
  }
  .menu-btn {
    width: 100%;
    text-align: left;
    padding: 12px 14px;
  }
  .menu-btn small {
    display: block;
    margin-top: 4px;
    color: var(--muted);
    font-size: 11px;
  }
  .page-stack {
    display: grid;
    gap: 20px;
  }
  .page {
    display: none;
  }
  .page.active {
    display: block;
  }
  .panel {
    background: linear-gradient(180deg, var(--panel), var(--panel-2));
    border: 1px solid var(--border);
    border-radius: 24px;
    box-shadow: 0 30px 90px rgba(0,0,0,0.34);
    overflow: hidden;
  }
  .panel.info-panel {
    background: linear-gradient(180deg, rgba(34, 37, 41, 0.82), rgba(29, 33, 38, 0.96));
  }
  .panel.action-panel {
    background: linear-gradient(180deg, rgba(22, 31, 49, 0.94), rgba(18, 26, 40, 0.98));
    border-color: rgba(87, 146, 255, 0.28);
    box-shadow:
      0 30px 90px rgba(0,0,0,0.34),
      inset 0 0 0 1px rgba(87, 146, 255, 0.08);
  }
  .panel.action-panel .panel-head {
    background: linear-gradient(90deg, rgba(41, 115, 255, 0.08), rgba(87, 146, 255, 0.03));
  }
  .panel-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 16px 18px 12px;
    border-bottom: 1px solid rgba(255,255,255,0.06);
  }
  .panel-head h2, .panel-head h3 {
    margin: 0;
    font-size: 15px;
    letter-spacing: 0.02em;
  }
  .panel-head h2,
  .panel-head h3,
  .catalog-group-title,
  .card .label,
  .statusline,
  .menu-btn small {
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
  }
  .panel-body {
    padding: 16px 18px 18px;
  }
  .agent-filter {
    width: 100%;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: rgba(8, 12, 24, 0.72);
    color: var(--text);
  }
  .agent-table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 14px;
    font-size: 13px;
  }
  .agent-table th, .agent-table td {
    padding: 10px 12px;
    border-bottom: 1px solid rgba(255,255,255,0.05);
    text-align: left;
    vertical-align: top;
  }
  .agent-table th {
    color: var(--muted);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .agent-table tr {
    cursor: pointer;
    transition: background 120ms ease, transform 120ms ease;
  }
  .agent-table tr:hover td {
    background: rgba(120, 168, 255, 0.08);
  }
  .agent-table tr.selected td {
    background: rgba(98, 212, 168, 0.12);
  }
  .agent-table tr.group-row td {
    background: rgba(87, 146, 255, 0.07);
    color: var(--morpho-blue-light);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 12px;
    letter-spacing: 0.02em;
  }
  .group-toggle {
    appearance: none;
    border: none;
    background: transparent;
    color: inherit;
    cursor: pointer;
    padding: 0;
    font: inherit;
  }
  .tree-cell {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  .muted {
    color: var(--muted);
  }
  .badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 11px;
    border: 1px solid rgba(255,255,255,0.1);
    background: rgba(255,255,255,0.04);
  }
  .success { color: var(--accent); }
  .warning { color: var(--warn); }
  .danger { color: var(--danger); }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(132px, 1fr));
    gap: 10px;
    margin-bottom: 16px;
  }
  .hero-banner {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 14px;
    padding: 16px 18px;
    margin-bottom: 14px;
    border-radius: 18px;
    border: 1px solid rgba(87, 146, 255, 0.18);
    background: linear-gradient(135deg, rgba(41, 115, 255, 0.14), rgba(34, 37, 41, 0.18));
  }
  .hero-banner h3 {
    margin: 0 0 6px;
    font-size: 20px;
    font-weight: 400;
    letter-spacing: -0.02em;
  }
  .hero-banner p {
    margin: 0;
    color: var(--muted);
  }
  .hero-copy {
    display: grid;
    gap: 10px;
    min-width: 0;
    flex: 1 1 auto;
  }
  .hero-copy p {
    word-break: break-word;
  }
  .hero-side {
    display: grid;
    gap: 12px;
    min-width: min(360px, 100%);
    flex: 0 1 360px;
  }
  .hero-badges {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .status-badge {
    display: inline-flex;
    align-items: center;
    padding: 6px 10px;
    border-radius: 999px;
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 11px;
    border: 1px solid rgba(255,255,255,0.12);
    background: rgba(255,255,255,0.05);
  }
  .status-badge.success { border-color: rgba(98,212,168,0.25); background: rgba(98,212,168,0.12); }
  .status-badge.warning { border-color: rgba(245,182,111,0.25); background: rgba(245,182,111,0.1); }
  .status-badge.danger { border-color: rgba(255,123,140,0.25); background: rgba(255,123,140,0.1); }
  .hierarchy-card {
    padding: 12px 14px;
    border-radius: 16px;
    border: 1px solid rgba(87, 146, 255, 0.18);
    background: rgba(9, 15, 28, 0.46);
  }
  .hierarchy-label {
    display: block;
    margin-bottom: 10px;
    color: var(--morpho-blue-light);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .hierarchy-track {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .hierarchy-node {
    display: inline-flex;
    align-items: center;
    min-height: 30px;
    padding: 6px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255,255,255,0.1);
    background: rgba(255,255,255,0.04);
    color: var(--text);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 11px;
  }
  .hierarchy-node.self {
    border-color: rgba(98, 212, 168, 0.3);
    background: rgba(98, 212, 168, 0.14);
    color: #d7fff0;
  }
  .hierarchy-sep {
    color: var(--morpho-blue-light);
    opacity: 0.75;
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 12px;
  }
  .hierarchy-empty {
    color: var(--muted);
    font-size: 13px;
  }
  .agent-overview-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(280px, 0.9fr);
    gap: 14px;
    margin-bottom: 16px;
    align-items: stretch;
  }
  .agent-actions-panel {
    padding: 16px 18px;
    border-radius: 18px;
    border: 1px solid rgba(87, 146, 255, 0.22);
    background: linear-gradient(180deg, rgba(18, 28, 46, 0.96), rgba(12, 19, 32, 0.98));
    box-shadow: inset 0 0 0 1px rgba(87, 146, 255, 0.05);
  }
  .agent-actions-head {
    margin-bottom: 12px;
  }
  .agent-actions-head h3 {
    margin: 0 0 4px;
    font-size: 14px;
    letter-spacing: 0.03em;
  }
  .agent-actions-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
  .agent-action-btn {
    display: grid;
    gap: 3px;
    min-height: 72px;
    padding: 12px 14px;
    border-radius: 14px;
    border-color: rgba(87, 146, 255, 0.22);
    background: linear-gradient(180deg, rgba(87, 146, 255, 0.16), rgba(31, 46, 76, 0.42));
    text-align: left;
    align-content: start;
  }
  .agent-action-btn:hover {
    background: linear-gradient(180deg, rgba(87, 146, 255, 0.24), rgba(35, 53, 86, 0.58));
    border-color: rgba(87, 146, 255, 0.36);
  }
  .agent-action-btn.warn {
    border-color: rgba(255, 123, 140, 0.34);
    background: linear-gradient(180deg, rgba(255, 123, 140, 0.16), rgba(75, 29, 41, 0.52));
  }
  .agent-action-btn.warn:hover {
    background: linear-gradient(180deg, rgba(255, 123, 140, 0.24), rgba(88, 31, 46, 0.64));
  }
  .agent-action-btn strong,
  .agent-action-btn small {
    display: block;
  }
  .agent-action-btn strong {
    font-size: 13px;
  }
  .agent-action-btn small {
    color: var(--muted);
    font-size: 11px;
    line-height: 1.35;
  }
  .info-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }
  .info-block {
    padding: 14px;
    border-radius: 16px;
    border: 1px solid rgba(255,255,255,0.08);
    background: rgba(255,255,255,0.03);
  }
  .info-block h4 {
    margin: 0 0 10px;
    font-size: 12px;
    color: var(--morpho-blue-light);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .info-row {
    display: flex;
    justify-content: space-between;
    gap: 10px;
    padding: 7px 0;
    border-bottom: 1px solid rgba(255,255,255,0.05);
  }
  .info-row:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }
  .info-row span:first-child {
    color: var(--muted);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .info-row span:last-child {
    text-align: right;
    word-break: break-word;
  }
  .card {
    background: rgba(255,255,255,0.03);
    border: 1px solid rgba(255,255,255,0.06);
    border-radius: 12px;
    padding: 12px;
  }
  .card .label {
    display: block;
    color: var(--muted);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    margin-bottom: 6px;
  }
  .card .value {
    word-break: break-word;
    font-size: 13px;
  }
  .btn {
    appearance: none;
    border: 1px solid rgba(255,255,255,0.1);
    background: rgba(255,255,255,0.05);
    color: var(--text);
    border-radius: 10px;
    padding: 9px 12px;
    cursor: pointer;
    transition: transform 120ms ease, background 120ms ease, border-color 120ms ease;
  }
  .btn:hover {
    transform: translateY(-1px);
    background: rgba(255,255,255,0.1);
  }
  .btn.primary {
    background: linear-gradient(135deg, rgba(41, 115, 255, 0.2), rgba(87, 146, 255, 0.24));
    border-color: rgba(87, 146, 255, 0.34);
  }
  .btn.warn {
    border-color: rgba(255, 123, 140, 0.28);
    color: #ffd9df;
  }
  .btn.active {
    background: rgba(87, 146, 255, 0.18);
    border-color: rgba(87, 146, 255, 0.4);
  }
  .content-box, .command-output {
    min-height: 340px;
    max-height: 52vh;
    overflow: auto;
    border-radius: 14px;
    border: 1px solid rgba(255,255,255,0.08);
    background: rgba(8, 12, 24, 0.78);
    padding: 16px;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: var(--mono);
    font-size: 12px;
    line-height: 1.5;
  }
  .content-box {
    background: rgba(10, 14, 22, 0.78);
  }
  .command-output {
    background: linear-gradient(180deg, rgba(12, 18, 30, 0.94), rgba(8, 12, 24, 0.98));
    border-color: rgba(87, 146, 255, 0.2);
  }
  .catalog-layout {
    display: grid;
    grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
    gap: 14px;
    margin-top: 14px;
  }
  .catalog-list {
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 14px;
    background: rgba(8, 12, 24, 0.72);
    max-height: 60vh;
    overflow: auto;
    padding: 10px;
  }
  .catalog-group {
    margin-bottom: 14px;
  }
  .catalog-group-title {
    color: var(--muted);
    font-size: 11px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    margin: 4px 4px 8px;
  }
  .catalog-item {
    display: block;
    width: 100%;
    text-align: left;
    margin-bottom: 6px;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid transparent;
    background: rgba(255,255,255,0.03);
    color: var(--text);
    cursor: pointer;
  }
  .catalog-item:hover {
    border-color: rgba(120, 168, 255, 0.24);
    background: rgba(120, 168, 255, 0.08);
  }
  .catalog-item.active {
    border-color: rgba(98, 212, 168, 0.34);
    background: rgba(98, 212, 168, 0.12);
  }
  .catalog-item strong {
    display: block;
    margin-bottom: 4px;
    font-size: 13px;
  }
  .catalog-item span {
    color: var(--muted);
    font-size: 12px;
  }
  .catalog-detail {
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 14px;
    background: rgba(8, 12, 24, 0.72);
    padding: 14px;
  }
  .catalog-preview {
    margin-top: 12px;
    min-height: 120px;
    max-height: 24vh;
    overflow: auto;
    border-radius: 12px;
    border: 1px solid rgba(255,255,255,0.08);
    background: rgba(255,255,255,0.03);
    padding: 12px;
    font: 12px/1.45 var(--mono);
    white-space: pre-wrap;
    word-break: break-word;
  }
  .details-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 12px;
    margin-top: 16px;
  }
  .global-actions-list {
    grid-template-columns: 1fr;
  }
  .action-stack {
    display: grid;
    gap: 10px;
    margin-top: 12px;
  }
  .action-stack .btn {
    width: 100%;
    justify-content: flex-start;
    text-align: left;
  }
  details {
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 14px;
    background: rgba(255,255,255,0.03);
    padding: 12px 14px;
  }
  .action-card {
    border-color: rgba(87, 146, 255, 0.18);
    background: linear-gradient(180deg, rgba(27, 35, 56, 0.72), rgba(16, 22, 35, 0.9));
  }
  .action-card > summary {
    color: var(--morpho-blue-light);
  }
  .action-card p {
    margin: 8px 0 12px;
    color: var(--muted);
  }
  .action-card .button-row .btn {
    min-width: 150px;
  }
  details > summary {
    cursor: pointer;
    font-weight: 600;
    color: var(--text);
  }
  .form-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 10px;
    margin-top: 12px;
  }
  .form-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .form-field label {
    color: var(--muted);
    font-size: 12px;
  }
  .form-field input, .form-field select, .form-field textarea {
    width: 100%;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: rgba(8, 12, 24, 0.72);
    color: var(--text);
  }
  .form-field textarea {
    min-height: 84px;
    resize: vertical;
  }
  .command-panel {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    margin-top: 12px;
  }
  .statusline {
    color: var(--muted);
    font-size: 12px;
  }
  .loading-pill {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    margin: 10px 0 8px;
    padding: 8px 12px;
    border-radius: 999px;
    border: 1px solid rgba(87, 146, 255, 0.24);
    background: rgba(87, 146, 255, 0.12);
    color: var(--morpho-blue-light);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 12px;
  }
  .loading-pill.hidden {
    display: none;
  }
  .loading-banner {
    position: fixed;
    right: 24px;
    bottom: 24px;
    z-index: 20;
    display: none;
    align-items: center;
    gap: 12px;
    min-width: 260px;
    max-width: 420px;
    padding: 14px 16px;
    border-radius: 16px;
    border: 1px solid rgba(87, 146, 255, 0.32);
    background: linear-gradient(135deg, rgba(18, 26, 40, 0.98), rgba(33, 45, 73, 0.94));
    box-shadow: 0 18px 50px rgba(0,0,0,0.34);
  }
  .loading-banner.visible {
    display: inline-flex;
  }
  .loading-spinner {
    width: 18px;
    height: 18px;
    border-radius: 999px;
    border: 2px solid rgba(196, 218, 255, 0.18);
    border-top-color: var(--morpho-blue-medium);
    animation: spin 0.9s linear infinite;
    flex: 0 0 auto;
  }
  .loading-copy strong {
    display: block;
    margin-bottom: 2px;
    color: var(--morpho-white);
    font-family: "FKGrotesk SemiMono", ui-monospace, Menlo, Consolas, monospace;
    font-size: 12px;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .loading-copy span {
    color: var(--muted);
    font-size: 13px;
  }
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
  .empty {
    color: var(--muted);
    padding: 20px 0;
  }
  @media (max-width: 1100px) {
    .layout {
      grid-template-columns: 1fr;
    }
    .sidebar {
      order: -1;
    }
    .catalog-layout {
      grid-template-columns: 1fr;
    }
    .agent-overview-grid {
      grid-template-columns: 1fr;
    }
    .hero-banner {
      flex-direction: column;
    }
    .hero-side {
      min-width: 0;
      width: 100%;
    }
  }
  @media (max-width: 720px) {
    .agent-actions-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
</head>
<body>
<div class="shell">
  <header class="topbar">
    <div class="title">
      <h1>safe-agentic dashboard</h1>
      <p>Browser control surface for the same agent/actions set as the TUI and CLI. Refreshes every {{.RefreshIntervalS}}s.</p>
    </div>
    <div class="top-actions">
      <button class="btn" id="refresh-btn" type="button">Refresh now</button>
    </div>
  </header>

  <main class="layout">
    <aside class="sidebar">
      <section class="panel info-panel">
        <div class="panel-head">
          <h2>Navigate</h2>
          <div class="statusline" id="fleet-status">Connecting…</div>
        </div>
        <div class="panel-body">
          <div class="menu">
            <button class="btn menu-btn active" data-page="fleet" type="button">Fleet overview<small>Live table, filter, select agent.</small></button>
            <button class="btn menu-btn" data-page="agent" type="button">Selected agent<small id="menu-agent-copy">Open tabs and actions for current selection.</small></button>
            <button class="btn menu-btn" data-page="global" type="button">Global actions<small>Spawn, fleet, pipeline, cleanup, audit, VM.</small></button>
            <button class="btn menu-btn" data-page="cli" type="button">CLI catalog<small>Structured browser forms for every CLI command.</small></button>
          </div>
        </div>
      </section>
    </aside>

    <section class="page-stack">
      <div class="page active" data-page="fleet">
        <section class="panel info-panel">
          <div class="panel-head">
            <h2>Fleet</h2>
            <div class="statusline">Pick an agent to jump to its dedicated page.</div>
          </div>
          <div class="panel-body">
            <input class="agent-filter" id="agent-filter" type="search" placeholder="Filter by name, repo, type, status, network…">
            <table class="agent-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Repo</th>
                  <th>Status</th>
                  <th>Activity</th>
                  <th>CPU</th>
                  <th>Mem</th>
                  <th>PIDs</th>
                </tr>
              </thead>
              <tbody id="agent-rows"></tbody>
            </table>
          </div>
        </section>
      </div>

      <div class="page" data-page="agent">
        <section class="panel info-panel">
          <div class="panel-head">
            <div>
              <h2 id="agent-title">No agent selected</h2>
              <div class="statusline" id="agent-subtitle">Select an agent from the fleet table.</div>
            </div>
          </div>

          <div class="panel-body">
            <div class="agent-overview-grid">
              <div class="hero-banner" id="agent-hero"></div>
              <section class="agent-actions-panel">
                <div class="agent-actions-head">
                  <h3>Agent actions</h3>
                  <div class="statusline">Fast controls, clearly separated from runtime info.</div>
                </div>
                <div class="agent-actions-grid">
                  <button class="btn agent-action-btn" type="button" data-interactive="attach"><strong>Attach cmd</strong><small>Open terminal attach command for current agent.</small></button>
                  <button class="btn agent-action-btn" type="button" data-interactive="resume"><strong>Resume cmd</strong><small>Reconnect to the latest supported session.</small></button>
                  <button class="btn agent-action-btn warn" type="button" data-action="stop"><strong>Stop agent</strong><small>Stop the selected container immediately.</small></button>
                  <button class="btn agent-action-btn" type="button" data-action="checkpoint"><strong>Create checkpoint</strong><small>Capture current workspace state before risky work.</small></button>
                  <button class="btn agent-action-btn" type="button" data-action="sessions"><strong>Export sessions</strong><small>Pull session history for offline inspection.</small></button>
                  <button class="btn agent-action-btn" type="button" data-action="pr"><strong>Create PR</strong><small>Open a pull request from the selected agent branch.</small></button>
                </div>
              </section>
            </div>
            <div class="info-grid" id="agent-cards"></div>

            <div class="tab-row">
              <button class="btn active" data-tab="summary" type="button">Summary</button>
              <button class="btn" data-tab="preview" type="button">Preview</button>
              <button class="btn" data-tab="logs" type="button">Logs</button>
              <button class="btn" data-tab="describe" type="button">Describe</button>
              <button class="btn" data-tab="diff" type="button">Diff</button>
              <button class="btn" data-tab="todo" type="button">Todos</button>
              <button class="btn" data-tab="review" type="button">Review</button>
              <button class="btn" data-tab="cost" type="button">Cost</button>
            </div>
            <div class="loading-pill hidden" id="tab-loading"><span>◌</span><span id="tab-loading-copy">Loading…</span></div>
            <div class="statusline" id="tab-status" style="margin: 2px 0 8px;">Loading summary…</div>
            <pre class="content-box" id="content-box">Loading…</pre>
          </div>
        </section>
      </div>

      <div class="page" data-page="global">
        <section class="panel action-panel">
          <div class="panel-head">
            <h2>Global actions</h2>
            <div class="statusline">VM-level and fleet-level work lives here, separate from agent tabs.</div>
          </div>
          <div class="panel-body">
            <div class="details-grid global-actions-list">
              <details class="action-card" open>
                <summary>Spawn agent</summary>
                <p>Create a fresh containerized Claude or Codex worker with the same launch surface as the CLI.</p>
                <div class="form-grid">
                  <div class="form-field"><label>Type</label><select id="spawn-type"><option>claude</option><option>codex</option></select></div>
                  <div class="form-field"><label>Repo URL</label><input id="spawn-repo" type="text" placeholder="git@github.com:org/repo.git"></div>
                  <div class="form-field"><label>Name</label><input id="spawn-name" type="text" placeholder="optional suffix"></div>
                  <div class="form-field"><label>AWS profile</label><input id="spawn-aws" type="text" placeholder="optional"></div>
                  <div class="form-field"><label>Identity</label><input id="spawn-identity" type="text" placeholder="Name <email>"></div>
                  <div class="form-field"><label>Prompt</label><textarea id="spawn-prompt" placeholder="Optional initial task"></textarea></div>
                </div>
                <div class="button-row" style="margin-top:12px;">
                  <label><input id="spawn-ssh" type="checkbox" checked> SSH</label>
                  <label><input id="spawn-auth" type="checkbox" checked> Reuse auth</label>
                  <label><input id="spawn-gh-auth" type="checkbox"> Reuse GH auth</label>
                  <label><input id="spawn-docker" type="checkbox"> Docker</label>
                </div>
                <div class="action-stack">
                  <button class="btn primary" id="spawn-submit" type="button">Spawn in background</button>
                </div>
              </details>

              <details class="action-card">
                <summary>File transfer</summary>
                <p>Move files both ways between the selected agent container and the VM workspace.</p>
                <div class="form-grid">
                  <div class="form-field"><label>Agent path</label><input id="copy-source" type="text" placeholder="/workspace/..."></div>
                  <div class="form-field"><label>VM path</label><input id="copy-dest" type="text" placeholder="./"></div>
                  <div class="form-field"><label>VM source</label><input id="push-source" type="text" placeholder="./artifact.txt"></div>
                  <div class="form-field"><label>Agent destination</label><input id="push-dest" type="text" placeholder="/workspace/..."></div>
                </div>
                <div class="action-stack">
                  <button class="btn" id="copy-submit" type="button">Copy from selected agent</button>
                  <button class="btn" id="push-submit" type="button">Push to selected agent</button>
                </div>
              </details>

              <details class="action-card">
                <summary>Fleet / pipeline</summary>
                <p>Kick off manifest-driven fan-out or staged workflows without dropping to the terminal.</p>
                <div class="form-grid">
                  <div class="form-field"><label>Fleet manifest</label><input id="fleet-path" type="text" placeholder="examples/fleet.yaml"></div>
                  <div class="form-field"><label>Pipeline manifest</label><input id="pipeline-path" type="text" placeholder="examples/pipeline.yaml"></div>
                </div>
                <div class="action-stack">
                  <button class="btn" id="fleet-submit" type="button">Run fleet</button>
                  <button class="btn" id="pipeline-submit" type="button">Run pipeline</button>
                </div>
              </details>

              <details class="action-card">
                <summary>Interactive helpers</summary>
                <p>Generate terminal-ready commands for flows that should stay attached to a real TTY.</p>
                <div class="form-grid">
                  <div class="form-field">
                    <label>MCP server</label>
                    <input id="mcp-server" type="text" placeholder="linear, notion, github…">
                  </div>
                </div>
                <div class="action-stack">
                  <button class="btn" id="attach-cmd" type="button">Show attach command</button>
                  <button class="btn" id="resume-cmd" type="button">Show resume command</button>
                  <button class="btn" id="mcp-cmd" type="button">Show MCP login command</button>
                </div>
              </details>

              <details class="action-card" open>
                <summary>Global controls</summary>
                <p>Workspace-wide controls and audit views that are not tied to a single agent container.</p>
                <div class="action-stack">
                  <button class="btn" id="audit-btn" type="button">Audit</button>
                  <button class="btn warn" id="kill-all-btn" type="button">Stop all</button>
                </div>
                <div class="statusline" style="margin-top:10px;">Use these for fleet-wide state changes and shared environment operations.</div>
              </details>
            </div>
          </div>
        </section>
      </div>

      <div class="page" data-page="cli">
        <section class="panel action-panel">
          <div class="panel-head" style="padding-bottom: 0; border-bottom:none;">
            <div>
              <h2>CLI workbench</h2>
              <div class="statusline">Two paths: fast ad-hoc runner, or structured forms for the full CLI surface.</div>
            </div>
          </div>
          <div class="panel-body">
            <div>
              <div class="panel-head" style="padding-left:0; padding-right:0; border-bottom:none;">
                <h3>Command runner</h3>
                <span class="statusline">Run any non-interactive <code>safe-ag</code> subcommand.</span>
              </div>
              <div class="command-panel">
                <input class="agent-filter" id="command-input" type="text" placeholder='Examples: summary --latest | cost agent-codex-demo | stop --all'>
                <button class="btn primary" id="command-run" type="button">Run</button>
              </div>
            </div>

            <div style="margin-top: 18px;">
              <div class="panel-head" style="padding-left:0; padding-right:0; border-bottom:none;">
                <h3>CLI catalog</h3>
                <span class="statusline">Structured browser forms for the full CLI surface. Interactive commands return copyable terminal invocations.</span>
              </div>
              <div class="catalog-layout">
                <div class="catalog-list" id="catalog-list"></div>
                <div class="catalog-detail">
                  <h3 id="catalog-title" style="margin:0 0 6px;">Select a command</h3>
                  <div class="statusline" id="catalog-description">Choose a CLI command to render its fields and preview.</div>
                  <div class="form-grid" id="catalog-fields" style="margin-top:14px;"></div>
                  <div class="button-row" style="margin-top:12px;">
                    <button class="btn primary" id="catalog-run" type="button">Run command</button>
                    <button class="btn" id="catalog-copy" type="button">Copy command</button>
                  </div>
                  <pre class="catalog-preview" id="catalog-preview">safe-ag …</pre>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>

      <section class="panel info-panel">
        <div class="panel-head">
          <h2>Activity console</h2>
          <div class="statusline">Shared output surface for agent actions, global actions, and CLI commands.</div>
        </div>
        <div class="panel-body">
          <pre class="command-output" id="command-output">Command output will appear here.</pre>
        </div>
      </section>
    </section>
  </main>
</div>
<div class="loading-banner" id="global-loading">
  <div class="loading-spinner" aria-hidden="true"></div>
  <div class="loading-copy">
    <strong>Working</strong>
    <span id="global-loading-copy">Loading dashboard…</span>
  </div>
</div>

<script>
  const state = {
    agents: {{.InitialAgentsJS}},
    selected: {{printf "%q" .InitialAgent}},
    page: {{if .InitialAgent}}"agent"{{else}}"fleet"{{end}},
    filter: '',
    activeTab: 'summary',
    logsTail: '500',
    contentRequest: 0,
    catalogSelected: 'attach',
    collapsedGroups: {}
  };
  const refreshIntervalMs = {{.RefreshIntervalS}} * 1000;
  const commandCatalog = [
    {id:'attach', group:'agent', title:'attach', description:'Attach to selected agent.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'resume', group:'agent', title:'resume', description:'Resume selected agent session.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'stop', group:'agent', title:'stop', description:'Stop selected agent.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'peek', group:'agent', title:'peek', description:'Peek current output.', fields:[{id:'target', label:'Target', type:'target'},{id:'lines', label:'Lines', type:'text', placeholder:'30'}]},
    {id:'logs', group:'agent', title:'logs', description:'Session logs.', fields:[{id:'target', label:'Target', type:'target'},{id:'lines', label:'Lines', type:'text', placeholder:'50'},{id:'follow', label:'Follow', type:'checkbox'}]},
    {id:'summary', group:'agent', title:'summary', description:'Compact overview.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'output', group:'agent', title:'output', description:'Show output/diff/files/commits/json.', fields:[{id:'target', label:'Target', type:'target'},{id:'diff', label:'Diff', type:'checkbox'},{id:'files', label:'Files', type:'checkbox'},{id:'commits', label:'Commits', type:'checkbox'},{id:'json', label:'JSON', type:'checkbox'}]},
    {id:'diff', group:'agent', title:'diff', description:'Workspace diff.', fields:[{id:'target', label:'Target', type:'target'},{id:'stat', label:'Stat only', type:'checkbox'}]},
    {id:'review', group:'agent', title:'review', description:'AI review of changes.', fields:[{id:'target', label:'Target', type:'target'},{id:'base', label:'Base', type:'text', placeholder:'main'}]},
    {id:'replay', group:'agent', title:'replay', description:'Replay session event log.', fields:[{id:'target', label:'Target', type:'target'},{id:'toolsOnly', label:'Tools only', type:'checkbox'}]},
    {id:'retry', group:'agent', title:'retry', description:'Retry with optional feedback.', fields:[{id:'target', label:'Target', type:'target'},{id:'feedback', label:'Feedback', type:'textarea', placeholder:'Optional guidance'}]},
    {id:'sessions', group:'agent', title:'sessions', description:'Export sessions.', fields:[{id:'target', label:'Target', type:'target'},{id:'dest', label:'Destination', type:'text', placeholder:'optional export path'}]},
    {id:'aws-refresh', group:'agent', title:'aws-refresh', description:'Refresh AWS credentials.', fields:[{id:'target', label:'Target', type:'target'},{id:'profile', label:'Profile', type:'text', placeholder:'optional profile'}]},
    {id:'cost', group:'agent', title:'cost', description:'Estimate session cost.', fields:[{id:'target', label:'Target', type:'target'},{id:'history', label:'History period', type:'text', placeholder:'e.g. 7d (optional)'}]},
    {id:'pr', group:'agent', title:'pr', description:'Create PR.', fields:[{id:'target', label:'Target', type:'target'},{id:'base', label:'Base', type:'text', placeholder:'main'},{id:'title', label:'Title', type:'text', placeholder:'optional title'}]},
    {id:'checkpoint-create', group:'agent', title:'checkpoint create', description:'Create checkpoint.', fields:[{id:'target', label:'Target', type:'target'},{id:'label', label:'Label', type:'text', placeholder:'optional label'}]},
    {id:'checkpoint-list', group:'agent', title:'checkpoint list', description:'List checkpoints.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'checkpoint-revert', group:'agent', title:'checkpoint revert', description:'Revert to checkpoint ref.', fields:[{id:'target', label:'Target', type:'target'},{id:'ref', label:'Ref', type:'text', placeholder:'checkpoint ref'}]},
    {id:'todo-add', group:'agent', title:'todo add', description:'Add merge-gate todo.', fields:[{id:'target', label:'Target', type:'target'},{id:'text', label:'Todo text', type:'textarea', placeholder:'Required'}]},
    {id:'todo-list', group:'agent', title:'todo list', description:'List todos.', fields:[{id:'target', label:'Target', type:'target'}]},
    {id:'todo-check', group:'agent', title:'todo check', description:'Check todo item.', fields:[{id:'target', label:'Target', type:'target'},{id:'index', label:'Index', type:'text', placeholder:'1'}]},
    {id:'todo-uncheck', group:'agent', title:'todo uncheck', description:'Uncheck todo item.', fields:[{id:'target', label:'Target', type:'target'},{id:'index', label:'Index', type:'text', placeholder:'1'}]},
    {id:'mcp-login', group:'agent', title:'mcp-login', description:'Authenticate MCP service for selected agent.', fields:[{id:'service', label:'Service', type:'text', placeholder:'linear'}]},
    {id:'list', group:'global', title:'list', description:'List agents.', fields:[{id:'json', label:'JSON', type:'checkbox'}]},
    {id:'audit', group:'global', title:'audit', description:'Show audit log.', fields:[{id:'lines', label:'Lines', type:'text', placeholder:'50'}]},
    {id:'cleanup', group:'global', title:'cleanup', description:'Cleanup containers/networks.', fields:[{id:'auth', label:'Remove auth volumes', type:'checkbox'}]},
    {id:'setup', group:'global', title:'setup', description:'Initialize VM + image.', fields:[]},
    {id:'update', group:'global', title:'update', description:'Rebuild image.', fields:[{id:'full', label:'Full rebuild', type:'checkbox'},{id:'quick', label:'Quick rebuild', type:'checkbox'}]},
    {id:'diagnose', group:'global', title:'diagnose', description:'Environment health checks.', fields:[]},
    {id:'vm-start', group:'global', title:'vm start', description:'Start VM.', fields:[]},
    {id:'vm-stop', group:'global', title:'vm stop', description:'Stop VM.', fields:[]},
    {id:'vm-ssh', group:'global', title:'vm ssh', description:'Open VM shell.', fields:[]},
    {id:'tui', group:'global', title:'tui', description:'Launch terminal dashboard.', fields:[]},
    {id:'dashboard', group:'global', title:'dashboard', description:'Launch web dashboard.', fields:[{id:'bind', label:'Bind', type:'text', placeholder:'localhost:8420'}]},
    {id:'spawn-cli', group:'global', title:'spawn', description:'CLI spawn form equivalent.', fields:[{id:'agentType', label:'Type', type:'select', options:['claude','codex','shell']},{id:'repo', label:'Repo URL', type:'text', placeholder:'optional'},{id:'name', label:'Name', type:'text', placeholder:'optional'},{id:'prompt', label:'Prompt', type:'textarea', placeholder:'optional'},{id:'ssh', label:'SSH', type:'checkbox'},{id:'reuseAuth', label:'Reuse auth', type:'checkbox'},{id:'reuseGHAuth', label:'Reuse GH auth', type:'checkbox'},{id:'background', label:'Background', type:'checkbox', checked:true}]},
    {id:'run', group:'global', title:'run', description:'Quick spawn wrapper.', fields:[{id:'repo', label:'Repo URL', type:'text', placeholder:'required'},{id:'prompt', label:'Prompt', type:'textarea', placeholder:'optional'},{id:'background', label:'Background', type:'checkbox'}]},
    {id:'fleet', group:'global', title:'fleet', description:'Run fleet manifest.', fields:[{id:'path', label:'Manifest path', type:'text', placeholder:'examples/fleet.yaml'},{id:'dryRun', label:'Dry run', type:'checkbox'}]},
    {id:'fleet-status', group:'global', title:'fleet status', description:'Show fleet status.', fields:[]},
    {id:'pipeline', group:'global', title:'pipeline', description:'Run pipeline manifest.', fields:[{id:'path', label:'Pipeline path', type:'text', placeholder:'examples/pipeline.yaml'},{id:'dryRun', label:'Dry run', type:'checkbox'}]},
    {id:'config-show', group:'global', title:'config show', description:'Show config.', fields:[]},
    {id:'config-get', group:'global', title:'config get', description:'Get one config key.', fields:[{id:'key', label:'Key', type:'text', placeholder:'memory'}]},
    {id:'config-set', group:'global', title:'config set', description:'Set config key.', fields:[{id:'key', label:'Key', type:'text', placeholder:'memory'},{id:'value', label:'Value', type:'text', placeholder:'16g'}]},
    {id:'config-reset', group:'global', title:'config reset', description:'Reset key or all.', fields:[{id:'key', label:'Key', type:'text', placeholder:'memory'},{id:'all', label:'Reset all', type:'checkbox'}]},
    {id:'template-list', group:'global', title:'template list', description:'List templates.', fields:[]},
    {id:'template-show', group:'global', title:'template show', description:'Show template.', fields:[{id:'name', label:'Template name', type:'text', placeholder:'bug-fix'}]},
    {id:'template-create', group:'global', title:'template create', description:'Create template in editor.', fields:[{id:'name', label:'Template name', type:'text', placeholder:'my-template'}]},
    {id:'cron-add', group:'global', title:'cron add', description:'Add scheduled job.', fields:[{id:'name', label:'Name', type:'text', placeholder:'nightly-review'},{id:'schedule', label:'Schedule', type:'text', placeholder:'daily 02:00'},{id:'command', label:'Command', type:'textarea', placeholder:'pipeline pipeline.yaml'}]},
    {id:'cron-list', group:'global', title:'cron list', description:'List cron jobs.', fields:[]},
    {id:'cron-remove', group:'global', title:'cron remove', description:'Remove cron job.', fields:[{id:'name', label:'Name', type:'text', placeholder:'nightly-review'}]},
    {id:'cron-enable', group:'global', title:'cron enable', description:'Enable cron job.', fields:[{id:'name', label:'Name', type:'text', placeholder:'nightly-review'}]},
    {id:'cron-disable', group:'global', title:'cron disable', description:'Disable cron job.', fields:[{id:'name', label:'Name', type:'text', placeholder:'nightly-review'}]},
    {id:'cron-run', group:'global', title:'cron run', description:'Run cron job now.', fields:[{id:'name', label:'Name', type:'text', placeholder:'nightly-review'}]},
    {id:'cron-daemon', group:'global', title:'cron daemon', description:'Run scheduler daemon.', fields:[]}
  ];

  function $(id) { return document.getElementById(id); }

  function esc(s) {
    return String(s ?? '').replace(/[&<>"]/g, ch => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[ch]));
  }

  function parseArgs(input) {
    const args = [];
    let current = '';
    let quote = '';
    let escape = false;
    for (const ch of input.trim()) {
      if (escape) {
        current += ch;
        escape = false;
        continue;
      }
      if (ch === '\\') {
        escape = true;
        continue;
      }
      if (quote) {
        if (ch === quote) {
          quote = '';
        } else {
          current += ch;
        }
        continue;
      }
      if (ch === '"' || ch === "'") {
        quote = ch;
        continue;
      }
      if (/\s/.test(ch)) {
        if (current) {
          args.push(current);
          current = '';
        }
        continue;
      }
      current += ch;
    }
    if (current) args.push(current);
    return args;
  }

  function selectedAgent() {
    return state.agents.find(a => a.Name === state.selected) || null;
  }

  function hierarchySegmentsForDisplay(agent) {
    const raw = String(agent.Hierarchy || '').trim();
    if (raw) return raw.split('/').filter(Boolean);
    if (agent.Fleet) return [String(agent.Fleet)];
    return [];
  }

  function renderHierarchy(agent) {
    const segments = hierarchySegmentsForDisplay(agent);
    const items = segments.concat([String(agent.Name || 'agent')]);
    if (items.length === 1) {
      return '<div class="hierarchy-card"><span class="hierarchy-label">Hierarchy</span><div class="hierarchy-empty">Standalone agent</div></div>';
    }
    return '<div class="hierarchy-card"><span class="hierarchy-label">Hierarchy</span><div class="hierarchy-track">' +
      items.map((item, idx) => {
        const self = idx === items.length - 1 ? ' self' : '';
        const node = '<span class="hierarchy-node' + self + '">' + esc(item) + '</span>';
        return idx === 0 ? node : '<span class="hierarchy-sep">›</span>' + node;
      }).join('') +
      '</div></div>';
  }

  function ensureSelection() {
    if (state.selected && selectedAgent()) return;
    state.selected = state.agents[0] ? state.agents[0].Name : '';
  }

  function renderFleet() {
    const tbody = $('agent-rows');
    const q = state.filter.trim().toLowerCase();
    const filtered = state.agents.filter(agent => {
      if (!q) return true;
      const fields = [agent.Name, agent.Type, agent.Repo, agent.Status, agent.Activity, agent.NetworkMode, agent.Auth, agent.GHAuth];
      return fields.some(v => String(v || '').toLowerCase().includes(q));
    });
    const rows = buildFleetRows(filtered);
    tbody.innerHTML = rows.map(row => {
      if (row.isGroup) {
        const chevron = row.collapsed ? '▸' : '▾';
        return '<tr class="group-row" data-group-path="' + esc(row.path) + '"><td colspan="8">' +
          '<button class="group-toggle" type="button" data-group-path="' + esc(row.path) + '" style="padding-left:' + (row.depth * 18) + 'px">' +
          '<span class="tree-cell"><strong>' + esc(chevron + ' ' + row.groupName) + '</strong></span></button></td></tr>';
      }
      const agent = row.agent;
      const selected = agent.Name === state.selected ? 'selected' : '';
      const statusClass = agent.Running || agent.Finished ? 'success' : 'danger';
      const activityClass = agent.Activity === 'Working' ? 'success' : (agent.Activity === 'Idle' ? 'warning' : 'muted');
      const typeIcon = agent.Type === 'claude' ? '🟠 ' : (agent.Type === 'codex' ? '🔵 ' : '');
      return '<tr class="' + selected + '" data-agent="' + esc(agent.Name) + '">' +
        '<td><strong><span class="tree-cell" style="padding-left:' + (row.depth * 18) + 'px">' + esc(typeIcon + agent.Name) + '</span></strong></td>' +
        '<td>' + esc(agent.Type) + '</td>' +
        '<td>' + esc(agent.Repo) + '</td>' +
        '<td class="' + statusClass + '">' + esc(agent.Status) + '</td>' +
        '<td class="' + activityClass + '">' + esc(agent.Activity) + '</td>' +
        '<td>' + esc(agent.CPU) + '</td>' +
        '<td>' + esc(agent.Memory) + '</td>' +
        '<td>' + esc(agent.PIDs) + '</td>' +
      '</tr>';
    }).join('');
    $('fleet-status').textContent = filtered.length + ' visible / ' + state.agents.length + ' total';
  }

  function groupSegments(agent) {
    if (agent.Hierarchy) {
      const parts = String(agent.Hierarchy).split('/');
      const agentLeaf = String(agent.Name || '').replace(/^agent-(claude|codex|shell)-/, '');
      if (parts.length > 1 && parts[parts.length - 1] === agentLeaf) {
        parts.pop();
      }
      return parts;
    }
    if (agent.Fleet) return [String(agent.Fleet)];
    return [];
  }

  function buildFleetRows(agents) {
    const root = {children: new Map(), order: [], agentIndices: []};
    const standalone = [];
    agents.forEach((agent, idx) => {
      const segments = groupSegments(agent);
      if (!segments.length) {
        standalone.push(idx);
        return;
      }
      let node = root;
      segments.forEach(seg => {
        if (!node.children.has(seg)) {
          node.children.set(seg, {name: seg, children: new Map(), order: [], agentIndices: []});
          node.order.push(seg);
        }
        node = node.children.get(seg);
      });
      node.agentIndices.push(idx);
    });

    const rows = [];
    root.order.forEach(name => buildFleetGroupRows(root.children.get(name), [], 0, rows, agents));
    standalone.forEach(idx => rows.push({isGroup: false, depth: 0, agent: agents[idx]}));
    return rows;
  }

  function buildFleetGroupRows(node, parents, depth, rows, agents) {
    const path = parents.concat(node.name).join('/');
    const collapsed = state.collapsedGroups[path] !== undefined ? state.collapsedGroups[path] : false;
    rows.push({isGroup: true, groupName: node.name, path, depth, collapsed});
    if (collapsed) return;
    const totalChildren = node.agentIndices.length + node.order.length;
    let childIndex = 0;
    node.agentIndices.forEach(idx => {
      rows.push({isGroup: false, depth: depth + 1, agent: agents[idx]});
      childIndex++;
    });
    node.order.forEach(name => {
      buildFleetGroupRows(node.children.get(name), parents.concat(node.name), depth + 1, rows, agents);
      childIndex++;
    });
  }

  function renderPage() {
    document.querySelectorAll('.page').forEach(node => {
      node.classList.toggle('active', node.dataset.page === state.page);
    });
    document.querySelectorAll('.menu-btn[data-page]').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.page === state.page);
    });
  }

  function renderAgent() {
    ensureSelection();
    const agent = selectedAgent();
    if (!agent) {
      $('agent-title').textContent = 'No agent selected';
      $('agent-subtitle').textContent = 'Spawn an agent or wait for one to appear.';
      $('agent-hero').innerHTML = '<div class="hero-copy"><div><h3>No agent selected</h3><p>Pick an agent from the fleet page to inspect details, logs, diff, review, cost, and PR state.</p></div></div><div class="hero-side">' + renderHierarchy({Name: 'agent'}) + '<div class="hero-badges"><span class="status-badge warning">Awaiting selection</span></div></div>';
      $('agent-cards').innerHTML = '<div class="empty">No active agents.</div>';
      $('content-box').textContent = 'No agent selected.';
      $('menu-agent-copy').textContent = 'Open tabs and actions for current selection.';
      syncCatalogTargets();
      return;
    }
    $('agent-title').textContent = agent.Name;
    $('agent-subtitle').textContent = [agent.Type, agent.Repo || 'no repo', agent.Status].filter(Boolean).join(' • ');
    $('menu-agent-copy').textContent = agent.Name + ' • ' + agent.Activity + ' • ' + agent.Status;
    const statusTone = agent.Running || agent.Finished ? 'success' : 'danger';
    const activityTone = agent.Activity === 'Working' ? 'success' : (agent.Activity === 'Idle' ? 'warning' : 'danger');
    $('agent-hero').innerHTML =
      '<div class="hero-copy"><div><h3>' + esc(agent.Name) + '</h3><p>' + esc(agent.Repo || 'No repository label') + '</p></div></div>' +
      '<div class="hero-side">' + renderHierarchy(agent) +
      '<div class="hero-badges">' +
        '<span class="status-badge ' + statusTone + '">' + esc(agent.Status) + '</span>' +
        '<span class="status-badge ' + activityTone + '">' + esc(agent.Activity) + '</span>' +
        '<span class="status-badge">' + esc(agent.Type) + '</span>' +
      '</div></div>';
    const blocks = [
      ['Overview', [
        ['Repo', agent.Repo || '—'],
        ['Fleet', agent.Fleet || 'standalone'],
        ['Type', agent.Type || '—']
      ]],
      ['Runtime', [
        ['Status', agent.Status || '—'],
        ['Activity', agent.Activity || '—'],
        ['CPU', agent.CPU || '—'],
        ['Memory', agent.Memory || '—'],
        ['Net I/O', agent.NetIO || '—'],
        ['PIDs', agent.PIDs || '—']
      ]],
      ['Access', [
        ['SSH', agent.SSH || '—'],
        ['Auth', agent.Auth || '—'],
        ['GH auth', agent.GHAuth || '—'],
        ['Docker', agent.Docker || '—'],
        ['Network', agent.NetworkMode || '—']
      ]]
    ];
    $('agent-cards').innerHTML = blocks.map(([title, rows]) =>
      '<section class="info-block"><h4>' + esc(title) + '</h4>' +
      rows.map(([label, value]) => '<div class="info-row"><span>' + esc(label) + '</span><span>' + esc(value) + '</span></div>').join('') +
      '</section>'
    ).join('');
    document.querySelectorAll('[data-tab]').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.tab === state.activeTab);
    });
    syncCatalogTargets();
  }

  function currentTargetValue() {
    return state.selected || '--latest';
  }

  function shellJoin(args) {
    return 'safe-ag ' + args.map(arg => {
      const value = String(arg ?? '');
      const needsQuote = /[\s"'\\$!&|;(){}[\]<>?*#~]/.test(value) || value.includes(String.fromCharCode(96));
      return needsQuote ? "'" + value.replace(/'/g, "'\\''") + "'" : value;
    }).join(' ');
  }

  function selectedCatalogSpec() {
    return commandCatalog.find(spec => spec.id === state.catalogSelected) || commandCatalog[0];
  }

  function showBusy(message) {
    $('global-loading-copy').textContent = message;
    $('global-loading').classList.add('visible');
  }

  function clearBusy() {
    $('global-loading').classList.remove('visible');
  }

  function setTabLoading(visible, message) {
    $('tab-loading').classList.toggle('hidden', !visible);
    if (visible) $('tab-loading-copy').textContent = message || 'Loading…';
  }

  function showBusy(message) {
    $('global-loading-copy').textContent = message;
    $('global-loading').classList.add('visible');
  }

  function clearBusy() {
    $('global-loading').classList.remove('visible');
  }

  function setTabLoading(visible, message) {
    const pill = $('tab-loading');
    pill.classList.toggle('hidden', !visible);
    if (visible) $('tab-loading-copy').textContent = message || 'Loading…';
  }

  function catalogFieldValue(id) {
    const el = document.querySelector('[data-catalog-field="' + id + '"]');
    if (!el) return '';
    if (el.type === 'checkbox') return !!el.checked;
    return el.value;
  }

  function catalogValues() {
    const values = {};
    const spec = selectedCatalogSpec();
    (spec.fields || []).forEach(field => { values[field.id] = catalogFieldValue(field.id); });
    return values;
  }

  function buildCatalogArgs(spec, values) {
    const v = values || {};
    const target = String(v.target || currentTargetValue()).trim() || '--latest';
    const pushTarget = args => args.concat(target === '--latest' ? ['--latest'] : [target]);
    switch (spec.id) {
      case 'attach': return pushTarget(['attach']);
      case 'resume': return pushTarget(['resume']);
      case 'stop': return pushTarget(['stop']);
      case 'peek': return appendIf(pushTarget(['peek']), v.lines, '--lines');
      case 'logs': {
        let args = pushTarget(['logs']);
        args = appendIf(args, v.lines, '--lines');
        if (v.follow) args.push('--follow');
        return args;
      }
      case 'summary': return pushTarget(['summary']);
      case 'output': {
        let args = pushTarget(['output']);
        if (v.diff) args.push('--diff');
        if (v.files) args.push('--files');
        if (v.commits) args.push('--commits');
        if (v.json) args.push('--json');
        return args;
      }
      case 'diff': {
        let args = pushTarget(['diff']);
        if (v.stat) args.push('--stat');
        return args;
      }
      case 'review': return appendIf(pushTarget(['review']), v.base, '--base');
      case 'replay': {
        let args = pushTarget(['replay']);
        if (v.toolsOnly) args.push('--tools-only');
        return args;
      }
      case 'retry': return appendIf(pushTarget(['retry']), v.feedback, '--feedback');
      case 'sessions': {
        let args = pushTarget(['sessions']);
        if (String(v.dest || '').trim()) args.push(String(v.dest).trim());
        return args;
      }
      case 'aws-refresh': {
        let args = pushTarget(['aws-refresh']);
        if (String(v.profile || '').trim()) args.push(String(v.profile).trim());
        return args;
      }
      case 'cost': {
        if (String(v.history || '').trim()) return ['cost', '--history', String(v.history).trim()];
        return pushTarget(['cost']);
      }
      case 'pr': {
        let args = pushTarget(['pr']);
        args = appendIf(args, v.base, '--base');
        args = appendIf(args, v.title, '--title');
        return args;
      }
      case 'checkpoint-create': {
        let args = pushTarget(['checkpoint', 'create']);
        if (String(v.label || '').trim()) args.push(String(v.label).trim());
        return args;
      }
      case 'checkpoint-list': return pushTarget(['checkpoint', 'list']);
      case 'checkpoint-revert': return pushTarget(['checkpoint', 'revert']).concat(requiredValue(v.ref));
      case 'todo-add': return pushTarget(['todo', 'add']).concat(requiredValue(v.text));
      case 'todo-list': return pushTarget(['todo', 'list']);
      case 'todo-check': return pushTarget(['todo', 'check']).concat(requiredValue(v.index));
      case 'todo-uncheck': return pushTarget(['todo', 'uncheck']).concat(requiredValue(v.index));
      case 'mcp-login': return ['mcp-login', requiredValue(v.service), target];
      case 'list': return v.json ? ['list', '--json'] : ['list'];
      case 'audit': return appendIf(['audit'], v.lines, '--lines');
      case 'cleanup': return v.auth ? ['cleanup', '--auth'] : ['cleanup'];
      case 'setup': return ['setup'];
      case 'update': {
        const args = ['update'];
        if (v.full) args.push('--full');
        if (v.quick) args.push('--quick');
        return args;
      }
      case 'diagnose': return ['diagnose'];
      case 'vm-start': return ['vm', 'start'];
      case 'vm-stop': return ['vm', 'stop'];
      case 'vm-ssh': return ['vm', 'ssh'];
      case 'tui': return ['tui'];
      case 'dashboard': return appendIf(['dashboard'], v.bind, '--bind');
      case 'spawn-cli': {
        let args = ['spawn', String(v.agentType || 'claude')];
        args = appendIfPos(args, v.repo, '--repo');
        args = appendIf(args, v.name, '--name');
        args = appendIf(args, v.prompt, '--prompt');
        if (v.ssh) args.push('--ssh');
        if (v.reuseAuth) args.push('--reuse-auth');
        if (v.reuseGHAuth) args.push('--reuse-gh-auth');
        if (v.docker) args.push('--docker');
        if (v.background) args.push('--background');
        args = appendIf(args, v.identity, '--identity');
        args = appendIf(args, v.awsProfile, '--aws');
        return args;
      }
      case 'run': {
        let args = ['run', requiredValue(v.repo)];
        if (String(v.prompt || '').trim()) args.push(String(v.prompt).trim());
        if (v.background) args.push('--background');
        return args;
      }
      case 'fleet': {
        let args = ['fleet', requiredValue(v.path)];
        if (v.dryRun) args.push('--dry-run');
        return args;
      }
      case 'fleet-status': return ['fleet', 'status'];
      case 'pipeline': {
        let args = ['pipeline', requiredValue(v.path)];
        if (v.dryRun) args.push('--dry-run');
        return args;
      }
      case 'config-show': return ['config', 'show'];
      case 'config-get': return ['config', 'get', requiredValue(v.key)];
      case 'config-set': return ['config', 'set', requiredValue(v.key), requiredValue(v.value)];
      case 'config-reset': return v.all ? ['config', 'reset', '--all'] : ['config', 'reset', requiredValue(v.key)];
      case 'template-list': return ['template', 'list'];
      case 'template-show': return ['template', 'show', requiredValue(v.name)];
      case 'template-create': return ['template', 'create', requiredValue(v.name)];
      case 'cron-add': return ['cron', 'add', requiredValue(v.name), requiredValue(v.schedule)].concat(parseArgs(requiredValue(v.command)));
      case 'cron-list': return ['cron', 'list'];
      case 'cron-remove': return ['cron', 'remove', requiredValue(v.name)];
      case 'cron-enable': return ['cron', 'enable', requiredValue(v.name)];
      case 'cron-disable': return ['cron', 'disable', requiredValue(v.name)];
      case 'cron-run': return ['cron', 'run', requiredValue(v.name)];
      case 'cron-daemon': return ['cron', 'daemon'];
      default: return [];
    }
  }

  function requiredValue(value) {
    const out = String(value ?? '').trim();
    if (!out) throw new Error('Missing required field');
    return out;
  }

  function appendIf(args, value, flag) {
    const out = String(value ?? '').trim();
    return out ? args.concat([flag, out]) : args;
  }

  function appendIfPos(args, value, flag) {
    const out = String(value ?? '').trim();
    return out ? args.concat([flag, out]) : args;
  }

  function renderCatalog() {
    const groups = {agent: 'Agent commands', global: 'Global commands'};
    const list = $('catalog-list');
    list.innerHTML = Object.entries(groups).map(([groupId, label]) => {
      const items = commandCatalog.filter(spec => spec.group === groupId).map(spec =>
        '<button type="button" class="catalog-item ' + (spec.id === state.catalogSelected ? 'active' : '') + '" data-catalog="' + esc(spec.id) + '">' +
          '<strong>' + esc(spec.title) + '</strong><span>' + esc(spec.description) + '</span></button>'
      ).join('');
      return '<div class="catalog-group"><div class="catalog-group-title">' + esc(label) + '</div>' + items + '</div>';
    }).join('');
  }

  function renderCatalogFields() {
    const spec = selectedCatalogSpec();
    $('catalog-title').textContent = spec.title;
    $('catalog-description').textContent = spec.description;
    $('catalog-fields').innerHTML = (spec.fields || []).map(field => renderCatalogField(field)).join('');
    updateCatalogPreview();
  }

  function syncCatalogTargets() {
    document.querySelectorAll('[data-catalog-field="target"]').forEach(el => {
      if (!el.dataset.dirty || !el.value || el.value === '--latest') {
        el.value = currentTargetValue();
      }
    });
    updateCatalogPreview();
  }

  function renderCatalogField(field) {
    if (field.type === 'checkbox') {
      return '<label class="form-field"><span>' + esc(field.label) + '</span><input data-catalog-field="' + esc(field.id) + '" type="checkbox" ' + (field.checked ? 'checked' : '') + '></label>';
    }
    if (field.type === 'select') {
      return '<label class="form-field"><span>' + esc(field.label) + '</span><select data-catalog-field="' + esc(field.id) + '">' +
        (field.options || []).map(opt => '<option value="' + esc(opt) + '">' + esc(opt) + '</option>').join('') +
      '</select></label>';
    }
    if (field.type === 'textarea') {
      return '<label class="form-field"><span>' + esc(field.label) + '</span><textarea data-catalog-field="' + esc(field.id) + '" placeholder="' + esc(field.placeholder || '') + '">' + esc(field.default || '') + '</textarea></label>';
    }
    const value = field.type === 'target' ? esc(currentTargetValue()) : esc(field.default || '');
    const placeholder = field.type === 'target' ? 'selected agent or --latest' : esc(field.placeholder || '');
    return '<label class="form-field"><span>' + esc(field.label) + '</span><input data-catalog-field="' + esc(field.id) + '" type="text" value="' + value + '" placeholder="' + placeholder + '"></label>';
  }

  function updateCatalogPreview() {
    try {
      const args = buildCatalogArgs(selectedCatalogSpec(), catalogValues());
      $('catalog-preview').textContent = shellJoin(args);
    } catch (err) {
      $('catalog-preview').textContent = 'Incomplete: ' + err.message;
    }
  }

  async function runCatalogCommand(copyOnly) {
    try {
      const args = buildCatalogArgs(selectedCatalogSpec(), catalogValues());
      const preview = shellJoin(args);
      if (copyOnly) {
        $('catalog-preview').textContent = preview;
        if (navigator.clipboard) navigator.clipboard.writeText(preview).catch(() => {});
        $('command-output').textContent = preview;
        return;
      }
      showBusy('Running ' + preview);
      $('command-output').textContent = 'Running ' + preview + '…';
      const data = await fetchJSON('/api/command', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({args})
      });
      $('command-output').textContent = data.command || data.output || '(no output)';
      await refreshAgents();
      if (selectedAgent()) await loadTab(state.activeTab);
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
      $('catalog-preview').textContent = 'Incomplete: ' + String(err.message || err);
    } finally {
      clearBusy();
    }
  }

  async function fetchText(url) {
    const res = await fetch(url);
    const text = await res.text();
    if (!res.ok) throw new Error(text || ('Request failed: ' + res.status));
    return text;
  }

  async function fetchJSON(url, options) {
    const res = await fetch(url, options);
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) {}
    if (!res.ok) {
      throw new Error((data && data.error) || text || ('Request failed: ' + res.status));
    }
    return data;
  }

  async function loadTab(tab) {
    state.activeTab = tab;
    renderAgent();
    const agent = selectedAgent();
    if (!agent) return;
    const requestId = ++state.contentRequest;
    setTabLoading(true, 'Loading ' + tab);
    $('tab-status').textContent = 'Loading ' + tab + '…';
    try {
      const url = '/api/agents/' + encodeURIComponent(agent.Name) + '/content/' + tab + (tab === 'logs' ? '?tail=' + state.logsTail : '');
      const text = await fetchText(url);
      if (requestId !== state.contentRequest) return;
      $('content-box').textContent = text;
      $('tab-status').textContent = 'Showing ' + tab + ' for ' + agent.Name;
    } catch (err) {
      if (requestId !== state.contentRequest) return;
      $('content-box').textContent = String(err.message || err);
      $('tab-status').textContent = 'Failed to load ' + tab;
    } finally {
      if (requestId === state.contentRequest) setTabLoading(false);
    }
  }

  async function postAgentAction(action, payload = {}) {
    const agent = selectedAgent();
    if (!agent) return;
    showBusy('Running ' + action + ' on ' + agent.Name);
    $('command-output').textContent = 'Running ' + action + ' on ' + agent.Name + '…';
    try {
      const data = await fetchJSON('/api/agents/' + encodeURIComponent(agent.Name) + '/action/' + action, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
      });
      $('command-output').textContent = data.output || '(no output)';
      await refreshAgents();
      await loadTab(state.activeTab);
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
    } finally {
      clearBusy();
    }
  }

  async function postGlobalAction(action, payload = {}) {
    showBusy('Running ' + action);
    $('command-output').textContent = 'Running ' + action + '…';
    try {
      const data = await fetchJSON('/api/global/action/' + action, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload)
      });
      $('command-output').textContent = data.output || '(no output)';
      await refreshAgents();
      if (state.activeTab === 'summary') await loadTab('summary');
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
    } finally {
      clearBusy();
    }
  }

  async function showInteractive(kind, server = '') {
    const agent = selectedAgent();
    if (!agent) return;
    try {
      const suffix = server ? ('?server=' + encodeURIComponent(server)) : '';
      const data = await fetchJSON('/api/agents/' + encodeURIComponent(agent.Name) + '/interactive/' + kind + suffix);
      $('command-output').textContent = data.command || '(no command)';
      if (data.command && navigator.clipboard) {
        navigator.clipboard.writeText(data.command).catch(() => {});
      }
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
    }
  }

  async function loadAudit() {
    showBusy('Loading audit');
    $('command-output').textContent = 'Loading audit…';
    try {
      const text = await fetchText('/api/global/content/audit');
      $('command-output').textContent = text;
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
    } finally {
      clearBusy();
    }
  }

  async function runCommand() {
    const input = $('command-input').value;
    const args = parseArgs(input);
    if (!args.length) return;
    showBusy('Running safe-ag ' + input);
    $('command-output').textContent = 'Running safe-ag ' + input + '…';
    try {
      const data = await fetchJSON('/api/command', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({args})
      });
      $('command-output').textContent = data.command || data.output || '(no output)';
      await refreshAgents();
      if (selectedAgent()) await loadTab(state.activeTab);
    } catch (err) {
      $('command-output').textContent = String(err.message || err);
    } finally {
      clearBusy();
    }
  }

  async function refreshAgents() {
    try {
      state.agents = await fetchJSON('/api/agents');
      ensureSelection();
      renderFleet();
      renderAgent();
    } catch (err) {
      $('fleet-status').textContent = 'Refresh failed: ' + err.message;
    }
  }

  function hookEvents() {
    $('agent-filter').addEventListener('input', e => {
      state.filter = e.target.value;
      renderFleet();
    });
    document.querySelectorAll('.menu-btn[data-page]').forEach(btn => {
      btn.addEventListener('click', async () => {
        state.page = btn.dataset.page;
        renderPage();
        if (state.page === 'agent' && selectedAgent()) {
          await loadTab(state.activeTab);
        }
      });
    });
    $('refresh-btn').addEventListener('click', async () => {
      await refreshAgents();
      if (selectedAgent()) await loadTab(state.activeTab);
    });
    $('audit-btn').addEventListener('click', async () => {
      state.page = 'global';
      renderPage();
      await loadAudit();
    });
    $('kill-all-btn').addEventListener('click', () => {
      if (confirm('Stop all agents?')) postGlobalAction('kill-all');
    });
    $('agent-rows').addEventListener('click', async e => {
      const toggle = e.target.closest('[data-group-path]');
      if (toggle && !toggle.closest('[data-agent]')) {
        const path = toggle.dataset.groupPath;
        state.collapsedGroups[path] = !(state.collapsedGroups[path] !== undefined ? state.collapsedGroups[path] : false);
        renderFleet();
        return;
      }
      const row = e.target.closest('tr[data-agent]');
      if (!row) return;
      state.selected = row.dataset.agent;
      state.page = 'agent';
      renderFleet();
      renderPage();
      renderAgent();
      await loadTab(state.activeTab);
    });
    document.querySelectorAll('[data-tab]').forEach(btn => {
      btn.addEventListener('click', () => loadTab(btn.dataset.tab));
    });
    document.querySelectorAll('[data-action]').forEach(btn => {
      btn.addEventListener('click', () => {
        const action = btn.dataset.action;
        if (action === 'checkpoint') {
          const label = prompt('Checkpoint label (optional):', '');
          postAgentAction(action, {label: label || ''});
          return;
        }
        postAgentAction(action);
      });
    });
    document.querySelectorAll('[data-interactive]').forEach(btn => {
      btn.addEventListener('click', () => showInteractive(btn.dataset.interactive));
    });
    $('copy-submit').addEventListener('click', () => {
      postAgentAction('copy', {
        source: $('copy-source').value,
        destination: $('copy-dest').value
      });
    });
    $('push-submit').addEventListener('click', () => {
      postAgentAction('push', {
        source: $('push-source').value,
        destination: $('push-dest').value
      });
    });
    $('spawn-submit').addEventListener('click', () => {
      postGlobalAction('spawn', {
        agentType: $('spawn-type').value,
        repoURL: $('spawn-repo').value,
        name: $('spawn-name').value,
        prompt: $('spawn-prompt').value,
        ssh: $('spawn-ssh').checked,
        reuseAuth: $('spawn-auth').checked,
        reuseGHAuth: $('spawn-gh-auth').checked,
        awsProfile: $('spawn-aws').value,
        docker: $('spawn-docker').checked,
        identity: $('spawn-identity').value
      });
    });
    $('fleet-submit').addEventListener('click', () => postGlobalAction('fleet', {path: $('fleet-path').value}));
    $('pipeline-submit').addEventListener('click', () => postGlobalAction('pipeline', {path: $('pipeline-path').value}));
    $('attach-cmd').addEventListener('click', () => showInteractive('attach'));
    $('resume-cmd').addEventListener('click', () => showInteractive('resume'));
    $('mcp-cmd').addEventListener('click', () => showInteractive('mcp-login', $('mcp-server').value));
    $('command-run').addEventListener('click', runCommand);
    $('command-input').addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        runCommand();
      }
    });
    $('catalog-list').addEventListener('click', e => {
      const btn = e.target.closest('[data-catalog]');
      if (!btn) return;
      state.catalogSelected = btn.dataset.catalog;
      renderCatalog();
      renderCatalogFields();
    });
    $('catalog-fields').addEventListener('input', e => {
      if (e.target.matches('[data-catalog-field]')) {
        if (e.target.dataset.catalogField === 'target') e.target.dataset.dirty = '1';
        updateCatalogPreview();
      }
    });
    $('catalog-run').addEventListener('click', () => runCatalogCommand(false));
    $('catalog-copy').addEventListener('click', () => runCatalogCommand(true));
  }

  function startAgentStream() {
    const es = new EventSource('/events');
    es.onmessage = async e => {
      try {
        state.agents = JSON.parse(e.data);
        const current = selectedAgent();
        if (!current && state.selected && state.agents.length) {
          ensureSelection();
        }
        renderFleet();
        renderAgent();
      } catch (_) {}
    };
    es.onerror = () => {
      $('fleet-status').textContent = 'Live stream disconnected; using timer refresh';
    };
  }

  function startContentRefresh() {
    setInterval(() => {
      if (!selectedAgent()) return;
      if (state.page === 'agent') {
        loadTab(state.activeTab);
      }
    }, 3000);
    setInterval(refreshAgents, refreshIntervalMs);
  }

  async function init() {
    ensureSelection();
    renderFleet();
    renderPage();
    renderAgent();
    renderCatalog();
    renderCatalogFields();
    hookEvents();
    startAgentStream();
    startContentRefresh();
    if (state.selected) {
      await loadTab(state.activeTab);
    } else {
      $('content-box').textContent = 'No agents available yet. Use the spawn form or command runner.';
    }
  }

  init();
</script>
</body>
</html>{{end}}
`
