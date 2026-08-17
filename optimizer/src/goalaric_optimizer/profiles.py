"""Named, hashed match profiles used by optimizer search and confirmation."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Literal

from .canonical import sha256_json


class ProfileError(ValueError):
    """A campaign contains an invalid or unresolved match profile."""


@dataclass(frozen=True)
class MatchProfile:
    """The resolved immutable identity passed to a match runner."""

    name: str
    mode: Literal["time", "nodes"]
    tc: str | None
    nodes: int | None
    hash: str
    source: str

    @classmethod
    def create(
        cls,
        name: str,
        tc: str | None = None,
        source: str = "",
        *,
        nodes: int | None = None,
    ) -> "MatchProfile":
        if not isinstance(name, str) or not name.strip():
            raise ProfileError("profile name must be a non-empty string")
        clean_name = name.strip()
        if (tc is None) == (nodes is None):
            raise ProfileError(f"profile {name!r} must define exactly one of tc or nodes")
        if tc is not None:
            if not isinstance(tc, str) or not tc.strip():
                raise ProfileError(f"profile {name!r} must define a non-empty tc")
            clean_tc = tc.strip()
            return cls(
                name=clean_name,
                mode="time",
                tc=clean_tc,
                nodes=None,
                hash=sha256_json({"name": clean_name, "tc": clean_tc}),
                source=source,
            )
        if not isinstance(nodes, int) or isinstance(nodes, bool) or nodes <= 0:
            raise ProfileError(f"profile {name!r}.nodes must be a positive integer")
        return cls(
            name=clean_name,
            mode="nodes",
            tc=None,
            nodes=nodes,
            hash=sha256_json({"name": clean_name, "mode": "nodes", "nodes": nodes}),
            source=source,
        )

    def as_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "name": self.name,
            "mode": self.mode,
            "hash": self.hash,
            "source": self.source,
        }
        if self.mode == "nodes":
            result["nodes"] = self.nodes
        else:
            result["tc"] = self.tc
        return result


def profile_identity(value: dict[str, Any] | None) -> dict[str, Any] | None:
    """Return the comparable profile identity, accepting pre-v1.2 time rows."""
    if value is None:
        return None
    if not isinstance(value, dict):
        return None
    name = value.get("name")
    profile_hash = value.get("hash")
    if not isinstance(name, str) or not isinstance(profile_hash, str):
        return None
    mode = value.get("mode")
    if mode is None:
        mode = "nodes" if value.get("nodes") is not None else "time"
    if mode == "time":
        tc = value.get("tc")
        if not isinstance(tc, str):
            return None
        return {"name": name, "mode": mode, "tc": tc, "hash": profile_hash}
    if mode == "nodes":
        nodes = value.get("nodes")
        if not isinstance(nodes, int) or isinstance(nodes, bool) or nodes <= 0:
            return None
        return {"name": name, "mode": mode, "nodes": nodes, "hash": profile_hash}
    return None


def profiles_equivalent(left: dict[str, Any] | None, right: dict[str, Any] | None) -> bool:
    return profile_identity(left) == profile_identity(right)


def resolve_profile(
    real_goals: dict[str, Any], requested_name: Any = None, role: str = "search"
) -> MatchProfile:
    """Resolve a named profile, retaining real.tc as the legacy default."""
    if not isinstance(real_goals, dict):
        raise ProfileError("goals.real must be an object")
    base_tc = real_goals.get("tc", "10+0.1")
    profiles = real_goals.get("profiles", {})
    if profiles is None:
        profiles = {}
    if not isinstance(profiles, dict):
        raise ProfileError("goals.real.profiles must be an object")

    if requested_name is None:
        if not isinstance(base_tc, str) or not base_tc.strip():
            raise ProfileError("goals.real.tc must be a non-empty string")
        return MatchProfile.create("default", base_tc, "real.tc")
    if not isinstance(requested_name, str) or not requested_name.strip():
        raise ProfileError(f"{role} profile must be a non-empty name")
    name = requested_name.strip()
    raw = profiles.get(name)
    if not isinstance(raw, dict):
        raise ProfileError(f"{role} profile {name!r} is not defined in goals.real.profiles")
    has_tc = "tc" in raw
    has_nodes = "nodes" in raw
    if has_tc == has_nodes:
        raise ProfileError(
            f"goals.real.profiles.{name} must define exactly one of tc or nodes"
        )
    if has_tc:
        return MatchProfile.create(name, raw.get("tc"), f"real.profiles.{name}")
    return MatchProfile.create(name, None, f"real.profiles.{name}", nodes=raw.get("nodes"))
