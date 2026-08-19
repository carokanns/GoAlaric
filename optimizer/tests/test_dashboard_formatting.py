from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "src"))

from goalaric_optimizer.dashboard import _DASHBOARD_HTML, _html_number, render_report_html


class DashboardFormattingTest(unittest.TestCase):
    def test_dashboard_number_formatting_rounds_only_the_web_display(self) -> None:
        self.assertEqual(_html_number(51.40625, 1), "51.4")
        self.assertEqual(_html_number(10.625, 0), "11")
        self.assertEqual(_html_number(-13.4, 0), "-13")
        self.assertEqual(_html_number(None, 1), "—")

        snapshot = {
            "campaign": {"name": "formatting", "campaign_id": "formatting", "status": "completed"},
            "campaign_metrics": {
                "wins": 10,
                "draws": 20,
                "losses": 10,
                "score_percent": 51.40625,
                "elo_estimate": 10.625,
                "score_ci_low": 44.90494158,
                "score_ci_high": 55.59505842,
                "elo_ci_low": -13.4,
                "elo_ci_high": 33.8,
            },
            "candidates": [
                {
                    "trial_id": "trial-1",
                    "status": "completed",
                    "metrics": {"score_percent": 61.71875, "elo_estimate": 42.6},
                }
            ],
        }
        report = render_report_html(snapshot)
        self.assertIn("51.4%", report)
        self.assertIn("11", report)
        self.assertIn("44.9% … 55.6%", report)
        self.assertIn("-13 … 34", report)
        self.assertIn("61.7%", report)

        # The live dashboard uses the same presentation policy in its browser code.
        self.assertIn("numberText(metric(m,'score_percent'),1)", _DASHBOARD_HTML)
        self.assertIn("numberText(metric(m,'elo_estimate'),0)", _DASHBOARD_HTML)
