"""Local, read-only dashboard and final reports for optimizer campaigns."""

from __future__ import annotations

import html
import json
import signal
import sqlite3
import threading
from datetime import datetime, timezone
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from .statistics import aggregate_wdl


class DashboardError(RuntimeError):
    """Raised when a dashboard request cannot be served safely."""


TERMINAL_CAMPAIGN_STATES = {"completed", "failed", "rejected", "interrupted"}
TERMINAL_TRIAL_STATES = {"completed", "failed", "rejected"}
WAITING_TRIAL_STATES = {"pending", "running", "paused", "interrupted"}


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _json(raw: str | None, default: Any) -> Any:
    if not raw:
        return default
    try:
        return json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        return default


def _parameter_values(document: dict[str, Any]) -> dict[str, int]:
    values: dict[str, int] = {}
    for item in document.get("parameters", []):
        if isinstance(item, dict) and isinstance(item.get("name"), str) and isinstance(item.get("value"), int):
            values[item["name"]] = int(item["value"])
    return values


def _fallback_metrics(result: dict[str, Any] | None) -> dict[str, Any] | None:
    """Read coordinate-search evidence when no match blocks exist."""
    if not isinstance(result, dict):
        return None
    source = result.get("statistics")
    if not isinstance(source, dict):
        source = result
    games = source.get("games")
    wins = source.get("wins")
    draws = source.get("draws")
    losses = source.get("losses")
    if not all(isinstance(value, int) and not isinstance(value, bool) for value in (games, wins, draws, losses)):
        return None
    metrics = {
        "blocks_completed": int(source.get("blocks_completed", 0)),
        "games": int(games),
        "wins": int(wins),
        "draws": int(draws),
        "losses": int(losses),
        "score": float(source.get("score_percent", source.get("score", 0.0))),
        "score_percent": float(source.get("score_percent", source.get("score", 0.0))),
        "score_ci_low": float(source.get("score_ci_low", 0.0)),
        "score_ci_high": float(source.get("score_ci_high", 100.0)),
        "elo_estimate": float(source.get("elo_estimate", 0.0)),
        "elo_ci_low": float(source.get("elo_ci_low", -800.0)),
        "elo_ci_high": float(source.get("elo_ci_high", 800.0)),
        "uncertainty": float(source.get("uncertainty", 50.0)),
        "block_ids": list(source.get("block_ids", [])) if isinstance(source.get("block_ids", []), list) else [],
    }
    return metrics


def _metrics(blocks: list[dict[str, Any]], result: dict[str, Any] | None) -> dict[str, Any]:
    match_metrics = aggregate_wdl(blocks)
    if match_metrics["games"]:
        return match_metrics
    return _fallback_metrics(result) or match_metrics


def _category(status: str) -> str:
    if status == "completed":
        return "completed"
    if status in {"rejected", "failed"}:
        return "rejected"
    return "waiting"


def _parameter_record(row: dict[str, Any] | None) -> dict[str, Any] | None:
    if row is None:
        return None
    document = _json(row.get("document_json"), {})
    if not isinstance(document, dict):
        document = {}
    return {
        "parameter_set_id": row["parameter_set_id"],
        "parameter_hash": row["parameter_hash"],
        "group_name": row["group_name"],
        "created_at": row["created_at"],
        "document": document,
        "values": _parameter_values(document),
    }


def _trial_record(row: dict[str, Any], parameter: dict[str, Any] | None, blocks: list[dict[str, Any]]) -> dict[str, Any]:
    result = _json(row.get("result_json"), None)
    if not isinstance(result, dict):
        result = None
    metrics = _metrics(blocks, result)
    current_block = next((block for block in blocks if block["status"] == "running"), None)
    if current_block is None:
        current_block = next((block for block in blocks if block["status"] == "pending"), None)
    return {
        "trial_id": row["trial_id"],
        "status": row["status"],
        "category": _category(str(row["status"])),
        "algorithm": row["algorithm"],
        "seed": row["seed"],
        "created_at": row["created_at"],
        "started_at": row["started_at"],
        "finished_at": row["finished_at"],
        "updated_at": row["updated_at"],
        "error": row["error"],
        "parameter": parameter,
        "metrics": metrics,
        "result": result,
        "blocks": [
            {
                "block_id": block["block_id"],
                "block_index": block["block_index"],
                "status": block["status"],
                "attempt": block["attempt"],
                "wins": block["wins"],
                "draws": block["draws"],
                "losses": block["losses"],
                "score": block["score"],
                "run_dir": block["run_dir"],
                "error": block["error"],
                "updated_at": block["updated_at"],
            }
            for block in blocks
        ],
        "current_block": (
            {
                "block_id": current_block["block_id"],
                "block_index": current_block["block_index"],
                "status": current_block["status"],
                "attempt": current_block["attempt"],
                "run_dir": current_block["run_dir"],
            }
            if current_block is not None
            else None
        ),
    }


def _parameter_diff(baseline: dict[str, int], best: dict[str, int]) -> list[dict[str, Any]]:
    names = list(baseline)
    names.extend(name for name in best if name not in baseline)
    return [
        {
            "name": name,
            "baseline": baseline.get(name),
            "best": best.get(name),
            "delta": (best.get(name, 0) - baseline.get(name, 0)) if name in baseline and name in best else None,
            "changed": baseline.get(name) != best.get(name),
        }
        for name in names
    ]


def _finished(campaign_status: str, trials: list[dict[str, Any]], blocks: list[dict[str, Any]]) -> bool:
    if campaign_status in TERMINAL_CAMPAIGN_STATES:
        return True
    if not trials or any(trial["status"] not in TERMINAL_TRIAL_STATES for trial in trials):
        return False
    return not any(block["status"] in {"pending", "running", "interrupted"} for block in blocks)


class DashboardReader:
    """Read a campaign using a SQLite read-only connection for every snapshot."""

    def __init__(self, data_dir: Path, campaign_id: str):
        self.data_dir = Path(data_dir).resolve()
        self.campaign_id = campaign_id
        self.database_path = self.data_dir / campaign_id / "campaign.db"

    def _connection(self) -> sqlite3.Connection:
        if not self.database_path.is_file():
            raise DashboardError(f"campaign database does not exist: {self.database_path}")
        connection = sqlite3.connect(self.database_path.as_uri() + "?mode=ro", uri=True, timeout=5)
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA query_only=ON")
        connection.execute("PRAGMA busy_timeout=5000")
        return connection

    def snapshot(self) -> dict[str, Any]:
        with self._connection() as connection:
            campaign_row = connection.execute(
                "SELECT * FROM campaigns WHERE campaign_id=?", (self.campaign_id,)
            ).fetchone()
            if campaign_row is None:
                raise DashboardError(f"unknown campaign: {self.campaign_id}")
            campaign = dict(campaign_row)
            parameter_rows = [
                dict(row)
                for row in connection.execute(
                    "SELECT * FROM parameter_sets WHERE campaign_id=? ORDER BY created_at,parameter_set_id",
                    (self.campaign_id,),
                ).fetchall()
            ]
            parameters = {row["parameter_set_id"]: _parameter_record(row) for row in parameter_rows}
            block_rows = [
                dict(row)
                for row in connection.execute(
                    "SELECT * FROM match_blocks WHERE campaign_id=? ORDER BY block_index,created_at,block_id",
                    (self.campaign_id,),
                ).fetchall()
            ]
            blocks_by_trial: dict[str, list[dict[str, Any]]] = {}
            for block in block_rows:
                blocks_by_trial.setdefault(str(block["trial_id"]), []).append(block)
            trial_rows = [
                dict(row)
                for row in connection.execute(
                    "SELECT * FROM trials WHERE campaign_id=? ORDER BY created_at DESC,trial_id DESC",
                    (self.campaign_id,),
                ).fetchall()
            ]
            trials = [
                _trial_record(
                    row,
                    parameters.get(str(row["parameter_set_id"])),
                    blocks_by_trial.get(str(row["trial_id"]), []),
                )
                for row in trial_rows
            ]
            checkpoint_row = connection.execute(
                "SELECT * FROM optimizer_state WHERE campaign_id=?", (self.campaign_id,)
            ).fetchone()
            checkpoint = None
            if checkpoint_row is not None:
                checkpoint = dict(checkpoint_row)
                checkpoint["state"] = _json(checkpoint.pop("state_json"), {})
            error_rows: list[dict[str, Any]] = []
            for trial in trial_rows:
                if trial["error"]:
                    error_rows.append(
                        {
                            "source": "trial",
                            "id": trial["trial_id"],
                            "error": trial["error"],
                            "updated_at": trial["updated_at"],
                        }
                    )
            for block in block_rows:
                if block["error"]:
                    error_rows.append(
                        {
                            "source": "match_block",
                            "id": block["block_id"],
                            "error": block["error"],
                            "updated_at": block["updated_at"],
                        }
                    )
            error_rows.sort(key=lambda item: (item["updated_at"] or "", item["id"]), reverse=True)

        current = next((trial for trial in trials if trial["status"] in {"running", "pending"}), None)
        if current is None:
            current = next((trial for trial in trials if trial["status"] in {"paused", "interrupted"}), None)
        if current is None and trials:
            current = trials[0]

        completed_trials = [trial for trial in trials if trial["status"] == "completed"]
        best_trial = max(
            completed_trials,
            key=lambda trial: (
                float(trial["metrics"].get("elo_estimate", 0.0)) if trial["metrics"].get("games", 0) else -100000.0,
                float(trial["metrics"].get("score_percent", 0.0)),
                str(trial["created_at"]),
            ),
            default=None,
        )
        baseline = next(
            (item for item in parameters.values() if item is not None and item["group_name"] == "baseline"),
            None,
        )
        best_parameter = best_trial["parameter"] if best_trial is not None else baseline
        baseline_values = baseline["values"] if baseline is not None else {}
        best_values = best_parameter["values"] if best_parameter is not None else {}
        all_metrics = _metrics(block_rows, None)
        counts = {"completed": 0, "rejected": 0, "waiting": 0}
        for trial in trials:
            counts[trial["category"]] += 1
        config = _json(campaign["config_json"], {})
        if not isinstance(config, dict):
            config = {}
        with self._connection() as confirmation_connection:
            confirmation_row = confirmation_connection.execute(
                "SELECT * FROM confirmations WHERE campaign_id=?", (self.campaign_id,)
            ).fetchone()
        confirmation: dict[str, Any] | None = None
        if confirmation_row is not None:
            confirmation = {
                "confirmation_id": confirmation_row["confirmation_id"],
                "status": confirmation_row["status"],
                "outcome": confirmation_row["outcome"],
                "games_target": confirmation_row["games_target"],
                "wins": confirmation_row["wins"],
                "draws": confirmation_row["draws"],
                "losses": confirmation_row["losses"],
                "score": confirmation_row["score"],
                "score_ci_low": confirmation_row["score_ci_low"],
                "score_ci_high": confirmation_row["score_ci_high"],
                "recommendation_parameter_hash": confirmation_row["recommendation_parameter_hash"],
            }
        finished = _finished(str(campaign["status"]), trials, block_rows) and (
            confirmation is None or confirmation["status"] == "completed"
        )
        return {
            "schema_version": 1,
            "generated_at": _now(),
            "read_only": True,
            "campaign": {
                "campaign_id": campaign["campaign_id"],
                "name": campaign["name"],
                "status": campaign["status"],
                "finished": finished,
                "config": config,
                "config_hash": campaign["config_hash"],
                "baseline_engine_id": campaign["baseline_engine_id"],
                "baseline_parameter_hash": campaign["baseline_parameter_hash"],
                "registry_name": campaign["registry_name"],
                "master_seed": campaign["master_seed"],
                "created_at": campaign["created_at"],
                "updated_at": campaign["updated_at"],
                "finished_at": campaign["finished_at"],
            },
            "current_trial": current,
            "campaign_metrics": all_metrics,
            "confirmation": confirmation,
            "candidate_counts": counts,
            "candidates": trials,
            "consumed_games": int(all_metrics.get("games", 0)),
            "checkpoint": checkpoint,
            "latest_error": error_rows[0] if error_rows else None,
            "best_parameters": {
                "source": "completed_trial" if best_trial is not None else "baseline",
                "trial_id": best_trial["trial_id"] if best_trial is not None else None,
                "parameter_set_id": best_parameter["parameter_set_id"] if best_parameter is not None else None,
                "parameter_hash": best_parameter["parameter_hash"] if best_parameter is not None else None,
                "values": best_values,
                "metrics": best_trial["metrics"] if best_trial is not None else None,
            },
            "baseline_parameters": {
                "parameter_set_id": baseline["parameter_set_id"] if baseline is not None else None,
                "parameter_hash": baseline["parameter_hash"] if baseline is not None else None,
                "values": baseline_values,
            },
            "parameter_differences": _parameter_diff(baseline_values, best_values),
            "database": {"path": str(self.database_path), "journal_mode": "wal"},
        }


def _listen_address(value: str) -> tuple[str, int]:
    if ":" not in value:
        raise DashboardError("dashboard --listen must be 127.0.0.1:<port>")
    host, raw_port = value.rsplit(":", 1)
    if host != "127.0.0.1":
        raise DashboardError("dashboard may bind only to 127.0.0.1")
    try:
        port = int(raw_port)
    except ValueError as exc:
        raise DashboardError("dashboard port must be an integer") from exc
    if not 0 <= port <= 65535:
        raise DashboardError("dashboard port must be between 0 and 65535")
    return host, port


def _html_text(value: Any) -> str:
    return html.escape("—" if value is None or value == "" else str(value))


def render_report_html(snapshot: dict[str, Any]) -> str:
    campaign = snapshot["campaign"]
    current = snapshot.get("current_trial") or {}
    metrics = current.get("metrics") or snapshot.get("campaign_metrics") or {}
    counts = snapshot.get("candidate_counts") or {}
    rows = "".join(
        "<tr>"
        f"<td>{_html_text(item.get('trial_id'))}</td>"
        f"<td>{_html_text(item.get('status'))}</td>"
        f"<td>{_html_text(item.get('metrics', {}).get('wins'))}-"
        f"{_html_text(item.get('metrics', {}).get('draws'))}-"
        f"{_html_text(item.get('metrics', {}).get('losses'))}</td>"
        f"<td>{_html_text(item.get('metrics', {}).get('score_percent'))}%</td>"
        f"<td>{_html_text(item.get('parameter', {}).get('parameter_hash') if item.get('parameter') else None)}</td>"
        "</tr>"
        for item in snapshot.get("candidates", [])
    )
    parameter_rows = "".join(
        "<tr>"
        f"<td>{_html_text(item.get('name'))}</td>"
        f"<td>{_html_text(item.get('baseline'))}</td>"
        f"<td>{_html_text(item.get('best'))}</td>"
        f"<td>{_html_text(item.get('delta'))}</td>"
        "</tr>"
        for item in snapshot.get("parameter_differences", [])
    )
    checkpoint = json.dumps(snapshot.get("checkpoint"), ensure_ascii=False, indent=2)
    latest_error = snapshot.get("latest_error")
    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>GoAlaric campaign report</title>
<style>body{{font:15px system-ui,sans-serif;max-width:1180px;margin:2rem auto;padding:0 1rem;color:#17202a;background:#f6f7f9}}section{{background:white;border:1px solid #d9dee5;border-radius:10px;padding:1rem;margin:1rem 0}}.grid{{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.7rem}}.card{{background:#f0f4f8;border-radius:8px;padding:.8rem}}.label{{color:#5b6775;font-size:.8rem}}.value{{font-size:1.3rem;font-weight:650;margin-top:.25rem}}table{{border-collapse:collapse;width:100%}}th,td{{padding:.45rem;border-bottom:1px solid #e4e7eb;text-align:left;font-size:.9rem}}pre{{overflow:auto;background:#111827;color:#e5e7eb;padding:1rem;border-radius:8px}}</style></head>
<body><h1>GoAlaric optimizer report</h1><p><strong>{_html_text(campaign.get('name'))}</strong> · {_html_text(campaign.get('campaign_id'))} · status: {_html_text(campaign.get('status'))}</p>
<section><h2>Result</h2><div class="grid">
<div class="card"><div class="label">W–D–L</div><div class="value">{_html_text(metrics.get('wins'))}–{_html_text(metrics.get('draws'))}–{_html_text(metrics.get('losses'))}</div></div>
<div class="card"><div class="label">Score</div><div class="value">{_html_text(metrics.get('score_percent'))}%</div></div>
<div class="card"><div class="label">Elo</div><div class="value">{_html_text(metrics.get('elo_estimate'))}</div></div>
<div class="card"><div class="label">95% Elo CI</div><div class="value">{_html_text(metrics.get('elo_ci_low'))} … {_html_text(metrics.get('elo_ci_high'))}</div></div>
<div class="card"><div class="label">95% score CI</div><div class="value">{_html_text(metrics.get('score_ci_low'))}% … {_html_text(metrics.get('score_ci_high'))}%</div></div>
<div class="card"><div class="label">Consumed games</div><div class="value">{_html_text(snapshot.get('consumed_games'))}</div></div>
</div></section>
<section><h2>Candidates</h2><p>Completed: {_html_text(counts.get('completed', 0))} · rejected: {_html_text(counts.get('rejected', 0))} · waiting: {_html_text(counts.get('waiting', 0))}</p>
<table><thead><tr><th>Trial</th><th>Status</th><th>W–D–L</th><th>Score</th><th>Parameter hash</th></tr></thead><tbody>{rows}</tbody></table></section>
<section><h2>Best parameters vs baseline</h2><table><thead><tr><th>Parameter</th><th>Baseline</th><th>Best</th><th>Delta</th></tr></thead><tbody>{parameter_rows}</tbody></table></section>
<section><h2>Latest checkpoint</h2><pre>{html.escape(checkpoint)}</pre><h2>Latest error</h2><pre>{html.escape(json.dumps(latest_error, ensure_ascii=False, indent=2))}</pre></section>
<footer>Generated {_html_text(snapshot.get('generated_at'))}; SQLite source opened read-only.</footer></body></html>"""


_DASHBOARD_HTML = r"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GoAlaric optimizer dashboard</title>
<style>
:root{color-scheme:light;--ink:#17202a;--muted:#617080;--line:#d9dee5;--panel:#fff;--bg:#f4f6f8;--accent:#1b6ca8}
*{box-sizing:border-box}body{font:15px system-ui,sans-serif;max-width:1280px;margin:0 auto;padding:1.2rem;color:var(--ink);background:var(--bg)}
header{display:flex;justify-content:space-between;gap:1rem;align-items:baseline;flex-wrap:wrap}h1{margin:.2rem 0;font-size:1.7rem}h2{font-size:1.1rem;margin:.1rem 0 .8rem}h3{font-size:1rem;margin:0 0 .4rem}.muted,.label{color:var(--muted)}
section{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:1rem;margin:1rem 0;box-shadow:0 1px 2px #00000008}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:.7rem}.card{background:#eef4f8;border-radius:8px;padding:.75rem}.value{font-size:1.25rem;font-weight:650;margin-top:.25rem;word-break:break-word}
table{border-collapse:collapse;width:100%}th,td{padding:.46rem;border-bottom:1px solid #e4e7eb;text-align:left;vertical-align:top;font-size:.88rem}th{color:var(--muted);font-weight:600}.scroll{overflow:auto}pre{overflow:auto;max-height:18rem;background:#111827;color:#e5e7eb;padding:.8rem;border-radius:8px;font-size:.78rem}.ok{color:#177245}.warn{color:#9a5b00}.bad{color:#a12622}a{color:var(--accent)}
</style></head><body>
<header><div><h1>GoAlaric optimizer</h1><div id="campaign-name" class="muted">loading…</div></div><div><span id="readonly" class="ok">read-only</span> · <a href="/report">final report</a></div></header>
<section><h2>Campaign</h2><div class="grid"><div class="card"><div class="label">Status</div><div id="status" class="value">—</div></div><div class="card"><div class="label">Current trial</div><div id="current-trial" class="value">—</div></div><div class="card"><div class="label">Score</div><div id="score" class="value">—</div></div><div class="card"><div class="label">Elo</div><div id="elo" class="value">—</div></div><div class="card"><div class="label">95% CI (score)</div><div id="score-ci" class="value">—</div></div><div class="card"><div class="label">95% CI (Elo)</div><div id="elo-ci" class="value">—</div></div><div class="card"><div class="label">W–D–L</div><div id="wdl" class="value">—</div></div><div class="card"><div class="label">Consumed games</div><div id="games" class="value">—</div></div></div><p id="updated" class="muted"></p></section>
<section><h2>Candidate queue</h2><div class="grid"><div class="card"><div class="label">Completed</div><div id="completed-count" class="value">—</div></div><div class="card"><div class="label">Rejected</div><div id="rejected-count" class="value">—</div></div><div class="card"><div class="label">Waiting</div><div id="waiting-count" class="value">—</div></div></div><div class="scroll"><table><thead><tr><th>Trial</th><th>Status</th><th>Algorithm</th><th>W–D–L</th><th>Score</th><th>Elo</th><th>Parameter hash</th></tr></thead><tbody id="candidates"></tbody></table></div></section>
<section><h2>Current trial</h2><div id="current-details" class="muted">—</div></section>
<section><h2>Best parameter set vs baseline</h2><p id="best-source" class="muted">—</p><div class="scroll"><table><thead><tr><th>Parameter</th><th>Baseline</th><th>Best</th><th>Delta</th></tr></thead><tbody id="parameters"></tbody></table></div></section>
<section><h2>Checkpoint and latest error</h2><pre id="checkpoint">—</pre><pre id="error">—</pre></section>
<script>
const refreshMs=__REFRESH_MS__;
const h=value=>String(value===null||value===undefined||value===''?'—':value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const metric=(m,k)=>m&&m[k]!==undefined?m[k]:'—';
function render(data){
 const c=data.campaign||{}, t=data.current_trial||{}, m=t.metrics||data.campaign_metrics||{}, counts=data.candidate_counts||{};
 document.title='GoAlaric · '+(c.name||c.campaign_id||'dashboard');
 document.getElementById('campaign-name').textContent=(c.name||'—')+' · '+(c.campaign_id||'—');
 document.getElementById('status').textContent=c.status||'—';
 document.getElementById('current-trial').textContent=t.trial_id||'—';
 document.getElementById('score').textContent=h(metric(m,'score_percent'))+'%';
 document.getElementById('elo').textContent=h(metric(m,'elo_estimate'));
 document.getElementById('score-ci').textContent=h(metric(m,'score_ci_low'))+'% … '+h(metric(m,'score_ci_high'))+'%';
 document.getElementById('elo-ci').textContent=h(metric(m,'elo_ci_low'))+' … '+h(metric(m,'elo_ci_high'));
 document.getElementById('wdl').textContent=h(metric(m,'wins'))+'–'+h(metric(m,'draws'))+'–'+h(metric(m,'losses'));
 document.getElementById('games').textContent=h(data.consumed_games);
 document.getElementById('updated').textContent='Last refresh: '+(data.generated_at||'—')+' · database: '+((data.database||{}).path||'—');
 ['completed','rejected','waiting'].forEach(k=>document.getElementById(k+'-count').textContent=h(counts[k]||0));
 document.getElementById('candidates').innerHTML=(data.candidates||[]).map(item=>{const x=item.metrics||{};return '<tr><td>'+h(item.trial_id)+'</td><td>'+h(item.status)+'</td><td>'+h(item.algorithm)+'</td><td>'+h(x.wins)+'–'+h(x.draws)+'–'+h(x.losses)+'</td><td>'+h(x.score_percent)+'%</td><td>'+h(x.elo_estimate)+'</td><td>'+h((item.parameter||{}).parameter_hash)+'</td></tr>';}).join('');
 const block=t.current_block; document.getElementById('current-details').innerHTML='<strong>Status:</strong> '+h(t.status)+' · <strong>parameter:</strong> '+h((t.parameter||{}).parameter_hash)+' · <strong>next/current block:</strong> '+h(block?block.block_index:'—')+' · <strong>attempt:</strong> '+h(block?block.attempt:'—')+'<br><strong>algorithm:</strong> '+h(t.algorithm)+' · <strong>error:</strong> '+h(t.error);
 const best=data.best_parameters||{};document.getElementById('best-source').textContent='Source: '+(best.source||'—')+' · trial: '+(best.trial_id||'—')+' · hash: '+(best.parameter_hash||'—');
 document.getElementById('parameters').innerHTML=(data.parameter_differences||[]).map(item=>'<tr><td>'+h(item.name)+'</td><td>'+h(item.baseline)+'</td><td>'+h(item.best)+'</td><td>'+h(item.delta)+'</td></tr>').join('');
 document.getElementById('checkpoint').textContent=JSON.stringify(data.checkpoint||null,null,2);
 document.getElementById('error').textContent=JSON.stringify(data.latest_error||null,null,2);
 document.getElementById('readonly').textContent=data.read_only?'read-only':'unknown mode';
}
async function refresh(){try{const response=await fetch('/api/dashboard',{cache:'no-store'});const data=await response.json();if(!response.ok)throw new Error(data.error||response.statusText);render(data);}catch(error){document.getElementById('status').textContent='dashboard error';document.getElementById('status').className='value bad';document.getElementById('updated').textContent=error.message;}}
refresh();setInterval(refresh,refreshMs);
</script></body></html>"""


class _DashboardHandler(BaseHTTPRequestHandler):
    reader: DashboardReader
    refresh_ms: int

    def log_message(self, format: str, *args: Any) -> None:
        return

    def _send(self, body: bytes, content_type: str, status: int = HTTPStatus.OK) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_json(self, value: Any, status: int = HTTPStatus.OK) -> None:
        self._send(json.dumps(value, ensure_ascii=False, indent=2).encode("utf-8"), "application/json; charset=utf-8", status)

    def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        route = urlparse(self.path)
        try:
            if route.path in {"/", "/index.html"}:
                body = _DASHBOARD_HTML.replace("__REFRESH_MS__", str(self.refresh_ms)).encode("utf-8")
                self._send(body, "text/html; charset=utf-8")
                return
            snapshot = self.reader.snapshot()
            if route.path == "/api/dashboard":
                self._send_json(snapshot)
                return
            if route.path == "/api/report":
                if not snapshot["campaign"]["finished"]:
                    self._send_json({"error": "campaign is not finished"}, HTTPStatus.CONFLICT)
                else:
                    self._send_json(snapshot)
                return
            if route.path in {"/report", "/report.html"}:
                if not snapshot["campaign"]["finished"]:
                    self._send_json({"error": "campaign is not finished"}, HTTPStatus.CONFLICT)
                else:
                    self._send(render_report_html(snapshot).encode("utf-8"), "text/html; charset=utf-8")
                return
            self._send_json({"error": "not found"}, HTTPStatus.NOT_FOUND)
        except (DashboardError, OSError, sqlite3.Error) as exc:
            self._send_json({"error": str(exc)}, HTTPStatus.SERVICE_UNAVAILABLE)


def create_dashboard_server(
    data_dir: Path, campaign_id: str, listen: str = "127.0.0.1:8787", refresh_ms: int = 2000
) -> ThreadingHTTPServer:
    if refresh_ms < 250:
        raise DashboardError("dashboard refresh interval must be at least 250 ms")
    address = _listen_address(listen)
    reader = DashboardReader(data_dir, campaign_id)
    handler = type("DashboardHandler", (_DashboardHandler,), {"reader": reader, "refresh_ms": refresh_ms})
    server = ThreadingHTTPServer(address, handler)
    server.daemon_threads = True
    setattr(server, "dashboard_reader", reader)
    return server


def serve_dashboard(
    data_dir: Path, campaign_id: str, listen: str = "127.0.0.1:8787", refresh_ms: int = 2000
) -> None:
    server = create_dashboard_server(data_dir, campaign_id, listen, refresh_ms)
    host, port = server.server_address[:2]
    print(f"dashboard listening on http://{host}:{port}", flush=True)

    def stop(_signum: int, _frame: Any) -> None:
        threading.Thread(target=server.shutdown, daemon=True).start()

    previous_int = signal.signal(signal.SIGINT, stop)
    previous_term = signal.signal(signal.SIGTERM, stop)
    try:
        server.serve_forever(poll_interval=0.2)
    finally:
        signal.signal(signal.SIGINT, previous_int)
        signal.signal(signal.SIGTERM, previous_term)
        server.server_close()


def final_report(data_dir: Path, campaign_id: str, report_format: str = "html") -> tuple[dict[str, Any], str]:
    if report_format not in {"html", "json"}:
        raise DashboardError("report format must be html or json")
    snapshot = DashboardReader(data_dir, campaign_id).snapshot()
    if not snapshot["campaign"]["finished"]:
        raise DashboardError("campaign is not finished; final report is unavailable")
    if report_format == "json":
        content = json.dumps(snapshot, ensure_ascii=False, indent=2) + "\n"
    else:
        content = render_report_html(snapshot)
    return snapshot, content
