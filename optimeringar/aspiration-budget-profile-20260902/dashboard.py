#!/usr/bin/env python3
import argparse
import json
import math
import re
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

EXPERIMENT = "aspiration-budget-profile-20260902"
ARTIFACT = Path("/home/peter/Projekt/GoAlaric/artifacts/experiments") / EXPERIMENT
MATRIX = Path(__file__).with_name("matrix.json")


def iso(value):
    if not value:
        return None
    return datetime.fromisoformat(str(value).replace("Z", "+00:00"))


def duration(seconds):
    if seconds is None or not math.isfinite(seconds) or seconds < 0:
        return "—"
    hours, rest = divmod(int(seconds), 3600)
    minutes, secs = divmod(rest, 60)
    return f"{hours}h {minutes:02d}m {secs:02d}s"


def metrics(wins, draws, losses):
    games = wins + draws + losses
    if not games:
        return {"score": 0.0, "score_low": 0.0, "score_high": 100.0, "elo": 0.0, "elo_low": -800.0, "elo_high": 800.0}
    values = wins + draws * 0.5
    score = values / games
    second = (wins + draws * 0.25) / games
    variance = max(0.0, second - score * score) / games
    delta = 1.96 * math.sqrt(variance)
    low, high = max(0.0, score - delta), min(1.0, score + delta)

    def elo(point):
        point = min(max(point, 0.0001), 0.9999)
        return 400.0 * math.log10(point / (1.0 - point))

    return {
        "score": 100.0 * score,
        "score_low": 100.0 * low,
        "score_high": 100.0 * high,
        "elo": elo(score),
        "elo_low": elo(low),
        "elo_high": elo(high),
    }


def snapshot():
    plan = json.loads(MATRIX.read_text(encoding="utf-8"))
    cells = []
    starts, finishes = [], []
    total_games = total_wins = total_draws = total_losses = 0
    active = None
    runs = ARTIFACT / "runs"
    for nodes in plan["node_budgets_per_move"]:
        for margin in plan["candidate_margins_cp"]:
            name = f"nodes-{nodes}-margin-{margin:02d}"
            status_path = runs / name / "status.json"
            profile_path = runs / name / "aspiration-profile.json"
            status = {}
            profile = {}
            if status_path.exists():
                try:
                    status = json.loads(status_path.read_text(encoding="utf-8"))
                except (OSError, json.JSONDecodeError):
                    pass
            if profile_path.exists():
                try:
                    profile = json.loads(profile_path.read_text(encoding="utf-8"))
                except (OSError, json.JSONDecodeError):
                    pass
            state = status.get("state", "queued")
            games = int(status.get("games", 0))
            wins = int(status.get("wins", 0))
            draws = int(status.get("draws", 0))
            losses = int(status.get("losses", 0))
            values = metrics(wins, draws, losses)
            started = iso(status.get("started_at"))
            finished = iso(status.get("finished_at"))
            if started:
                starts.append(started)
            if finished:
                finishes.append(finished)
            total_games += games
            total_wins += wins
            total_draws += draws
            total_losses += losses
            cell = {
                "name": name,
                "nodes": nodes,
                "margin": margin,
                "state": state,
                "games": games,
                "target": plan["games_per_cell"],
                "wins": wins,
                "draws": draws,
                "losses": losses,
                **values,
                "median_depth": profile.get("median_depth"),
                "fail_percent": profile.get("aspiration_failure_percent"),
                "overhead_percent": profile.get("aspiration_overhead_percent"),
                "retries": profile.get("aspiration_retries"),
            }
            cells.append(cell)
            if state in {"starting", "running", "stopping"}:
                active = cell

    now = datetime.now(timezone.utc)
    elapsed = (now - min(starts)).total_seconds() if starts else None
    rate = total_games / elapsed if elapsed and total_games else 0.0
    remaining = (plan["total_games"] - total_games) / rate if rate > 0 else None
    overall = "completed" if total_games == plan["total_games"] and all(c["state"] == "completed" for c in cells) else (active["state"] if active else "waiting")
    return {
        "experiment": EXPERIMENT,
        "status": overall,
        "games": total_games,
        "target": plan["total_games"],
        "completed_cells": sum(c["state"] == "completed" for c in cells),
        "total_cells": len(cells),
        "elapsed": duration(elapsed),
        "remaining": duration(remaining),
        "active": active,
        "cells": cells,
        "updated_at": now.isoformat(),
    }


HTML = r'''<!doctype html><html lang="sv"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>GoAlaric aspiration profile</title><style>
:root{font-family:system-ui,sans-serif;color:#182433;background:#f3f6f9}body{margin:0;padding:16px}main{max-width:1400px;margin:auto}h1{margin:0}.sub{color:#64748b;margin:4px 0 20px}.panel{background:white;border:1px solid #d6dee8;border-radius:14px;padding:20px;margin:14px 0;box-shadow:0 1px 2px #0000000d}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(145px,1fr));gap:14px}.card{background:#edf3f7;border-radius:10px;padding:14px}.label{color:#64748b;font-size:.9rem}.value{font-size:1.45rem;font-weight:700;margin-top:5px}.active{border:2px solid #167c5a}table{border-collapse:collapse;width:100%;font-size:.92rem}th,td{text-align:left;padding:9px;border-bottom:1px solid #e2e8f0}th{color:#526174}.running{color:#08775b;font-weight:700}.completed{color:#3156a3}.failed{color:#b42318;font-weight:700}@media(max-width:700px){table{font-size:.78rem}th,td{padding:6px 3px}.hide-small{display:none}}
</style></head><body><main><h1>GoAlaric aspiration profile</h1><div class="sub">5 marginaler × 3 nodbudgetar · read-only · uppdateras varannan sekund</div><section class="panel"><div id="summary" class="grid"></div></section><section id="active-panel" class="panel active"><h2>Aktuell cell</h2><div id="active" class="grid"></div></section><section class="panel"><h2>Matris</h2><div style="overflow:auto"><table><thead><tr><th>Nodes/drag</th><th>Marginal</th><th>Status</th><th>Games</th><th>W–D–L</th><th>Score</th><th>Elo 95% CI</th><th>Djup</th><th class="hide-small">Fail</th><th class="hide-small">Omsökningsnoder</th></tr></thead><tbody id="rows"></tbody></table></div></section><div class="sub" id="refresh"></div></main><script>
const esc=x=>String(x??'—').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
const card=(a,b)=>`<div class="card"><div class="label">${esc(a)}</div><div class="value">${esc(b)}</div></div>`;
const num=(x,d=1)=>x==null?'—':Number(x).toFixed(d);
async function update(){const d=await fetch('/api/status',{cache:'no-store'}).then(r=>r.json());
document.getElementById('summary').innerHTML=card('Status',d.status)+card('Total games',`${d.games} / ${d.target}`)+card('Cells',`${d.completed_cells} / ${d.total_cells}`)+card('Elapsed',d.elapsed)+card('Estimated remaining',d.remaining);
const a=d.active;document.getElementById('active-panel').style.display=a?'block':'none';if(a)document.getElementById('active').innerHTML=card('Nodes/move',a.nodes)+card('Margin',`${a.margin} cp`)+card('Games',`${a.games} / ${a.target}`)+card('W–D–L',`${a.wins}–${a.draws}–${a.losses}`)+card('Score',`${num(a.score)}%`)+card('Elo',`${num(a.elo,0)} (${num(a.elo_low,0)} … ${num(a.elo_high,0)})`);
document.getElementById('rows').innerHTML=d.cells.map(c=>`<tr><td>${c.nodes}</td><td>${c.margin} cp</td><td class="${esc(c.state)}">${esc(c.state)}</td><td>${c.games}/${c.target}</td><td>${c.wins}–${c.draws}–${c.losses}</td><td>${num(c.score)}%</td><td>${num(c.elo_low,0)} … ${num(c.elo_high,0)}</td><td>${esc(c.median_depth)}</td><td class="hide-small">${c.fail_percent==null?'—':num(c.fail_percent)+'%'}</td><td class="hide-small">${c.overhead_percent==null?'—':num(c.overhead_percent)+'%'}</td></tr>`).join('');document.getElementById('refresh').textContent='Senast uppdaterad: '+d.updated_at;}update();setInterval(update,2000);
</script></body></html>'''


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/api/status":
            body = json.dumps(snapshot(), ensure_ascii=False).encode()
            content_type = "application/json; charset=utf-8"
        elif self.path in {"/", "/index.html"}:
            body = HTML.encode()
            content_type = "text/html; charset=utf-8"
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8804)
    args = parser.parse_args()
    ThreadingHTTPServer((args.listen, args.port), Handler).serve_forever()
