from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .canonical import canonical_json, sha256_bytes, sha256_json
from .registry import (
    Registry,
    ValidationError,
    default_parameter_document,
    load_parameter_file,
    load_registry,
    normalize_parameter_document,
    parameter_hash,
)


CAMPAIGN_SCHEMA_VERSION = 1
CAMPAIGN_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")


@dataclass(frozen=True)
class CampaignDefinition:
    campaign_id: str
    name: str
    mode: str
    config: dict[str, Any]
    config_hash: str
    registry: Registry
    baseline_parameters: dict[str, Any]
    baseline_parameter_hash: str
    baseline_engine_id: str
    master_seed: int
    partitions: dict[str, Any]


def _resolve_input(path_value: str, base_dir: Path) -> Path:
    path = Path(path_value)
    if path.is_absolute():
        return path
    candidates = [base_dir / path, Path.cwd() / path]
    project_optimizer = Path(__file__).resolve().parents[2]
    candidates.append(project_optimizer / path)
    for candidate in candidates:
        if candidate.exists():
            return candidate.resolve()
    return candidates[0].resolve()


def load_campaign_definition(path: Path) -> CampaignDefinition:
    path = path.resolve()
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ValidationError(f"campaign file does not exist: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"campaign file is not valid JSON: {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise ValidationError("campaign must be a JSON object")
    if raw.get("schema_version") != CAMPAIGN_SCHEMA_VERSION:
        raise ValidationError("campaign schema_version is unsupported")
    campaign_id = raw.get("campaign_id")
    name = raw.get("name")
    mode = raw.get("mode", "fake")
    if not isinstance(campaign_id, str) or not CAMPAIGN_ID_PATTERN.fullmatch(campaign_id):
        raise ValidationError("campaign_id contains unsafe or invalid characters")
    if not isinstance(name, str) or not name.strip():
        raise ValidationError("campaign name must be a non-empty string")
    if mode not in {"fake", "real"}:
        raise ValidationError("campaign mode must be fake or real")
    registry_value = raw.get("registry")
    if not isinstance(registry_value, str) or not registry_value:
        raise ValidationError("campaign registry must be a path")
    registry = load_registry(_resolve_input(registry_value, path.parent))

    baseline = raw.get("baseline", {})
    if not isinstance(baseline, dict):
        raise ValidationError("campaign baseline must be an object")
    engine_id = baseline.get("engine_id", baseline.get("engine", "fake-engine" if mode == "fake" else ""))
    if not isinstance(engine_id, str) or not engine_id.strip():
        raise ValidationError("baseline.engine_id must be a non-empty string")

    parameter_value = baseline.get("parameters", raw.get("baseline_parameters"))
    parameter_file_value = baseline.get("parameter_file", raw.get("baseline_parameter_file"))
    if parameter_value is not None and parameter_file_value is not None:
        raise ValidationError("baseline parameters and baseline parameter_file are mutually exclusive")
    if parameter_file_value is not None:
        if not isinstance(parameter_file_value, str) or not parameter_file_value:
            raise ValidationError("baseline.parameter_file must be a path")
        baseline_parameters, baseline_parameter_hash = load_parameter_file(
            _resolve_input(parameter_file_value, path.parent), registry
        )
    elif parameter_value is not None:
        baseline_parameters = normalize_parameter_document(parameter_value, registry)
        baseline_parameter_hash = parameter_hash(baseline_parameters)
    else:
        baseline_parameters = default_parameter_document(registry)
        baseline_parameter_hash = parameter_hash(baseline_parameters)

    master_seed = raw.get("master_seed")
    if not isinstance(master_seed, int) or isinstance(master_seed, bool):
        raise ValidationError("master_seed must be an integer")
    partitions = raw.get("partitions", {"default": {"name": "default"}})
    if not isinstance(partitions, dict) or not partitions:
        raise ValidationError("partitions must be a non-empty object")
    for partition_name, partition in partitions.items():
        if not isinstance(partition_name, str) or not partition_name.strip():
            raise ValidationError("partition names must be non-empty strings")
        if not isinstance(partition, (dict, str, list)):
            raise ValidationError(f"partition {partition_name} must be an object, list or string")
    goals = raw.get("goals", {})
    if not isinstance(goals, dict):
        raise ValidationError("goals must be an object")

    normalized_config = {
        "schema_version": CAMPAIGN_SCHEMA_VERSION,
        "campaign_id": campaign_id,
        "name": name.strip(),
        "mode": mode,
        "registry": {
            "name": registry.name,
            "schema_version": registry.schema_version,
            "sha256": registry.sha256,
        },
        "baseline": {
            "engine_id": engine_id,
            "parameter_hash": baseline_parameter_hash,
        },
        "master_seed": master_seed,
        "partitions": partitions,
        "goals": goals,
    }
    return CampaignDefinition(
        campaign_id=campaign_id,
        name=name.strip(),
        mode=mode,
        config=normalized_config,
        config_hash=sha256_json(normalized_config),
        registry=registry,
        baseline_parameters=baseline_parameters,
        baseline_parameter_hash=baseline_parameter_hash,
        baseline_engine_id=engine_id,
        master_seed=master_seed,
        partitions=partitions,
    )


def baseline_parameter_bytes(definition: CampaignDefinition) -> bytes:
    return (canonical_json(definition.baseline_parameters) + "\n").encode("utf-8")


def baseline_parameter_sha256(definition: CampaignDefinition) -> str:
    return sha256_bytes(baseline_parameter_bytes(definition))
