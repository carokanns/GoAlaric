"""Deterministic W-D-L, score and Elo estimates for adaptive match gating."""

from __future__ import annotations

import math
from typing import Any, Iterable


Z95 = 1.959963984540054


def _elo_from_score(score_percent: float) -> float:
    score = min(99.99, max(0.01, float(score_percent))) / 100.0
    return 400.0 * math.log10(score / (1.0 - score))


def aggregate_wdl(blocks: Iterable[dict[str, Any]]) -> dict[str, Any]:
    """Aggregate only completed blocks, preserving their deterministic order."""
    completed = sorted(
        (block for block in blocks if block.get("status") == "completed"),
        key=lambda block: (int(block["block_index"]), str(block["block_id"])),
    )
    wins = sum(int(block["wins"]) for block in completed)
    draws = sum(int(block["draws"]) for block in completed)
    losses = sum(int(block["losses"]) for block in completed)
    games = wins + draws + losses
    if games == 0:
        return {
            "blocks_completed": 0,
            "games": 0,
            "wins": 0,
            "draws": 0,
            "losses": 0,
            "score": 0.0,
            "score_percent": 0.0,
            "score_ci_low": 0.0,
            "score_ci_high": 100.0,
            "elo_estimate": 0.0,
            "elo_ci_low": -800.0,
            "elo_ci_high": 800.0,
            "uncertainty": 50.0,
            "block_ids": [],
        }

    points = wins + draws / 2.0
    score = points / games
    values = [1.0] * wins + [0.5] * draws + [0.0] * losses
    variance = sum((value - score) ** 2 for value in values) / games
    standard_error = math.sqrt(variance / games)
    low = max(0.0, score - Z95 * standard_error)
    high = min(1.0, score + Z95 * standard_error)
    # A continuity correction keeps two-game all-win/all-loss blocks finite.
    corrected_score = (points + 0.5) / (games + 1.0)
    result = {
        "blocks_completed": len(completed),
        "games": games,
        "wins": wins,
        "draws": draws,
        "losses": losses,
        "score": round(score * 100.0, 8),
        "score_percent": round(score * 100.0, 8),
        "score_ci_low": round(low * 100.0, 8),
        "score_ci_high": round(high * 100.0, 8),
        "elo_estimate": round(_elo_from_score(corrected_score * 100.0), 8),
        "elo_ci_low": round(_elo_from_score(low * 100.0), 8),
        "elo_ci_high": round(_elo_from_score(high * 100.0), 8),
        "uncertainty": round((high - low) * 50.0, 8),
        "block_ids": [str(block["block_id"]) for block in completed],
    }
    return result
