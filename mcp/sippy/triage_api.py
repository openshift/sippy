"""Component Readiness triage toolset for the Sippy MCP server.

Triage CRUD over Sippy's existing triage REST API, built on the shared client in sippy_client.py.
"""

from __future__ import annotations

import urllib.parse
from collections.abc import Mapping
from datetime import datetime, timezone
from typing import Any

from sippy_client import (
    HTTPResponse,
    SippyClient,
    ToolFailure,
    decode_json,
    invalid,
    tool_result,
    unknown_outcome_hint,
    validate_acting_for,
    validate_id,
)

TRIAGES_PATH = "/api/component_readiness/triages"
TRIAGE_TYPES = ("ci-infra", "product-infra", "product", "test")
TRIAGE_STATUSES = ("open", "resolved")

DEFAULT_LIST_LIMIT = 50
MAX_LIST_LIMIT = 200

# Go's zero time; Sippy decodes {"Valid": false} with this value as an unresolved triage.
_UNRESOLVED = {"Time": "0001-01-01T00:00:00Z", "Valid": False}
# Presentation-only fields a GET returns that must not be echoed into a PUT replacement body.
_READ_ONLY_FIELDS = ("links", "bug")
# Scalar fields update_triage can replace directly from tool arguments.
_EDITABLE_FIELDS = ("url", "description", "type")
_SUBJECT = "triage"

def _null_time(value: Any) -> str | None:
    """Convert Go sql.NullTime JSON ({"Time": ..., "Valid": ...}) to an RFC 3339 string or None."""
    if isinstance(value, dict):
        return value.get("Time") if value.get("Valid") else None
    return value if isinstance(value, str) else None


def _parse_time(value: str) -> datetime:
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timestamp must include a timezone")
    return parsed


def _regression_ids(raw: Mapping[str, Any]) -> list[int]:
    return [r["id"] for r in raw.get("regressions") or [] if isinstance(r, dict) and isinstance(r.get("id"), int)]


def summarize_regression(raw: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "id": raw.get("id"),
        "test_id": raw.get("test_id"),
        "test_name": raw.get("test_name"),
        "release": raw.get("release"),
        "base_release": raw.get("base_release"),
        "component": raw.get("component"),
        "capability": raw.get("capability"),
        "variants": raw.get("variants") or [],
        "opened": raw.get("opened"),
        "closed": _null_time(raw.get("closed")),
    }


def summarize_triage(raw: Mapping[str, Any], *, include_regressions: bool) -> dict[str, Any]:
    """Return a stable, compact triage representation for tool output."""
    bug = raw.get("bug")
    if not isinstance(bug, dict):
        bug = None
    summary: dict[str, Any] = {
        "id": raw.get("id"),
        "url": raw.get("url"),
        "description": raw.get("description") or "",
        "type": raw.get("type"),
        "resolved": _null_time(raw.get("resolved")),
        "resolution_reason": raw.get("resolution_reason") or None,
        "bug_id": raw.get("bug_id"),
        "bug": {k: bug.get(k) for k in ("key", "status", "summary", "url")} if bug else None,
        "regression_ids": sorted(_regression_ids(raw)),
        "created_at": raw.get("created_at"),
        "updated_at": raw.get("updated_at"),
        "links": raw.get("links") or {},
    }
    if include_regressions:
        summary["regressions"] = [summarize_regression(r) for r in raw.get("regressions") or [] if isinstance(r, dict)]
    return summary


# --- input validation -------------------------------------------------------------------------

# --- input validation -------------------------------------------------------------------------


def validate_id_list(values: Any, name: str) -> list[int]:
    if not isinstance(values, list):
        raise invalid(f"{name} must be a list of positive integers")
    if not values:
        raise invalid(f"{name} must not be empty; omit it to leave regression associations unchanged")
    ids = [validate_id(v, f"each entry in {name}") for v in values]
    if len(set(ids)) != len(ids):
        raise invalid(f"{name} must not contain duplicates")
    return ids


def validate_url(value: Any) -> str:
    if not isinstance(value, str) or not value.strip():
        raise invalid("url is required and must not be blank")
    url = value.strip()
    message = "url must be an absolute http(s) URL, typically a Jira issue"
    try:
        parts = urllib.parse.urlsplit(url)
        hostname = parts.hostname
        parts.port  # noqa: B018 - raises ValueError for a non-numeric or out-of-range port
    except ValueError:
        raise invalid(message) from None
    if parts.scheme not in ("http", "https") or not hostname:
        raise invalid(message)
    return url


def validate_type(value: Any) -> str:
    if value not in TRIAGE_TYPES:
        raise invalid(f"type must be one of: {', '.join(TRIAGE_TYPES)}")
    return value


def validate_description(value: Any) -> str:
    if not isinstance(value, str):
        raise invalid("description must be a string")
    return value


def validate_resolved(value: Any) -> str:
    """Normalize an RFC 3339 resolution timestamp to UTC."""
    if not isinstance(value, str) or not value.strip():
        raise invalid("resolved must be an RFC 3339 timestamp with a timezone; use clear_resolved to unresolve")
    try:
        parsed = _parse_time(value.strip())
    except ValueError:
        raise invalid("resolved must be an RFC 3339 timestamp with a timezone, e.g. 2026-01-02T15:04:05Z") from None
    return parsed.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


# --- domain operations ------------------------------------------------------------------------


# --- domain operations ------------------------------------------------------------------------


class TriageService:
    """Triage CRUD over Sippy's existing REST API.

    Create and update return the record Sippy persisted, which the POST and PUT responses already
    contain. Mutations are never retried: when the outcome is unknown the caller is told to read state
    first.
    """

    def __init__(self, client: SippyClient) -> None:
        self.client = client

    def _send(self, method: str, path: str, acting_for: str | None, *, mutation: bool, triage_id: int | None = None, **kwargs: Any) -> HTTPResponse:
        return self.client.send(
            method,
            path,
            acting_for,
            mutation=mutation,
            subject=_SUBJECT,
            not_found=f"triage {triage_id} not found" if triage_id else None,
            details={"triage_id": triage_id},
            **kwargs,
        )

    def _fetch(self, triage_id: int, acting_for: str | None = None) -> dict[str, Any]:
        response = self._send("GET", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=False, triage_id=triage_id)
        raw = decode_json(response)
        if not isinstance(raw, dict) or raw.get("id") != triage_id:
            raise ToolFailure("upstream_error", "Sippy returned an unexpected triage record", triage_id=triage_id)
        return raw

    @tool_result
    def list_triages(
        self,
        view: str | None = None,
        include_archived: bool = False,
        type: str | None = None,
        status: str | None = None,
        url_contains: str | None = None,
        regression_id: int | None = None,
        limit: int = DEFAULT_LIST_LIMIT,
    ) -> dict[str, Any]:
        query: dict[str, str] = {}
        if view is not None:
            if not isinstance(view, str) or not view.strip():
                raise invalid("view must be a non-blank view name, e.g. 4.22-main")
            query["view"] = view.strip()
        if not isinstance(include_archived, bool):
            raise invalid("include_archived must be a boolean")
        if include_archived:
            query["include_archived"] = "true"
        if type is not None:
            validate_type(type)
        if status is not None and status not in TRIAGE_STATUSES:
            raise invalid(f"status must be one of: {', '.join(TRIAGE_STATUSES)}")
        if url_contains is not None and (not isinstance(url_contains, str) or not url_contains.strip()):
            raise invalid("url_contains must be a non-blank string")
        if regression_id is not None:
            validate_id(regression_id, "regression_id")
        if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= MAX_LIST_LIMIT:
            raise invalid(f"limit must be an integer between 1 and {MAX_LIST_LIMIT}")

        # Sippy's list endpoint has no paging or server-side filters beyond view and include_archived,
        # so the full listing is fetched (bounded by MAX_RESPONSE_BYTES) and filtered here.
        raw = decode_json(self._send("GET", TRIAGES_PATH, None, mutation=False, query=query or None))
        if raw is None:
            raw = []
        if not isinstance(raw, list):
            raise ToolFailure("upstream_error", "Sippy returned an unexpected triage listing")

        needle = url_contains.strip().lower() if url_contains else None
        matches = []
        for record in raw:
            if not isinstance(record, dict):
                continue
            if type is not None and record.get("type") != type:
                continue
            resolved = _null_time(record.get("resolved")) is not None
            if status == "open" and resolved or status == "resolved" and not resolved:
                continue
            if needle and needle not in str(record.get("url", "")).lower():
                continue
            if regression_id is not None and regression_id not in _regression_ids(record):
                continue
            matches.append(record)
        matches.sort(key=lambda r: r.get("id") or 0, reverse=True)
        returned = [summarize_triage(r, include_regressions=False) for r in matches[:limit]]
        return {
            "triages": returned,
            "count": len(returned),
            "total_matches": len(matches),
            "truncated": len(matches) > limit,
        }

    @tool_result
    def get_triage(self, triage_id: int) -> dict[str, Any]:
        triage_id = validate_id(triage_id, "triage_id")
        return {"triage": summarize_triage(self._fetch(triage_id), include_regressions=True)}

    @tool_result
    def create_triage(
        self,
        acting_for: str,
        url: str,
        type: str,
        description: str | None = None,
        regression_ids: list[int] | None = None,
    ) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        body: dict[str, Any] = {"url": validate_url(url), "type": validate_type(type)}
        if description is not None:
            body["description"] = validate_description(description)
        ids = validate_id_list(regression_ids, "regression_ids") if regression_ids is not None else []
        body["regressions"] = [{"id": i} for i in ids]

        response = self._send("POST", TRIAGES_PATH, acting_for, mutation=True, body=body)
        try:
            created = decode_json(response)
        except ToolFailure:
            created = None
        triage_id = created.get("id") if isinstance(created, dict) else None
        if not isinstance(triage_id, int) or triage_id <= 0:
            raise ToolFailure("outcome_unknown", f"Sippy accepted the request but returned no triage id. {unknown_outcome_hint(_SUBJECT)}")
        return {"triage": summarize_triage(created, include_regressions=True)}

    @tool_result
    def update_triage(
        self,
        acting_for: str,
        triage_id: int,
        url: str | None = None,
        description: str | None = None,
        type: str | None = None,
        resolved: str | None = None,
        clear_resolved: bool = False,
        add_regression_ids: list[int] | None = None,
        remove_regression_ids: list[int] | None = None,
    ) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        triage_id = validate_id(triage_id, "triage_id")
        if not isinstance(clear_resolved, bool):
            raise invalid("clear_resolved must be a boolean")
        changes: dict[str, Any] = {}
        if url is not None:
            changes["url"] = validate_url(url)
        if description is not None:
            changes["description"] = validate_description(description)
        if type is not None:
            changes["type"] = validate_type(type)
        if resolved is not None and clear_resolved:
            raise invalid("resolved and clear_resolved are mutually exclusive")
        if resolved is not None:
            changes["resolved"] = validate_resolved(resolved)
        add = validate_id_list(add_regression_ids, "add_regression_ids") if add_regression_ids is not None else []
        remove = validate_id_list(remove_regression_ids, "remove_regression_ids") if remove_regression_ids is not None else []
        if set(add) & set(remove):
            raise invalid("a regression id cannot be both added and removed")
        if not changes and not clear_resolved and not add and not remove:
            raise invalid("no changes requested; supply at least one field to update")

        # Sippy's PUT replaces the whole record, so start from the complete current record.
        current = self._fetch(triage_id, acting_for)
        current_ids = _regression_ids(current)
        already_linked = sorted(set(add) & set(current_ids))
        if already_linked:
            raise invalid(f"regressions already linked to triage {triage_id}: {already_linked}")
        not_linked = sorted(set(remove) - set(current_ids))
        if not_linked:
            raise invalid(f"regressions not linked to triage {triage_id}: {not_linked}")

        body = {k: v for k, v in current.items() if k not in _READ_ONLY_FIELDS}
        new_ids = [i for i in current_ids if i not in remove] + add
        body["regressions"] = [{"id": i} for i in new_ids]
        for field in _EDITABLE_FIELDS:
            if field in changes:
                body[field] = changes[field]
        if "resolved" in changes:
            body["resolved"] = {"Time": changes["resolved"], "Valid": True}
        elif clear_resolved:
            body["resolved"] = dict(_UNRESOLVED)

        response = self._send("PUT", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=True, triage_id=triage_id, body=body)
        try:
            updated = decode_json(response)
        except ToolFailure:
            updated = None
        if not isinstance(updated, dict) or updated.get("id") != triage_id:
            raise ToolFailure(
                "upstream_error",
                "Sippy accepted the update but returned an unexpected triage record; read the triage to see its state",
                triage_id=triage_id,
            )

        changed_fields = [k for k in _EDITABLE_FIELDS if k in changes]
        if "resolved" in changes or clear_resolved:
            changed_fields.append("resolved")
        if add or remove:
            changed_fields.append("regression_ids")
        return {"triage": summarize_triage(updated, include_regressions=True), "changed_fields": sorted(changed_fields)}

    @tool_result
    def delete_triage(self, acting_for: str, triage_id: int) -> dict[str, Any]:
        acting_for = validate_acting_for(acting_for)
        triage_id = validate_id(triage_id, "triage_id")
        self._send("DELETE", f"{TRIAGES_PATH}/{triage_id}", acting_for, mutation=True, triage_id=triage_id)
        return {"deleted": True, "triage_id": triage_id}
