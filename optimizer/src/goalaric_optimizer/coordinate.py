from __future__ import annotations

import hashlib
import json
import math
import random
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from .canonical import sha256_json
from .database import Database
from .registry import Registry, ValidationError, normalize_parameter_document
from .service import campaign_lock, load_database


class CoordinateSearchError(RuntimeError):
    pass


Evaluator = Callable[[dict[str, Any], int], dict[str, Any]]


def _jsonable(value: Any) -> Any:
    if isinstance(value, tuple):
        return [_jsonable(item) for item in value]
    if isinstance(value, list):
        return [_jsonable(item) for item in value]
    return value


def _tuple_state(value: Any) -> Any:
    if isinstance(value, list):
        return tuple(_tuple_state(item) for item in value)
    return value


def _stable_trial_seed(campaign_id: str, parameter_hash: str, master_seed: int) -> int:
    digest = hashlib.sha256(f"{campaign_id}:{master_seed}:{parameter_hash}".encode("utf-8")).digest()
    return int.from_bytes(digest[:8], "big", signed=False) & ((1 << 63) - 1)


@dataclass(frozen=True)
class ParameterSpec:
    name: str
    minimum: int
    maximum: int
    step: int
    min_step: int = 1


def _parameter_specs(registry: Registry) -> tuple[ParameterSpec, ...]:
    specs: list[ParameterSpec] = []
    for item in registry.parameters:
        name = str(item["name"])
        default = int(item["value"])
        minimum = item.get("min", default)
        maximum = item.get("max", default)
        step = item.get("step", 1)
        min_step = item.get("min_step", 1)
        if any(
            isinstance(value, bool) or not isinstance(value, int)
            for value in (minimum, maximum, step, min_step)
        ):
            raise ValidationError(f"coordinate bounds for {name} must be integers")
        if (
            step <= 0
            or min_step <= 0
            or min_step > step
            or minimum > maximum
            or not minimum <= default <= maximum
        ):
            raise ValidationError(f"invalid coordinate bounds for {name}")
        specs.append(ParameterSpec(name, minimum, maximum, step, min_step))
    return tuple(specs)


def _parameter_values(document: dict[str, Any]) -> dict[str, int]:
    return {str(item["name"]): int(item["value"]) for item in document["parameters"]}


def _with_value(document: dict[str, Any], name: str, value: int) -> dict[str, Any]:
    result = {"schema_version": document["schema_version"], "registry": document["registry"], "parameters": []}
    for item in document["parameters"]:
        result["parameters"].append(
            {"name": item["name"], "value": value if item["name"] == name else item["value"]}
        )
    return result


def _normalize_result(value: dict[str, Any]) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise CoordinateSearchError("coordinate evaluator must return an object")
    counts = [value.get(name) for name in ("wins", "draws", "losses")]
    if any(not isinstance(item, int) or isinstance(item, bool) or item < 0 for item in counts):
        raise CoordinateSearchError("coordinate W-D-L must be non-negative integers")
    games = sum(counts)
    if games < 1:
        raise CoordinateSearchError("coordinate result must contain at least one game")
    score = value.get("score")
    if score is None:
        score = (counts[0] + counts[1] / 2) / games * 100
    if not isinstance(score, (int, float)) or isinstance(score, bool) or not math.isfinite(float(score)):
        raise CoordinateSearchError("coordinate score must be a finite number")
    uncertainty = value.get("uncertainty", 0.0)
    if not isinstance(uncertainty, (int, float)) or isinstance(uncertainty, bool) or uncertainty < 0:
        raise CoordinateSearchError("coordinate uncertainty must be non-negative")
    normalized = dict(value)
    normalized.update(
        {
            "wins": counts[0],
            "draws": counts[1],
            "losses": counts[2],
            "games": games,
            "score": round(float(score), 8),
            "uncertainty": round(float(uncertainty), 8),
            "uncertain": bool(value.get("uncertain", False)),
        }
    )
    return normalized


class CoordinateSearch:
    """Deterministic one-step-at-a-time coordinate search.

    Each coordinate evaluates +step and -step from the same anchor. A clear
    win is eligible for selection; losses and uncertain results never move the
    anchor. When a pass improves the anchor, another deterministic pass starts.
    """

    ALGORITHM = "coordinate-v1"

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        registry: Registry,
        evaluator: Evaluator,
        max_passes: int = 100,
    ) -> None:
        if max_passes < 1:
            raise CoordinateSearchError("max_passes must be positive")
        self.database = database
        self.campaign_id = campaign_id
        self.registry = registry
        self.evaluator = evaluator
        self.specs = _parameter_specs(registry)
        self.max_passes = max_passes

    def run(self, max_results: int = 0) -> dict[str, Any]:
        if max_results < 0:
            raise CoordinateSearchError("max_results cannot be negative")
        self._enter_running_state()
        state = self._load_or_initialize_state()
        processed = 0
        while state["phase"] != "completed" and (max_results == 0 or processed < max_results):
            state, produced_result = self._step(state)
            if produced_result:
                processed += 1
        if state["phase"] == "completed":
            campaign = self.database.campaign(self.campaign_id)
            if campaign["status"] == "running":
                self.database.transition_campaign(self.campaign_id, "completed", "coordinate search completed")
        return self.report()

    def _enter_running_state(self) -> None:
        campaign = self.database.campaign(self.campaign_id)
        status = str(campaign["status"])
        if status in {"pending", "paused", "interrupted"}:
            self.database.transition_campaign(self.campaign_id, "running", "coordinate search start/resume")
        elif status in {"completed", "failed", "rejected"}:
            # A completed coordinate state is read-only and idempotent.
            if status != "completed":
                raise CoordinateSearchError(f"campaign is terminal: {status}")

    def _load_or_initialize_state(self) -> dict[str, Any]:
        stored = self.database.optimizer_state(self.campaign_id)["state"]
        if stored.get("algorithm") == self.ALGORITHM:
            if stored.get("registry_sha256") != self.registry.sha256:
                raise CoordinateSearchError("coordinate registry changed after initialization")
            self.max_passes = int(stored["max_passes"])
            return stored
        campaign = self.database.campaign(self.campaign_id)
        baseline = self.database.parameter_set_by_hash(self.campaign_id, campaign["baseline_parameter_hash"])
        if baseline is None:
            raise CoordinateSearchError("baseline parameter set is missing")
        try:
            normalized_baseline = normalize_parameter_document(baseline["document"], self.registry)
        except ValidationError as exc:
            raise CoordinateSearchError(f"baseline does not match coordinate registry: {exc}") from exc
        if normalized_baseline != baseline["document"]:
            raise CoordinateSearchError("baseline parameter document is not canonical for coordinate registry")
        rng = random.Random(int(campaign["master_seed"]))
        state: dict[str, Any] = {
            "algorithm": self.ALGORITHM,
            "version": 1,
            "phase": "baseline",
            "pass": 0,
            "parameter_index": 0,
            "direction_index": 0,
            "max_passes": self.max_passes,
            "registry_sha256": self.registry.sha256,
            "registry_name": self.registry.name,
            "anchor_parameters": baseline["document"],
            "anchor_hash": baseline["parameter_hash"],
            "anchor_result": None,
            "coordinate_results": {},
            "improved_in_pass": False,
            "evaluated_parameter_hashes": [baseline["parameter_hash"]],
            "result_count": 0,
            "last_result": None,
            "rng_state": _jsonable(rng.getstate()),
        }
        self.database.checkpoint(self.campaign_id, state, event_type="coordinate_initialized")
        return state

    def _step(self, state: dict[str, Any]) -> tuple[dict[str, Any], bool]:
        if state["phase"] == "baseline":
            return self._evaluate_baseline(state)
        if state["phase"] != "coordinate":
            if state["phase"] == "completed":
                return state, False
            raise CoordinateSearchError(f"unknown coordinate phase: {state['phase']}")
        if state["parameter_index"] >= len(self.specs):
            return self._select_coordinate(state), False
        direction_index = int(state["direction_index"])
        if direction_index >= 2:
            return self._select_coordinate(state), False
        spec = self.specs[int(state["parameter_index"])]
        direction = 1 if direction_index == 0 else -1
        candidate = self._candidate(state["anchor_parameters"], spec, direction)
        if candidate is None:
            updated = dict(state)
            updated["direction_index"] = direction_index + 1
            updated["coordinate_results"] = dict(state["coordinate_results"])
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_candidate_skipped")
            return updated, False
        candidate_hash = sha256_json(candidate)
        existing = self.database.trial_for_parameter_hash(self.campaign_id, candidate_hash)
        if existing is not None and existing["status"] == "completed":
            result = _normalize_result(json.loads(existing["result_json"] or "{}"))
            result = self._classify(result, state["anchor_result"])
            updated = self._apply_result(state, spec, direction, candidate, candidate_hash, result)
            updated["last_result"] = result
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_result_reused")
            return updated, False

        parameter_set_id = self.database.add_parameter_set(self.campaign_id, candidate, group_name="coordinate")
        seed = _stable_trial_seed(self.campaign_id, candidate_hash, int(self.database.campaign(self.campaign_id)["master_seed"]))
        trial_id = self.database.create_trial(self.campaign_id, parameter_set_id, self.ALGORITHM, seed)
        raw_result = self.evaluator(candidate, seed)
        result = _normalize_result(raw_result)
        result["parameter_hash"] = candidate_hash
        result["trial_id"] = trial_id
        result["parameter"] = candidate
        result = self._classify(result, state["anchor_result"])
        updated = self._apply_result(state, spec, direction, candidate, candidate_hash, result, trial_id)
        rng = random.Random()
        rng.setstate(_tuple_state(state["rng_state"]))
        result["rng_draw"] = rng.getrandbits(64)
        updated["last_result"] = result
        updated["rng_state"] = _jsonable(rng.getstate())
        updated["result_count"] = int(state["result_count"]) + 1
        self.database.record_coordinate_result_atomically(self.campaign_id, trial_id, result, updated)
        return updated, True

    def _evaluate_baseline(self, state: dict[str, Any]) -> tuple[dict[str, Any], bool]:
        baseline = state["anchor_parameters"]
        parameter_hash = state["anchor_hash"]
        existing = self.database.trial_for_parameter_hash(self.campaign_id, parameter_hash)
        if existing is not None and existing["status"] == "completed":
            result = json.loads(existing["result_json"] or "{}")
            result["classification"] = "baseline"
            updated = dict(state)
            updated.update(
                {
                    "phase": "coordinate",
                    "anchor_result": result,
                    "coordinate_results": {},
                    "last_result": result,
                    "result_count": int(state["result_count"]) + 1,
                }
            )
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_baseline_reused")
            return updated, False
        parameter_set = self.database.parameter_set_by_hash(self.campaign_id, parameter_hash)
        if parameter_set is None:
            raise CoordinateSearchError("baseline parameter set is missing")
        seed = _stable_trial_seed(self.campaign_id, parameter_hash, int(self.database.campaign(self.campaign_id)["master_seed"]))
        trial_id = self.database.create_trial(self.campaign_id, parameter_set["parameter_set_id"], self.ALGORITHM, seed)
        result = _normalize_result(self.evaluator(baseline, seed))
        result.update({"parameter_hash": parameter_hash, "trial_id": trial_id, "parameter": baseline, "classification": "baseline"})
        updated = dict(state)
        updated.update(
            {
                "phase": "coordinate",
                "anchor_result": result,
                "coordinate_results": {},
                "last_result": result,
                "result_count": int(state["result_count"]) + 1,
            }
        )
        rng = random.Random()
        rng.setstate(_tuple_state(state["rng_state"]))
        result["rng_draw"] = rng.getrandbits(64)
        updated["last_result"] = result
        updated["rng_state"] = _jsonable(rng.getstate())
        self.database.record_coordinate_result_atomically(self.campaign_id, trial_id, result, updated)
        return updated, True

    @staticmethod
    def _candidate(document: dict[str, Any], spec: ParameterSpec, direction: int) -> dict[str, Any] | None:
        values = _parameter_values(document)
        current = values[spec.name]
        value = current + direction * spec.step
        if value < spec.minimum or value > spec.maximum:
            return None
        return _with_value(document, spec.name, value)

    @staticmethod
    def _classify(result: dict[str, Any], anchor: dict[str, Any]) -> dict[str, Any]:
        reused_against_other_anchor = (
            result.get("reused")
            and "candidate_objective" not in result
            and result.get("reference_parameter_hash")
            and anchor.get("parameter_hash")
            and result.get("reference_parameter_hash") != anchor.get("parameter_hash")
        )
        if reused_against_other_anchor:
            delta = 0.0
            margin = max(float(result["uncertainty"]), float(anchor["uncertainty"]))
            classification = "uncertain"
        elif "candidate_objective" in result and "candidate_objective" in anchor:
            delta = float(result["candidate_objective"]) - float(anchor["candidate_objective"])
            margin = 0.0
            classification = "win" if delta > 0 else ("loss" if delta < 0 else "uncertain")
        else:
            delta = float(result["score"]) - float(anchor["score"])
            margin = max(float(result["uncertainty"]), float(anchor["uncertainty"]))
            decision = result.get("decision")
            if decision == "accept":
                classification = "win"
            elif decision in {"reject", "reject_early"}:
                classification = "loss"
            elif decision in {"continue", "uncertain", "interrupted"} or result.get("uncertain"):
                classification = "uncertain"
            else:
                classification = "uncertain" if abs(delta) <= margin else ("win" if delta > 0 else "loss")
        result["classification"] = classification
        result["score_delta"] = round(delta, 8)
        result["comparison_uncertainty"] = round(margin, 8)
        return result

    def _apply_result(
        self,
        state: dict[str, Any],
        spec: ParameterSpec,
        direction: int,
        candidate: dict[str, Any],
        candidate_hash: str,
        result: dict[str, Any],
        trial_id: str | None = None,
    ) -> dict[str, Any]:
        updated = dict(state)
        coordinate_results = dict(state["coordinate_results"])
        key = str(direction)
        stored = dict(result)
        stored["parameter_hash"] = candidate_hash
        stored["parameter"] = candidate
        if trial_id is not None:
            stored["trial_id"] = trial_id
        coordinate_results[key] = stored
        updated["coordinate_results"] = coordinate_results
        updated["direction_index"] = int(state["direction_index"]) + 1
        hashes = list(state["evaluated_parameter_hashes"])
        if candidate_hash not in hashes:
            hashes.append(candidate_hash)
        updated["evaluated_parameter_hashes"] = hashes
        return updated

    def _select_coordinate(self, state: dict[str, Any]) -> dict[str, Any]:
        choices = [state["coordinate_results"].get("1"), state["coordinate_results"].get("-1")]
        eligible = [item for item in choices if item is not None and item.get("classification") == "win"]
        updated = dict(state)
        if eligible:
            selected = max(eligible, key=lambda item: float(item["score"]))
            updated["anchor_parameters"] = selected["parameter"]
            updated["anchor_hash"] = selected["parameter_hash"]
            updated["anchor_result"] = selected
            updated["improved_in_pass"] = True
        next_index = int(state["parameter_index"]) + 1
        if next_index >= len(self.specs):
            if updated["improved_in_pass"] and int(state["pass"]) + 1 < self.max_passes:
                updated["pass"] = int(state["pass"]) + 1
                updated["parameter_index"] = 0
                updated["improved_in_pass"] = False
                updated["coordinate_results"] = {}
                updated["direction_index"] = 0
            else:
                updated["phase"] = "completed"
                updated["parameter_index"] = len(self.specs)
                updated["coordinate_results"] = {}
        else:
            updated["parameter_index"] = next_index
            updated["direction_index"] = 0
            updated["coordinate_results"] = {}
        self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_decision")
        return updated

    def report(self) -> dict[str, Any]:
        state = self.database.optimizer_state(self.campaign_id)
        campaign = self.database.status_snapshot(self.campaign_id)
        algorithm_state = state["state"]
        return {
            "campaign": campaign,
            "algorithm": self.ALGORITHM,
            "phase": algorithm_state.get("phase"),
            "pass": algorithm_state.get("pass"),
            "result_count": algorithm_state.get("result_count", 0),
            "evaluated_parameter_hashes": algorithm_state.get("evaluated_parameter_hashes", []),
            "best": {
                "parameter_hash": algorithm_state.get("anchor_hash"),
                "parameters": algorithm_state.get("anchor_parameters"),
                "result": algorithm_state.get("anchor_result"),
            },
            "last_result": algorithm_state.get("last_result"),
            "checkpoint": {
                "revision": state["revision"],
                "hash": state["checkpoint_hash"],
                "updated_at": state["updated_at"],
            },
        }


class MultiResolutionCoordinateSearch:
    """Deterministic coordinate descent with progressively smaller steps.

    A complete pass walks the selected parameters in registry order. For each
    parameter it evaluates ``+step`` and ``-step`` from the current anchor and
    accepts the best clear improvement. An improving pass restarts at the
    first parameter with the same step sizes. A resultless pass halves every
    step independently, down to its registered ``min_step``. Every evaluator
    result and the following search state are committed in one SQLite
    transaction, so a restart cannot create a second trial for a parameter
    hash or lose the next search position.
    """

    ALGORITHM = "coordinate-multires-v1"

    def __init__(
        self,
        database: Database,
        campaign_id: str,
        registry: Registry,
        evaluator: Evaluator,
        max_passes: int = 100,
        parameter_names: list[str] | tuple[str, ...] | None = None,
    ) -> None:
        if max_passes < 1:
            raise CoordinateSearchError("max_passes must be positive")
        self.database = database
        self.campaign_id = campaign_id
        self.registry = registry
        self.evaluator = evaluator
        all_specs = _parameter_specs(registry)
        if parameter_names is None:
            selected_names = tuple(spec.name for spec in all_specs)
        else:
            selected_names = tuple(parameter_names)
            if not selected_names or len(set(selected_names)) != len(selected_names):
                raise CoordinateSearchError("parameter_names must be non-empty and unique")
            known_names = {spec.name for spec in all_specs}
            unknown = [name for name in selected_names if name not in known_names]
            if unknown:
                raise CoordinateSearchError(f"unknown coordinate parameters: {','.join(unknown)}")
        by_name = {spec.name: spec for spec in all_specs}
        self.parameter_names = selected_names
        self.specs = tuple(by_name[name] for name in selected_names)
        self.max_passes = max_passes

    def run(self, max_results: int = 0) -> dict[str, Any]:
        if max_results < 0:
            raise CoordinateSearchError("max_results cannot be negative")
        self._enter_running_state()
        state = self._load_or_initialize_state()
        processed = 0
        while state["phase"] != "completed" and (max_results == 0 or processed < max_results):
            state, produced_result = self._step(state)
            if produced_result:
                processed += 1
        if state["phase"] == "completed":
            campaign = self.database.campaign(self.campaign_id)
            if campaign["status"] == "running":
                self.database.transition_campaign(
                    self.campaign_id, "completed", "multi-resolution coordinate search completed"
                )
        return self.report()

    def stop(self, reason: str) -> dict[str, Any]:
        """Persist a terminal search stop, for example an exhausted match budget."""
        if not reason or not isinstance(reason, str):
            raise CoordinateSearchError("search stop reason must be a non-empty string")
        self._enter_running_state()
        state = self._load_or_initialize_state()
        if state["phase"] != "completed":
            updated = dict(state)
            updated["phase"] = "completed"
            updated["stop_reason"] = reason
            updated["parameter_index"] = len(self.specs)
            updated["coordinate_results"] = {}
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_stopped")
        campaign = self.database.campaign(self.campaign_id)
        if campaign["status"] == "running":
            self.database.transition_campaign(self.campaign_id, "completed", reason)
        return self.report()

    def _enter_running_state(self) -> None:
        campaign = self.database.campaign(self.campaign_id)
        status = str(campaign["status"])
        if status in {"pending", "paused", "interrupted"}:
            self.database.transition_campaign(self.campaign_id, "running", "multi-resolution search start/resume")
        elif status in {"completed", "failed", "rejected"}:
            if status != "completed":
                raise CoordinateSearchError(f"campaign is terminal: {status}")

    def _load_or_initialize_state(self) -> dict[str, Any]:
        stored = self.database.optimizer_state(self.campaign_id)["state"]
        if stored.get("algorithm") == self.ALGORITHM:
            if stored.get("registry_sha256") != self.registry.sha256:
                raise CoordinateSearchError("multi-resolution registry changed after initialization")
            if tuple(stored.get("parameter_names", ())) != self.parameter_names:
                raise CoordinateSearchError("selected coordinate parameters changed after initialization")
            self.max_passes = int(stored["max_passes"])
            return stored
        if stored.get("algorithm") is not None:
            raise CoordinateSearchError(
                f"campaign already contains optimizer algorithm {stored.get('algorithm')}"
            )

        campaign = self.database.campaign(self.campaign_id)
        baseline = self.database.parameter_set_by_hash(self.campaign_id, campaign["baseline_parameter_hash"])
        if baseline is None:
            raise CoordinateSearchError("baseline parameter set is missing")
        try:
            normalized_baseline = normalize_parameter_document(baseline["document"], self.registry)
        except ValidationError as exc:
            raise CoordinateSearchError(f"baseline does not match coordinate registry: {exc}") from exc
        if normalized_baseline != baseline["document"]:
            raise CoordinateSearchError("baseline parameter document is not canonical for coordinate registry")

        rng = random.Random(int(campaign["master_seed"]))
        initial_steps = {spec.name: spec.step for spec in self.specs}
        min_steps = {spec.name: spec.min_step for spec in self.specs}
        state: dict[str, Any] = {
            "algorithm": self.ALGORITHM,
            "version": 1,
            "phase": "baseline",
            "pass": 0,
            "parameter_index": 0,
            "direction_index": 0,
            "max_passes": self.max_passes,
            "registry_sha256": self.registry.sha256,
            "registry_name": self.registry.name,
            "parameter_names": list(self.parameter_names),
            "initial_steps": initial_steps,
            "min_steps": min_steps,
            "step_by_parameter": dict(initial_steps),
            "step_history": [dict(initial_steps)],
            "anchor_parameters": baseline["document"],
            "anchor_hash": baseline["parameter_hash"],
            "anchor_result": None,
            "coordinate_base_parameters": baseline["document"],
            "coordinate_base_hash": baseline["parameter_hash"],
            "coordinate_base_result": None,
            "coordinate_results": {},
            "improved_in_pass": False,
            "evaluated_parameter_hashes": [baseline["parameter_hash"]],
            "result_count": 0,
            "last_result": None,
            "rng_state": _jsonable(rng.getstate()),
        }
        self.database.checkpoint(self.campaign_id, state, event_type="coordinate_multires_initialized")
        return state

    def _step(self, state: dict[str, Any]) -> tuple[dict[str, Any], bool]:
        if state["phase"] == "baseline":
            return self._evaluate_baseline(state)
        if state["phase"] != "coordinate":
            if state["phase"] == "completed":
                return state, False
            raise CoordinateSearchError(f"unknown multi-resolution phase: {state['phase']}")
        if int(state["parameter_index"]) >= len(self.specs):
            return self._finish_pass(state), False
        direction_index = int(state["direction_index"])
        if direction_index >= 2:
            return self._select_coordinate(state)

        spec = self.specs[int(state["parameter_index"])]
        step = int(state["step_by_parameter"][spec.name])
        direction = 1 if direction_index == 0 else -1
        base_parameters = state["coordinate_base_parameters"]
        candidate = self._candidate_at_step(base_parameters, spec, direction, step)
        if candidate is None:
            updated = dict(state)
            updated["direction_index"] = direction_index + 1
            updated["coordinate_results"] = dict(state["coordinate_results"])
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_candidate_skipped")
            return updated, False

        candidate_hash = sha256_json(candidate)
        result, trial_id, is_new = self._evaluate_candidate(candidate, candidate_hash, state)
        result = self._classify(result, state["coordinate_base_result"])
        result["direction"] = direction
        result["step"] = step
        updated = self._apply_coordinate_result(state, direction, candidate, candidate_hash, result)
        if is_new:
            rng = random.Random()
            rng.setstate(_tuple_state(state["rng_state"]))
            result["rng_draw"] = rng.getrandbits(64)
            updated["last_result"] = result
            updated["rng_state"] = _jsonable(rng.getstate())
            updated["result_count"] = int(state["result_count"]) + 1
            if trial_id is None:
                raise CoordinateSearchError("new coordinate result has no trial id")
            self.database.record_coordinate_result_atomically(self.campaign_id, trial_id, result, updated)
            return updated, True
        updated["last_result"] = result
        self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_result_reused")
        return updated, False

    def _evaluate_baseline(self, state: dict[str, Any]) -> tuple[dict[str, Any], bool]:
        baseline = state["anchor_parameters"]
        parameter_hash = state["anchor_hash"]
        existing = self.database.trial_for_parameter_hash(self.campaign_id, parameter_hash)
        if existing is not None and existing["status"] == "completed":
            result = _normalize_result(json.loads(existing["result_json"] or "{}"))
            result.update({"parameter_hash": parameter_hash, "trial_id": existing["trial_id"], "parameter": baseline})
            result["classification"] = "baseline"
            updated = dict(state)
            updated.update(
                {
                    "phase": "coordinate",
                    "coordinate_base_parameters": baseline,
                    "coordinate_base_hash": parameter_hash,
                    "coordinate_base_result": result,
                    "anchor_result": result,
                    "coordinate_results": {},
                    "last_result": result,
                    "result_count": max(1, int(state["result_count"])),
                }
            )
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_baseline_reused")
            return updated, False

        parameter_set = self.database.parameter_set_by_hash(self.campaign_id, parameter_hash)
        if parameter_set is None:
            raise CoordinateSearchError("baseline parameter set is missing")
        seed = _stable_trial_seed(
            self.campaign_id, parameter_hash, int(self.database.campaign(self.campaign_id)["master_seed"])
        )
        trial_id = self.database.create_trial(
            self.campaign_id, parameter_set["parameter_set_id"], self.ALGORITHM, seed
        )
        result = _normalize_result(self.evaluator(baseline, seed))
        result.update({"parameter_hash": parameter_hash, "trial_id": trial_id, "parameter": baseline, "classification": "baseline"})
        updated = dict(state)
        updated.update(
            {
                "phase": "coordinate",
                "coordinate_base_parameters": baseline,
                "coordinate_base_hash": parameter_hash,
                "coordinate_base_result": result,
                "anchor_result": result,
                "coordinate_results": {},
                "last_result": result,
                "result_count": int(state["result_count"]) + 1,
            }
        )
        rng = random.Random()
        rng.setstate(_tuple_state(state["rng_state"]))
        result["rng_draw"] = rng.getrandbits(64)
        updated["last_result"] = result
        updated["rng_state"] = _jsonable(rng.getstate())
        self.database.record_coordinate_result_atomically(self.campaign_id, trial_id, result, updated)
        return updated, True

    def _evaluate_candidate(
        self, candidate: dict[str, Any], candidate_hash: str, state: dict[str, Any]
    ) -> tuple[dict[str, Any], str | None, bool]:
        existing = self.database.trial_for_parameter_hash(self.campaign_id, candidate_hash)
        if existing is not None and existing["status"] == "completed":
            result = _normalize_result(json.loads(existing["result_json"] or "{}"))
            result.update(
                {
                    "parameter_hash": candidate_hash,
                    "trial_id": existing["trial_id"],
                    "parameter": candidate,
                    "reused": True,
                }
            )
            return result, None, False
        if existing is not None and existing["status"] in {"failed", "rejected"}:
            raise CoordinateSearchError(
                f"parameter hash {candidate_hash} has terminal trial {existing['status']}"
            )
        parameter_set_id = self.database.add_parameter_set(self.campaign_id, candidate, group_name="coordinate-multires")
        seed = _stable_trial_seed(
            self.campaign_id, candidate_hash, int(self.database.campaign(self.campaign_id)["master_seed"])
        )
        trial_id = self.database.create_trial(self.campaign_id, parameter_set_id, self.ALGORITHM, seed)
        result = _normalize_result(self.evaluator(candidate, seed))
        result.update(
            {
                "parameter_hash": candidate_hash,
                "trial_id": trial_id,
                "parameter": candidate,
                "reused": False,
            }
        )
        return result, trial_id, True

    @staticmethod
    def _candidate_at_step(
        document: dict[str, Any], spec: ParameterSpec, direction: int, step: int
    ) -> dict[str, Any] | None:
        values = _parameter_values(document)
        value = values[spec.name] + direction * step
        if value < spec.minimum or value > spec.maximum:
            return None
        return _with_value(document, spec.name, value)

    @staticmethod
    def _classify(result: dict[str, Any], anchor: dict[str, Any]) -> dict[str, Any]:
        reused_against_other_anchor = (
            result.get("reused")
            and "candidate_objective" not in result
            and result.get("reference_parameter_hash")
            and anchor.get("parameter_hash")
            and result.get("reference_parameter_hash") != anchor.get("parameter_hash")
        )
        if reused_against_other_anchor:
            delta = 0.0
            margin = max(float(result["uncertainty"]), float(anchor["uncertainty"]))
            classification = "uncertain"
        elif "candidate_objective" in result and "candidate_objective" in anchor:
            delta = float(result["candidate_objective"]) - float(anchor["candidate_objective"])
            margin = 0.0
            classification = "win" if delta > 0 else ("loss" if delta < 0 else "uncertain")
        else:
            delta = float(result["score"]) - float(anchor["score"])
            margin = max(float(result["uncertainty"]), float(anchor["uncertainty"]))
            decision = result.get("decision")
            if decision == "accept":
                classification = "win"
            elif decision in {"reject", "reject_early"}:
                classification = "loss"
            elif decision in {"continue", "uncertain", "interrupted"} or result.get("uncertain"):
                classification = "uncertain"
            else:
                classification = "uncertain" if abs(delta) <= margin else ("win" if delta > 0 else "loss")
        result["classification"] = classification
        result["score_delta"] = round(delta, 8)
        result["comparison_uncertainty"] = round(margin, 8)
        return result

    @staticmethod
    def _apply_coordinate_result(
        state: dict[str, Any],
        direction: int,
        candidate: dict[str, Any],
        candidate_hash: str,
        result: dict[str, Any],
    ) -> dict[str, Any]:
        updated = dict(state)
        coordinate_results = dict(state["coordinate_results"])
        stored = dict(result)
        stored.update({"parameter_hash": candidate_hash, "parameter": candidate})
        coordinate_results[str(direction)] = stored
        updated["coordinate_results"] = coordinate_results
        updated["direction_index"] = int(state["direction_index"]) + 1
        hashes = list(state["evaluated_parameter_hashes"])
        if candidate_hash not in hashes:
            hashes.append(candidate_hash)
        updated["evaluated_parameter_hashes"] = hashes
        return updated

    def _select_coordinate(self, state: dict[str, Any]) -> tuple[dict[str, Any], bool]:
        choices = [state["coordinate_results"].get("1"), state["coordinate_results"].get("-1")]
        eligible = [item for item in choices if item is not None and item.get("classification") == "win"]
        updated = dict(state)
        if eligible:
            selected = max(
                eligible,
                key=lambda item: (float(item["score"]), 1 if int(item.get("direction", 0)) == 1 else 0),
            )
            updated["anchor_parameters"] = selected["parameter"]
            updated["anchor_hash"] = selected["parameter_hash"]
            updated["anchor_result"] = selected
            updated["coordinate_base_parameters"] = selected["parameter"]
            updated["coordinate_base_hash"] = selected["parameter_hash"]
            updated["coordinate_base_result"] = selected
            updated["improved_in_pass"] = True
        updated["parameter_index"] = int(state["parameter_index"]) + 1
        updated["direction_index"] = 0
        updated["coordinate_results"] = {}
        self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_decision")
        return updated, False

    def _finish_pass(self, state: dict[str, Any]) -> dict[str, Any]:
        updated = dict(state)
        completed_pass = int(state["pass"]) + 1
        updated["pass"] = completed_pass
        if completed_pass >= int(state["max_passes"]):
            updated["phase"] = "completed"
            updated["parameter_index"] = len(self.specs)
            updated["coordinate_results"] = {}
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_completed")
            return updated

        if state["improved_in_pass"]:
            updated["parameter_index"] = 0
            updated["direction_index"] = 0
            updated["coordinate_results"] = {}
            updated["coordinate_base_parameters"] = updated["anchor_parameters"]
            updated["coordinate_base_hash"] = updated["anchor_hash"]
            updated["coordinate_base_result"] = updated["anchor_result"]
            updated["improved_in_pass"] = False
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_restart_pass")
            return updated

        old_steps = dict(state["step_by_parameter"])
        new_steps = {
            name: max(int(state["min_steps"][name]), int(step) // 2)
            for name, step in old_steps.items()
        }
        if new_steps == old_steps:
            updated["phase"] = "completed"
            updated["parameter_index"] = len(self.specs)
            updated["coordinate_results"] = {}
            self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_completed")
            return updated

        updated["step_by_parameter"] = new_steps
        updated["step_history"] = list(state["step_history"]) + [dict(new_steps)]
        updated["parameter_index"] = 0
        updated["direction_index"] = 0
        updated["coordinate_results"] = {}
        updated["coordinate_base_parameters"] = updated["anchor_parameters"]
        updated["coordinate_base_hash"] = updated["anchor_hash"]
        updated["coordinate_base_result"] = updated["anchor_result"]
        updated["improved_in_pass"] = False
        self.database.checkpoint(self.campaign_id, updated, event_type="coordinate_multires_halved_steps")
        return updated

    def report(self) -> dict[str, Any]:
        state = self.database.optimizer_state(self.campaign_id)
        campaign = self.database.status_snapshot(self.campaign_id)
        algorithm_state = state["state"]
        return {
            "campaign": campaign,
            "algorithm": self.ALGORITHM,
            "phase": algorithm_state.get("phase"),
            "pass": algorithm_state.get("pass"),
            "parameter_names": algorithm_state.get("parameter_names", []),
            "step_by_parameter": algorithm_state.get("step_by_parameter", {}),
            "step_history": algorithm_state.get("step_history", []),
            "stop_reason": algorithm_state.get("stop_reason"),
            "result_count": algorithm_state.get("result_count", 0),
            "evaluated_parameter_hashes": algorithm_state.get("evaluated_parameter_hashes", []),
            "best": {
                "parameter_hash": algorithm_state.get("anchor_hash"),
                "parameters": algorithm_state.get("anchor_parameters"),
                "result": algorithm_state.get("anchor_result"),
            },
            "last_result": algorithm_state.get("last_result"),
            "checkpoint": {
                "revision": state["revision"],
                "hash": state["checkpoint_hash"],
                "updated_at": state["updated_at"],
            },
        }


def synthetic_evaluator(optimum: dict[str, int], uncertain_values: set[tuple[str, int]] | None = None) -> Evaluator:
    """Return a deterministic objective with an explicit known optimum."""
    uncertain_values = uncertain_values or set()

    def evaluate(parameters: dict[str, Any], seed: int) -> dict[str, Any]:
        values = _parameter_values(parameters)
        distance = sum((value - int(optimum[name])) ** 2 for name, value in values.items())
        score = max(10.0, 100.0 - 10.0 * distance)
        draws = 10
        wins = max(0, min(90, round(score - 5)))
        losses = 90 - wins
        uncertain = any((name, value) in uncertain_values for name, value in values.items())
        return {
            "wins": wins,
            "draws": draws,
            "losses": losses,
            "score": float(wins + draws / 2),
            "uncertainty": 1.0,
            "uncertain": uncertain,
            "objective_seed": seed,
        }

    return evaluate


def run_synthetic_coordinate_search(
    data_dir: Path,
    campaign_id: str,
    registry: Registry,
    optimum: dict[str, int],
    max_results: int = 0,
    max_passes: int = 100,
    uncertain_values: set[tuple[str, int]] | None = None,
) -> dict[str, Any]:
    database = load_database(data_dir, campaign_id)
    names = {item["name"] for item in registry.parameters}
    if set(optimum) != names or any(not isinstance(value, int) or isinstance(value, bool) for value in optimum.values()):
        raise CoordinateSearchError("synthetic optimum must contain exactly every registry parameter as an integer")
    evaluator = synthetic_evaluator(optimum, uncertain_values)
    with campaign_lock(data_dir, campaign_id):
        return CoordinateSearch(database, campaign_id, registry, evaluator, max_passes=max_passes).run(max_results)


def run_synthetic_multiresolution_search(
    data_dir: Path,
    campaign_id: str,
    registry: Registry,
    optimum: dict[str, int],
    max_results: int = 0,
    max_passes: int = 100,
    uncertain_values: set[tuple[str, int]] | None = None,
    parameter_names: list[str] | tuple[str, ...] | None = None,
) -> dict[str, Any]:
    """Run coordinate-multires-v1 against the deterministic synthetic objective."""
    database = load_database(data_dir, campaign_id)
    names = {item["name"] for item in registry.parameters}
    if set(optimum) != names or any(
        not isinstance(value, int) or isinstance(value, bool) for value in optimum.values()
    ):
        raise CoordinateSearchError("synthetic optimum must contain exactly every registry parameter as an integer")
    evaluator = synthetic_evaluator(optimum, uncertain_values)
    with campaign_lock(data_dir, campaign_id):
        return MultiResolutionCoordinateSearch(
            database,
            campaign_id,
            registry,
            evaluator,
            max_passes=max_passes,
            parameter_names=parameter_names,
        ).run(max_results)
