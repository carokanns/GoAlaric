from __future__ import annotations

import unittest
import random

from goalaric_optimizer.bayes import (
    BayesianSearchError,
    Dimension,
    FiniteBayesianSearch,
    Observation,
    pentanomial_score_variance,
)


class Phase23NoiseAwareBayesianTest(unittest.TestCase):
    def setUp(self) -> None:
        self.dimensions = (
            Dimension("lmr", (175, 225, 275), 225),
            Dimension("lmp", (3, 4, 5), 4),
        )

    def test_pentanomial_variance_shrinks_with_repeated_pairs(self) -> None:
        points = [0.0, 0.5, 1.0, 1.5, 2.0] * 4
        score, variance = pentanomial_score_variance(points)
        repeated_score, repeated_variance = pentanomial_score_variance(points * 4)
        self.assertEqual(score, 0.5)
        self.assertEqual(repeated_score, score)
        self.assertLess(repeated_variance, variance)

    def test_initial_design_is_deterministic_and_starts_at_baseline(self) -> None:
        first = FiniteBayesianSearch(self.dimensions, seed=20260829, initial_points=5)
        second = FiniteBayesianSearch(self.dimensions, seed=20260829, initial_points=5)
        observations: list[Observation] = []
        sequence = []
        for _ in range(5):
            point = first.ask(observations)
            sequence.append(point)
            observations.append(Observation(point, 0.5, 0.01, 16))
        replay = []
        replay_observations: list[Observation] = []
        for _ in range(5):
            point = second.ask(replay_observations)
            replay.append(point)
            replay_observations.append(Observation(point, 0.5, 0.01, 16))
        self.assertEqual(sequence, replay)
        self.assertEqual(sequence[0], (225, 4))
        self.assertEqual(len(set(sequence)), len(sequence))

    def test_known_noise_gp_proposal_is_reproducible(self) -> None:
        search = FiniteBayesianSearch(self.dimensions, seed=7, initial_points=1)
        observations = [
            Observation((225, 4), 0.50, 0.002, 64),
            Observation((175, 3), 0.48, 0.003, 64),
            Observation((275, 5), 0.52, 0.001, 64),
        ]
        self.assertEqual(search.ask(observations), search.ask(observations))

    def test_noise_aware_search_beats_random_selection_on_interacting_surface(self) -> None:
        dimensions = (
            Dimension("x", (0, 1, 2, 3, 4), 2),
            Dimension("y", (0, 1, 2, 3, 4), 2),
        )

        def objective(point: tuple[int, ...]) -> float:
            x, y = point
            return 0.54 - 0.006 * (x - 4) ** 2 - 0.008 * (y - 1) ** 2 + 0.002 * (x - 2) * (y - 2)

        search = FiniteBayesianSearch(dimensions, seed=1, initial_points=5)
        observations: list[Observation] = []
        for _ in range(8):
            point = search.ask(observations)
            noise = random.Random(100 + point[0] * 10 + point[1]).gauss(0.0, 0.006)
            observations.append(Observation(point, objective(point) + noise, 0.006**2, 64))
        recommendation = max(observations, key=lambda item: item.score).values
        optimum = max(search.grid, key=objective)
        bayesian_regret = objective(optimum) - objective(recommendation)

        initial = {item.values for item in observations[:5]}
        remaining = [point for point in search.grid if point not in initial]
        random_regrets = []
        for seed in range(32):
            sampled = list(initial) + random.Random(seed).sample(remaining, 3)
            random_regrets.append(objective(optimum) - max(objective(point) for point in sampled))
        random_regrets.sort()
        self.assertLess(bayesian_regret, random_regrets[len(random_regrets) // 2])

    def test_invalid_pair_result_is_rejected(self) -> None:
        with self.assertRaises(BayesianSearchError):
            pentanomial_score_variance([1.0, 1.25])


if __name__ == "__main__":
    unittest.main()
