#!/usr/bin/env python3
"""Reproduce the phase-23 finite-grid Bayesian search benchmark."""

from __future__ import annotations

import json
import random

from goalaric_optimizer.bayes import Dimension, FiniteBayesianSearch, Observation


def objective(point: tuple[int, ...]) -> float:
    x, y = point
    return 0.54 - 0.006 * (x - 4) ** 2 - 0.008 * (y - 1) ** 2 + 0.002 * (x - 2) * (y - 2)


def main() -> None:
    dimensions = (
        Dimension("x", (0, 1, 2, 3, 4), 2),
        Dimension("y", (0, 1, 2, 3, 4), 2),
    )
    search = FiniteBayesianSearch(dimensions, seed=1, initial_points=5)
    observations: list[Observation] = []
    for _ in range(8):
        point = search.ask(observations)
        noise = random.Random(100 + point[0] * 10 + point[1]).gauss(0.0, 0.006)
        observations.append(Observation(point, objective(point) + noise, 0.006**2, 64))
    optimum = max(search.grid, key=objective)
    recommendation = max(observations, key=lambda item: item.score).values
    initial = {item.values for item in observations[:5]}
    remaining = [point for point in search.grid if point not in initial]
    random_regrets = sorted(
        objective(optimum)
        - max(objective(point) for point in list(initial) + random.Random(seed).sample(remaining, 3))
        for seed in range(32)
    )
    print(
        json.dumps(
            {
                "schema_version": 1,
                "algorithm": search.ALGORITHM,
                "evaluations": len(observations),
                "optimum": optimum,
                "recommendation": recommendation,
                "bayesian_regret": round(objective(optimum) - objective(recommendation), 8),
                "random_median_regret": round(random_regrets[len(random_regrets) // 2], 8),
                "sequence": [item.values for item in observations],
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
