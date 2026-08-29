"""Finite, noise-aware Bayesian search for GoAlaric parameter grids."""

from __future__ import annotations

import itertools
import math
from dataclasses import dataclass
from typing import Iterable, Sequence


class BayesianSearchError(RuntimeError):
    pass


@dataclass(frozen=True)
class Dimension:
    name: str
    values: tuple[int, ...]
    default: int

    def __post_init__(self) -> None:
        if not self.values or tuple(sorted(set(self.values))) != self.values:
            raise BayesianSearchError(f"{self.name} values must be sorted and unique")
        if self.default not in self.values:
            raise BayesianSearchError(f"{self.name} default is outside its values")


@dataclass(frozen=True)
class Observation:
    values: tuple[int, ...]
    score: float
    variance: float
    pairs: int

    def __post_init__(self) -> None:
        if not 0.0 <= self.score <= 1.0 or not math.isfinite(self.score):
            raise BayesianSearchError("score must be finite and between zero and one")
        if self.variance <= 0.0 or not math.isfinite(self.variance):
            raise BayesianSearchError("variance must be finite and positive")
        if self.pairs < 2:
            raise BayesianSearchError("an observation needs at least two opening pairs")


def pentanomial_score_variance(pair_points: Sequence[float]) -> tuple[float, float]:
    """Return score fraction and variance of its mean from paired outcomes."""
    if len(pair_points) < 2:
        raise BayesianSearchError("at least two opening pairs are required")
    if any(value not in {0.0, 0.5, 1.0, 1.5, 2.0} for value in pair_points):
        raise BayesianSearchError("pair points must be one of 0, 0.5, 1, 1.5, 2")
    samples = [value / 2.0 for value in pair_points]
    mean = sum(samples) / len(samples)
    sample_variance = sum((value - mean) ** 2 for value in samples) / (len(samples) - 1)
    return mean, max(sample_variance / len(samples), 1e-8)


class FiniteBayesianSearch:
    """Choose unobserved points from a finite grid using known-noise qLogNEI."""

    ALGORITHM = "finite-noise-aware-bo-v1"

    def __init__(self, dimensions: Sequence[Dimension], seed: int, initial_points: int = 9) -> None:
        if not dimensions:
            raise BayesianSearchError("at least one dimension is required")
        if initial_points < 1:
            raise BayesianSearchError("initial_points must be positive")
        self.dimensions = tuple(dimensions)
        self.seed = int(seed)
        self.grid = tuple(itertools.product(*(dimension.values for dimension in self.dimensions)))
        self.initial_points = min(initial_points, len(self.grid))
        self._initial_order = self._build_initial_order()

    def _normalize(self, values: tuple[int, ...]) -> tuple[float, ...]:
        result = []
        for value, dimension in zip(values, self.dimensions, strict=True):
            low, high = dimension.values[0], dimension.values[-1]
            result.append(0.0 if low == high else (value - low) / (high - low))
        return tuple(result)

    def _build_initial_order(self) -> tuple[tuple[int, ...], ...]:
        default = tuple(dimension.default for dimension in self.dimensions)
        selected = [default]
        remaining = set(self.grid) - {default}
        while remaining and len(selected) < self.initial_points:
            point = max(
                remaining,
                key=lambda candidate: (
                    min(
                        sum((a - b) ** 2 for a, b in zip(self._normalize(candidate), self._normalize(old), strict=True))
                        for old in selected
                    ),
                    tuple(-value for value in candidate),
                ),
            )
            selected.append(point)
            remaining.remove(point)
        return tuple(selected)

    def ask(self, observations: Iterable[Observation]) -> tuple[int, ...]:
        observed = list(observations)
        by_values = {item.values: item for item in observed}
        for point in self._initial_order:
            if point not in by_values:
                return point
        candidates = [point for point in self.grid if point not in by_values]
        if not candidates:
            raise BayesianSearchError("all parameter combinations have been observed")

        try:
            import torch
            from botorch import fit_gpytorch_mll
            from botorch.acquisition.logei import qLogNoisyExpectedImprovement
            from botorch.models import SingleTaskGP
            from botorch.sampling.normal import SobolQMCNormalSampler
            from gpytorch.mlls import ExactMarginalLogLikelihood
        except ImportError as exc:
            raise BayesianSearchError("install goalaric-optimizer[bayes]") from exc

        torch.manual_seed(self.seed + len(observed))
        dtype = torch.double
        train_x = torch.tensor([self._normalize(item.values) for item in observed], dtype=dtype)
        train_y = torch.tensor([[item.score] for item in observed], dtype=dtype)
        train_yvar = torch.tensor([[item.variance] for item in observed], dtype=dtype)
        model = SingleTaskGP(train_X=train_x, train_Y=train_y, train_Yvar=train_yvar)
        fit_gpytorch_mll(ExactMarginalLogLikelihood(model.likelihood, model))
        sampler = SobolQMCNormalSampler(sample_shape=torch.Size([256]), seed=self.seed)
        acquisition = qLogNoisyExpectedImprovement(
            model=model,
            X_baseline=train_x,
            sampler=sampler,
            prune_baseline=False,
        )
        candidate_x = torch.tensor([self._normalize(point) for point in candidates], dtype=dtype).unsqueeze(1)
        with torch.no_grad():
            values = acquisition(candidate_x)
        best = max(range(len(candidates)), key=lambda index: (float(values[index]), tuple(-v for v in candidates[index])))
        return candidates[best]
