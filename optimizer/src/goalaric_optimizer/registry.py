from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .canonical import canonical_json, sha256_bytes


class ValidationError(ValueError):
    pass


@dataclass(frozen=True)
class Registry:
    path: Path
    document: dict[str, Any]
    schema_version: int
    name: str
    parameters: tuple[dict[str, Any], ...]
    sha256: str


def load_registry(path: Path) -> Registry:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ValidationError(f"registry does not exist: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"registry is not valid JSON: {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise ValidationError("registry must be a JSON object")
    schema_version = raw.get("schema_version")
    name = raw.get("registry")
    parameters = raw.get("parameters")
    if not isinstance(schema_version, int) or isinstance(schema_version, bool) or schema_version < 1:
        raise ValidationError("registry.schema_version must be a positive integer")
    if not isinstance(name, str) or not name:
        raise ValidationError("registry.registry must be a non-empty string")
    if not isinstance(parameters, list) or not parameters:
        raise ValidationError("registry.parameters must be a non-empty list")
    seen: set[str] = set()
    normalized: list[dict[str, Any]] = []
    for item in parameters:
        if not isinstance(item, dict):
            raise ValidationError("every registry parameter must be an object")
        parameter_name = item.get("name")
        if not isinstance(parameter_name, str) or not parameter_name:
            raise ValidationError("every registry parameter needs a name")
        if parameter_name in seen:
            raise ValidationError(f"duplicate registry parameter: {parameter_name}")
        value = item.get("value")
        if not isinstance(value, int) or isinstance(value, bool):
            raise ValidationError(f"registry default for {parameter_name} must be an integer")
        seen.add(parameter_name)
        normalized.append(dict(item))
    document = dict(raw)
    document["parameters"] = normalized
    digest = sha256_bytes(canonical_json(document).encode("utf-8"))
    return Registry(path, document, schema_version, name, tuple(normalized), digest)


def default_parameter_document(registry: Registry) -> dict[str, Any]:
    return {
        "schema_version": registry.schema_version,
        "registry": registry.name,
        "parameters": [
            {"name": item["name"], "value": item["value"]} for item in registry.parameters
        ],
    }


def normalize_parameter_document(document: Any, registry: Registry) -> dict[str, Any]:
    if not isinstance(document, dict):
        raise ValidationError("parameter file must be a JSON object")
    if document.get("schema_version") != registry.schema_version:
        raise ValidationError("parameter file schema_version does not match the registry")
    if document.get("registry") != registry.name:
        raise ValidationError("parameter file registry does not match the registry")
    values = document.get("parameters")
    if not isinstance(values, list):
        raise ValidationError("parameter file parameters must be a list")
    by_name: dict[str, int] = {}
    for item in values:
        if not isinstance(item, dict) or set(item) != {"name", "value"}:
            raise ValidationError("parameter entries must contain only name and value")
        name = item.get("name")
        value = item.get("value")
        if not isinstance(name, str) or not name:
            raise ValidationError("parameter name must be a non-empty string")
        if name in by_name:
            raise ValidationError(f"duplicate parameter: {name}")
        if not isinstance(value, int) or isinstance(value, bool):
            raise ValidationError(f"parameter value for {name} must be an integer")
        by_name[name] = value
    expected = [item["name"] for item in registry.parameters]
    missing = [name for name in expected if name not in by_name]
    unknown = [name for name in by_name if name not in expected]
    if missing or unknown:
        detail = []
        if missing:
            detail.append(f"missing={','.join(missing)}")
        if unknown:
            detail.append(f"unknown={','.join(unknown)}")
        raise ValidationError("parameter file does not match registry: " + " ".join(detail))
    return {
        "schema_version": registry.schema_version,
        "registry": registry.name,
        "parameters": [{"name": name, "value": by_name[name]} for name in expected],
    }


def parameter_hash(document: dict[str, Any]) -> str:
    return sha256_bytes(canonical_json(document).encode("utf-8"))


def load_parameter_file(path: Path, registry: Registry) -> tuple[dict[str, Any], str]:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ValidationError(f"parameter file does not exist: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"parameter file is not valid JSON: {path}: {exc}") from exc
    normalized = normalize_parameter_document(raw, registry)
    return normalized, parameter_hash(normalized)
