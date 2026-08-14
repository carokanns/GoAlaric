"""Named, hashed match profiles used by optimizer search and confirmation."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from .canonical import sha256_json


class ProfileError(ValueError):
    """A campaign contains an invalid or unresolved match profile."""


@dataclass(frozen=True)
class MatchProfile:
    """The resolved immutable identity passed to a match runner."""

    name: str
    tc: str
    hash: str
    source: str

    @classmethod
    def create(cls, name: str, tc: str, source: str) -> "MatchProfile":
        if not isinstance(name, str) or not name.strip():
            raise ProfileError("profile name must be a non-empty string")
        if not isinstance(tc, str) or not tc.strip():
            raise ProfileError(f"profile {name!r} must define a non-empty tc")
        clean_name = name.strip()
        clean_tc = tc.strip()
        return cls(
            name=clean_name,
            tc=clean_tc,
            hash=sha256_json({"name": clean_name, "tc": clean_tc}),
            source=source,
        )

    def as_dict(self) -> dict[str, str]:
        return {
            "name": self.name,
            "tc": self.tc,
            "hash": self.hash,
            "source": self.source,
        }


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
    tc = raw.get("tc")
    if tc is None:
        raise ProfileError(f"goals.real.profiles.{name}.tc is required")
    return MatchProfile.create(name, tc, f"real.profiles.{name}")
