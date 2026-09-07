#!/usr/bin/env python3
"""
CR Parity Check: Compare BigQuery and Postgres providers for Component Readiness reports.

Runs a well-distributed sampling of CR API configurations against both data sources,
comparing response times and result parity. Reports factual status-pair mismatches
without attributing causes. Use cr-parity-analyze.py on the JSON output to investigate
the root cause of mismatches against the actual data.

Usage:
    python scripts/cr-parity-check.py [options]

Examples:
    # Run against staging with default settings
    python scripts/cr-parity-check.py --stale-days 5

    # Run with DB probing for accurate date detection
    python scripts/cr-parity-check.py --database-dsn "$(cat ~/Documents/sippy-postgres/staging.dsn)"

    # Run against a local server, specific scenarios only
    python scripts/cr-parity-check.py --server http://localhost:9090 --scenarios views,cross-variant

    # JSON output for downstream analysis
    python scripts/cr-parity-check.py --json 2>/dev/null > parity.json
    python scripts/cr-parity-analyze.py parity.json
"""

import argparse
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request
from collections import defaultdict
from datetime import datetime, timedelta, timezone

DEFAULT_SERVER = "https://sippy-staging.dptools.openshift.org"
DEFAULT_RELEASES = "5.0,4.22"
DEFAULT_TIMEOUT = 120

STATUS_NAMES = {
    -1000: "FailedFixedRegression",
    -500: "ExtremeRegression",
    -400: "SignificantRegression",
    -300: "ExtremeTriagedRegression",
    -200: "SignificantTriagedRegression",
    -150: "FixedRegression",
    -100: "MissingSample",
    0: "NotSignificant",
    100: "MissingBasis",
    200: "MissingBasisAndSample",
    300: "SignificantImprovement",
}


def parse_args():
    p = argparse.ArgumentParser(
        description="Compare BQ and PG providers for Component Readiness reports"
    )
    p.add_argument(
        "--server",
        default=DEFAULT_SERVER,
        help=f"Sippy server URL (default: {DEFAULT_SERVER})",
    )
    p.add_argument(
        "--database-dsn",
        help="Optional PG DSN for data availability probing",
    )
    p.add_argument(
        "--releases",
        default=DEFAULT_RELEASES,
        help=f"Comma-separated releases to test (default: {DEFAULT_RELEASES})",
    )
    p.add_argument(
        "--stale-days",
        type=int,
        default=0,
        help="Shift 'now' back N days for date calculations on stale DBs (default: 0)",
    )
    p.add_argument(
        "--no-force-refresh",
        action="store_true",
        help="Disable forceRefresh (default: forceRefresh=true)",
    )
    p.add_argument(
        "--scenarios",
        default="all",
        help="Comma-separated: views,cross-variant,filtered,date-range,test-details,all (default: all)",
    )
    p.add_argument(
        "--verbose",
        action="store_true",
        help="Show per-row comparison details for mismatches",
    )
    p.add_argument(
        "--json",
        action="store_true",
        help="Output results as JSON",
    )
    p.add_argument(
        "--timeout",
        type=int,
        default=DEFAULT_TIMEOUT,
        help=f"HTTP request timeout in seconds (default: {DEFAULT_TIMEOUT})",
    )
    return p.parse_args()


def api_get(server, path, params, timeout):
    """Make a GET request to the Sippy API and return (response_json, elapsed_seconds)."""
    query_parts = []
    for key, value in params:
        encoded_val = urllib.request.quote(str(value), safe="")
        query_parts.append(f"{key}={encoded_val}")
    query_string = "&".join(query_parts)

    url = f"{server.rstrip('/')}{path}"
    if query_string:
        url += f"?{query_string}"

    req = urllib.request.Request(url)
    req.add_header("Accept", "application/json")

    start = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            elapsed = time.monotonic() - start
            body = resp.read().decode("utf-8")
            return json.loads(body), elapsed, url, None
    except urllib.error.HTTPError as e:
        elapsed = time.monotonic() - start
        body = ""
        try:
            body = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        return None, elapsed, url, f"HTTP {e.code}: {body[:200]}"
    except urllib.error.URLError as e:
        elapsed = time.monotonic() - start
        return None, elapsed, url, f"URL error: {e.reason}"
    except TimeoutError:
        elapsed = time.monotonic() - start
        return None, elapsed, url, f"Timeout after {timeout}s"


def probe_date_with_db(dsn, release, date_str):
    """Check if data exists for a specific release and date using psql."""
    sql = f"SELECT EXISTS(SELECT 1 FROM test_cumulative_summaries WHERE release = '{release}' AND date = '{date_str}')"
    try:
        result = subprocess.run(
            ["psql", dsn, "-t", "-A", "-c", sql],
            capture_output=True,
            text=True,
            timeout=10,
        )
        return result.stdout.strip() == "t"
    except (subprocess.TimeoutExpired, FileNotFoundError, subprocess.CalledProcessError):
        return None


def find_latest_date_with_db(dsn, release, max_days_back=90):
    """Binary search for the most recent date with data for a release."""
    today = datetime.now(timezone.utc).date()

    lo, hi = 1, max_days_back
    latest_found = None

    while lo <= hi:
        mid = (lo + hi) // 2
        probe_date = today - timedelta(days=mid)
        exists = probe_date_with_db(dsn, release, probe_date.isoformat())
        if exists is None:
            return None
        if exists:
            latest_found = probe_date
            hi = mid - 1
        else:
            lo = mid + 1

    if latest_found is None:
        for days_back in range(1, max_days_back + 1):
            probe_date = today - timedelta(days=days_back)
            exists = probe_date_with_db(dsn, release, probe_date.isoformat())
            if exists is None:
                return None
            if exists:
                return probe_date

    return latest_found


def find_latest_date_with_api(server, timeout, stale_days):
    """Use the views API to get resolved dates and infer data availability."""
    data, _, _, err = api_get(server, "/api/component_readiness/views", [], timeout)
    if err or not data:
        return {}

    available_dates = {}
    for view in data:
        sample = view.get("sample_release", {})
        release = sample.get("release", "")
        end_str = sample.get("end", "")
        if release and end_str:
            try:
                end_dt = datetime.fromisoformat(end_str.replace("Z", "+00:00"))
                effective = end_dt.date() - timedelta(days=stale_days)
                if release not in available_dates or effective > available_dates[release]:
                    available_dates[release] = effective
            except (ValueError, TypeError):
                continue
    return available_dates


def probe_data_availability(args):
    """Determine the latest date with data for each release."""
    releases = [r.strip() for r in args.releases.split(",")]
    available = {}

    if args.database_dsn:
        log("Probing data availability via database...", args)
        for release in releases:
            latest = find_latest_date_with_db(args.database_dsn, release)
            if latest:
                available[release] = latest
                log(f"  {release}: data available through {latest}", args)
            else:
                log(f"  {release}: no data found (or psql unavailable)", args)
    else:
        if args.stale_days > 0:
            log(f"Using API views endpoint with --stale-days={args.stale_days} offset...", args)
        else:
            log("Using API views endpoint for date resolution (use --database-dsn for precise probing)...", args)
        available = find_latest_date_with_api(args.server, args.timeout, args.stale_days)
        for release in releases:
            if release in available:
                log(f"  {release}: estimated data through {available[release]}", args)
            else:
                log(f"  {release}: not found in views (will use relative dates)", args)

    return available


def build_base_params(view_name, force_refresh):
    """Build the common query parameters for a scenario."""
    params = [("view", view_name)]
    if force_refresh:
        params.append(("forceRefresh", "true"))
    return params


def build_view_scenarios(releases, force_refresh):
    """Category 1: Named views (standard cross-release)."""
    scenarios = []

    view_map = {
        "5.0": [
            ("5.0-main", "Main grid (5.0)"),
            ("5.0-rosa", "ROSA filtered (5.0)"),
            ("5.0-hypershift-candidates", "Hypershift candidates (5.0)"),
        ],
        "4.22": [
            ("4.22-main", "Main grid (4.22)"),
            ("4.22-rosa", "ROSA filtered (4.22)"),
            ("4.22-hypershift-candidates", "Hypershift candidates (4.22)"),
        ],
    }

    for release in releases:
        for view_name, label in view_map.get(release, []):
            params = build_base_params(view_name, force_refresh)
            scenarios.append({
                "name": label,
                "category": "views",
                "endpoint": "/api/component_readiness",
                "params": params,
                "view": view_name,
            })

    return scenarios


def build_cross_variant_scenarios(releases, force_refresh):
    """Category 2: Cross-variant views."""
    scenarios = []

    cross_variant_map = {
        "5.0": [
            ("5.0-ha-vs-single", "HA vs Single (5.0)", "Topology"),
            ("5.0-x86-vs-multi-arm", "x86 vs ARM (5.0)", "Architecture"),
            ("5.0-rhcos10-vs-rhcos9", "RHCOS10 vs RHCOS9 (5.0)", "OS"),
            ("5.0-ha-vs-two-node-arbiter", "HA vs Two-Node Arbiter (5.0)", "Topology"),
        ],
        "4.22": [
            ("4.22-ha-vs-single", "HA vs Single (4.22)", "Topology"),
            ("4.22-x86-vs-multi-arm", "x86 vs ARM (4.22)", "Architecture"),
            ("4.22-ha-vs-two-node-arbiter", "HA vs Two-Node Arbiter (4.22)", "Topology"),
        ],
    }

    for release in releases:
        for view_name, label, cross_key in cross_variant_map.get(release, []):
            params = build_base_params(view_name, force_refresh)
            scenarios.append({
                "name": label,
                "category": "cross-variant",
                "endpoint": "/api/component_readiness",
                "params": params,
                "view": view_name,
                "cross_compare_key": cross_key,
            })

    return scenarios


def build_filtered_scenarios(releases, force_refresh):
    """Category 3: Custom filtered queries using a base view with overrides."""
    scenarios = []

    base_views = {"5.0": "5.0-main", "4.22": "4.22-main"}

    for release in releases:
        base_view = base_views.get(release)
        if not base_view:
            continue

        # Component filter
        params = build_base_params(base_view, force_refresh)
        params.append(("component", "Etcd"))
        scenarios.append({
            "name": f"Component:Etcd ({release})",
            "category": "filtered",
            "endpoint": "/api/component_readiness",
            "params": params,
            "view": base_view,
        })

        # Single variant filter
        params = build_base_params(base_view, force_refresh)
        params.append(("includeVariant", "Platform:aws"))
        scenarios.append({
            "name": f"Platform:aws ({release})",
            "category": "filtered",
            "endpoint": "/api/component_readiness",
            "params": params,
            "view": base_view,
        })

        # Multi variant filter
        params = build_base_params(base_view, force_refresh)
        params.append(("includeVariant", "Platform:aws"))
        params.append(("includeVariant", "Architecture:amd64"))
        scenarios.append({
            "name": f"Platform:aws+Arch:amd64 ({release})",
            "category": "filtered",
            "endpoint": "/api/component_readiness",
            "params": params,
            "view": base_view,
        })

        # Column group by Topology
        params = build_base_params(base_view, force_refresh)
        params.append(("columnGroupBy", "Topology"))
        scenarios.append({
            "name": f"ColGroupBy:Topology ({release})",
            "category": "filtered",
            "endpoint": "/api/component_readiness",
            "params": params,
            "view": base_view,
        })

        # Column group by Architecture
        params = build_base_params(base_view, force_refresh)
        params.append(("columnGroupBy", "Architecture"))
        scenarios.append({
            "name": f"ColGroupBy:Architecture ({release})",
            "category": "filtered",
            "endpoint": "/api/component_readiness",
            "params": params,
            "view": base_view,
        })

    return scenarios


def build_date_range_scenarios(releases, available_dates, force_refresh):
    """Category 4: Custom date ranges with different sample window sizes."""
    scenarios = []

    release_bases = {"5.0": "4.22", "4.22": "4.21"}
    db_group_by = "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer,LayeredProduct"
    column_group_by = "Platform,Architecture,Network"

    for release in releases:
        latest = available_dates.get(release)
        base_release = release_bases.get(release)
        if not latest or not base_release:
            continue

        for window_days, label in [(7, "7d"), (14, "14d"), (30, "30d")]:
            sample_end = latest
            sample_start = sample_end - timedelta(days=window_days)

            params = [
                ("baseRelease", base_release),
                ("sampleRelease", release),
                ("sampleStartTime", datetime.combine(sample_start, datetime.min.time(), tzinfo=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")),
                ("sampleEndTime", datetime.combine(sample_end, datetime.max.time().replace(microsecond=0), tzinfo=timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")),
                ("dbGroupBy", db_group_by),
                ("columnGroupBy", column_group_by),
                ("confidence", "95"),
                ("pity", "5"),
                ("minFail", "3"),
                ("ignoreDisruption", "true"),
            ]
            if force_refresh:
                params.append(("forceRefresh", "true"))

            scenarios.append({
                "name": f"{release} {label} window ({sample_start} to {sample_end})",
                "category": "date-range",
                "endpoint": "/api/component_readiness",
                "params": params,
            })

    return scenarios


def extract_test_detail_targets(cr_response, view_name, max_targets=3):
    """Pick test targets from a CR response for test_details drilldown.

    The main CR grid has rows at the component level (no test_id). Individual tests
    with their full variant sets are found in regressed_tests/all_tests within columns.
    """
    if not cr_response or "rows" not in cr_response:
        return []

    targets = []
    seen_components = set()

    for row in cr_response.get("rows", []):
        component = row.get("component", "")
        if component in seen_components:
            continue

        for col in row.get("columns", []):
            # Look in regressed_tests first, then all_tests
            test_lists = col.get("regressed_tests", []) + col.get("all_tests", [])
            for test in test_lists:
                test_id = test.get("test_id", "")
                if not test_id:
                    continue

                test_component = test.get("component", component)
                test_capability = test.get("capability", "")
                variants = test.get("variants", {})
                status = test.get("status", col.get("status", 0))

                if not variants:
                    continue

                targets.append({
                    "test_id": test_id,
                    "component": test_component,
                    "capability": test_capability,
                    "variants": variants,
                    "status": status,
                })
                seen_components.add(component)
                break

            if component in seen_components:
                break

        if len(targets) >= max_targets:
            break

    return targets


def build_test_detail_scenarios(targets, view_name, force_refresh):
    """Category 5: Test details drilldown from extracted targets."""
    scenarios = []

    for i, target in enumerate(targets):
        params = [("view", view_name)]
        params.append(("testId", target["test_id"]))
        params.append(("component", target["component"]))
        if target["capability"]:
            params.append(("capability", target["capability"]))

        for key, value in sorted(target["variants"].items()):
            params.append((key, value))

        if force_refresh:
            params.append(("forceRefresh", "true"))

        status_name = STATUS_NAMES.get(target["status"], str(target["status"]))
        scenarios.append({
            "name": f"TestDetails: {target['component']}/{target['capability']} ({status_name})",
            "category": "test-details",
            "endpoint": "/api/component_readiness/test_details",
            "params": params,
            "view": view_name,
        })

    return scenarios


def make_row_key(row):
    """Create a hashable key for matching rows across BQ and PG."""
    return (
        row.get("component", ""),
        row.get("capability", ""),
        row.get("test_name", ""),
        row.get("test_id", ""),
    )


def make_column_key(col):
    """Create a hashable key for matching columns within a row."""
    variants = col.get("variants", {})
    return tuple(sorted(variants.items()))


def normalize_variant_key(col):
    """Normalize variant maps to handle empty-string vs missing key differences (TRT-2863)."""
    variants = col.get("variants", {})
    return {k: v for k, v in variants.items() if v != ""}



def check_last_failure_match(bq_data, pg_data):
    """Check whether last_failure timestamps match between BQ and PG test_details."""
    bq_analyses = bq_data.get("analyses", []) if bq_data else []
    pg_analyses = pg_data.get("analyses", []) if pg_data else []

    for bq_a, pg_a in zip(bq_analyses, pg_analyses):
        bq_lf = bq_a.get("last_failure")
        pg_lf = pg_a.get("last_failure")
        if bq_lf != pg_lf:
            return False
    return True


def compare_regressed_tests(bq_col, pg_col):
    """Compare regressed_tests arrays between BQ and PG for a single cell.

    Returns None if neither provider has regressed tests, otherwise returns
    a dict with bq_only_tests, pg_only_tests, and status_mismatches.
    """
    bq_tests = bq_col.get("regressed_tests") or []
    pg_tests = pg_col.get("regressed_tests") or []

    if not bq_tests and not pg_tests:
        return None

    bq_by_id = {}
    for t in bq_tests:
        key = t.get("test_id") or t.get("test_name", "")
        if key:
            bq_by_id[key] = t

    pg_by_id = {}
    for t in pg_tests:
        key = t.get("test_id") or t.get("test_name", "")
        if key:
            pg_by_id[key] = t

    bq_only_keys = set(bq_by_id.keys()) - set(pg_by_id.keys())
    pg_only_keys = set(pg_by_id.keys()) - set(bq_by_id.keys())
    shared_keys = set(bq_by_id.keys()) & set(pg_by_id.keys())

    bq_only_tests = []
    for key in sorted(bq_only_keys):
        t = bq_by_id[key]
        status = t.get("status", 0)
        bq_only_tests.append({
            "test_id": t.get("test_id", ""),
            "test_name": t.get("test_name", ""),
            "status": status,
            "status_name": STATUS_NAMES.get(status, str(status)),
        })

    pg_only_tests = []
    for key in sorted(pg_only_keys):
        t = pg_by_id[key]
        status = t.get("status", 0)
        pg_only_tests.append({
            "test_id": t.get("test_id", ""),
            "test_name": t.get("test_name", ""),
            "status": status,
            "status_name": STATUS_NAMES.get(status, str(status)),
        })

    status_mismatches = []
    for key in sorted(shared_keys):
        bq_t = bq_by_id[key]
        pg_t = pg_by_id[key]
        bq_status = bq_t.get("status", 0)
        pg_status = pg_t.get("status", 0)
        if bq_status != pg_status:
            status_mismatches.append({
                "test_id": bq_t.get("test_id", ""),
                "test_name": bq_t.get("test_name", ""),
                "bq_status": bq_status,
                "pg_status": pg_status,
                "bq_status_name": STATUS_NAMES.get(bq_status, str(bq_status)),
                "pg_status_name": STATUS_NAMES.get(pg_status, str(pg_status)),
            })

    if not bq_only_tests and not pg_only_tests and not status_mismatches:
        return {"match": True}

    return {
        "match": False,
        "bq_count": len(bq_tests),
        "pg_count": len(pg_tests),
        "bq_only_tests": bq_only_tests,
        "pg_only_tests": pg_only_tests,
        "status_mismatches": status_mismatches,
    }


def compare_cr_responses(bq_data, pg_data):
    """Compare two component readiness responses and return parity metrics."""
    if not bq_data or not pg_data:
        return {
            "bq_rows": len(bq_data.get("rows", [])) if bq_data else 0,
            "pg_rows": len(pg_data.get("rows", [])) if pg_data else 0,
            "matched_rows": 0,
            "bq_only_rows": 0,
            "pg_only_rows": 0,
            "status_matches": 0,
            "status_mismatches": 0,
            "status_match_pct": 0.0,
            "mismatch_details": [],
            "regressed_tests_total_cells": 0,
            "regressed_tests_match_cells": 0,
            "regressed_tests_mismatch_cells": 0,
            "regressed_tests_mismatches": [],
        }

    bq_rows = bq_data.get("rows", [])
    pg_rows = pg_data.get("rows", [])

    bq_by_key = {}
    for row in bq_rows:
        key = make_row_key(row)
        bq_by_key[key] = row

    pg_by_key = {}
    for row in pg_rows:
        key = make_row_key(row)
        pg_by_key[key] = row

    matched_keys = set(bq_by_key.keys()) & set(pg_by_key.keys())
    bq_only = set(bq_by_key.keys()) - set(pg_by_key.keys())
    pg_only = set(pg_by_key.keys()) - set(bq_by_key.keys())

    status_matches = 0
    status_mismatches = 0
    total_comparisons = 0
    mismatch_details = []
    status_pair_counts = defaultdict(int)
    rt_total_cells = 0
    rt_match_cells = 0
    rt_mismatch_cells = 0
    rt_mismatches = []

    for key in matched_keys:
        bq_row = bq_by_key[key]
        pg_row = pg_by_key[key]

        bq_cols = {make_column_key(c): c for c in bq_row.get("columns", [])}
        pg_cols = {make_column_key(c): c for c in pg_row.get("columns", [])}

        matched_col_keys = set(bq_cols.keys()) & set(pg_cols.keys())

        # Try normalized matching for unmatched columns (TRT-2863)
        bq_unmatched = set(bq_cols.keys()) - matched_col_keys
        pg_unmatched = set(pg_cols.keys()) - matched_col_keys

        if bq_unmatched or pg_unmatched:
            bq_norm = {}
            for ck in bq_unmatched:
                norm = tuple(sorted(normalize_variant_key(bq_cols[ck]).items()))
                bq_norm[norm] = ck

            for pck in list(pg_unmatched):
                norm = tuple(sorted(normalize_variant_key(pg_cols[pck]).items()))
                if norm in bq_norm:
                    bck = bq_norm[norm]
                    matched_col_keys.add(bck)
                    bq_unmatched.discard(bck)
                    pg_unmatched.discard(pck)

        for col_key in matched_col_keys:
            bq_col = bq_cols.get(col_key)
            pg_col = pg_cols.get(col_key)
            if not bq_col or not pg_col:
                for c in bq_row.get("columns", []):
                    if make_column_key(c) == col_key:
                        bq_col = c
                        break
                for c in pg_row.get("columns", []):
                    if make_column_key(c) == col_key:
                        pg_col = c
                        break
            if not bq_col or not pg_col:
                continue

            total_comparisons += 1
            bq_status = bq_col.get("status", 0)
            pg_status = pg_col.get("status", 0)

            if bq_status == pg_status:
                status_matches += 1
            else:
                status_mismatches += 1
                bq_name = STATUS_NAMES.get(bq_status, str(bq_status))
                pg_name = STATUS_NAMES.get(pg_status, str(pg_status))
                status_pair_counts[(bq_name, pg_name)] += 1
                mismatch_details.append({
                    "component": key[0],
                    "capability": key[1],
                    "test_name": key[2],
                    "test_id": key[3],
                    "variants": dict(col_key) if isinstance(col_key, tuple) else {},
                    "bq_status": bq_status,
                    "pg_status": pg_status,
                    "bq_status_name": bq_name,
                    "pg_status_name": pg_name,
                })

            rt_result = compare_regressed_tests(bq_col, pg_col)
            if rt_result is not None:
                rt_total_cells += 1
                if rt_result.get("match"):
                    rt_match_cells += 1
                else:
                    rt_mismatch_cells += 1
                    cell_variants = dict(col_key) if isinstance(col_key, tuple) else {}
                    rt_mismatches.append({
                        "component": key[0],
                        "capability": key[1],
                        "variants": cell_variants,
                        "cell_status_match": bq_status == pg_status,
                        "bq_count": rt_result["bq_count"],
                        "pg_count": rt_result["pg_count"],
                        "bq_only_tests": rt_result["bq_only_tests"],
                        "pg_only_tests": rt_result["pg_only_tests"],
                        "status_mismatches": rt_result["status_mismatches"],
                    })

    match_pct = (status_matches / total_comparisons * 100) if total_comparisons > 0 else 0.0

    return {
        "bq_rows": len(bq_rows),
        "pg_rows": len(pg_rows),
        "matched_rows": len(matched_keys),
        "bq_only_rows": len(bq_only),
        "pg_only_rows": len(pg_only),
        "status_matches": status_matches,
        "status_mismatches": status_mismatches,
        "total_comparisons": total_comparisons,
        "status_match_pct": match_pct,
        "mismatch_details": mismatch_details,
        "status_pair_counts": {f"BQ={k[0]}, PG={k[1]}": v for k, v in status_pair_counts.items()},
        "regressed_tests_total_cells": rt_total_cells,
        "regressed_tests_match_cells": rt_match_cells,
        "regressed_tests_mismatch_cells": rt_mismatch_cells,
        "regressed_tests_mismatches": rt_mismatches,
    }


def compare_test_detail_responses(bq_data, pg_data):
    """Compare two test_details responses."""
    if not bq_data or not pg_data:
        return {
            "has_data": False,
            "status_match": None,
            "count_match": None,
            "last_failure_match": None,
            "details": [],
        }

    bq_analyses = bq_data.get("analyses", [])
    pg_analyses = pg_data.get("analyses", [])

    if not bq_analyses or not pg_analyses:
        return {
            "has_data": bool(bq_analyses or pg_analyses),
            "status_match": None,
            "count_match": None,
            "last_failure_match": None,
            "details": [],
        }

    bq_a = bq_analyses[0]
    pg_a = pg_analyses[0]

    bq_status = bq_a.get("status", 0)
    pg_status = pg_a.get("status", 0)

    bq_sample = bq_a.get("sample_stats", {})
    pg_sample = pg_a.get("sample_stats", {})
    bq_base = bq_a.get("base_stats") or {}
    pg_base = pg_a.get("base_stats") or {}

    details = {
        "bq_status": bq_status,
        "pg_status": pg_status,
        "bq_sample_success": bq_sample.get("success_count", 0),
        "pg_sample_success": pg_sample.get("success_count", 0),
        "bq_sample_failure": bq_sample.get("failure_count", 0),
        "pg_sample_failure": pg_sample.get("failure_count", 0),
        "bq_base_success": bq_base.get("success_count", 0),
        "pg_base_success": pg_base.get("success_count", 0),
        "bq_base_failure": bq_base.get("failure_count", 0),
        "pg_base_failure": pg_base.get("failure_count", 0),
    }

    status_match = bq_status == pg_status
    sample_match = (
        bq_sample.get("success_count", 0) == pg_sample.get("success_count", 0)
        and bq_sample.get("failure_count", 0) == pg_sample.get("failure_count", 0)
        and bq_sample.get("flake_count", 0) == pg_sample.get("flake_count", 0)
    )
    base_match = (
        bq_base.get("success_count", 0) == pg_base.get("success_count", 0)
        and bq_base.get("failure_count", 0) == pg_base.get("failure_count", 0)
        and bq_base.get("flake_count", 0) == pg_base.get("flake_count", 0)
    )

    lf_match = check_last_failure_match(bq_data, pg_data)

    return {
        "has_data": True,
        "status_match": status_match,
        "count_match": sample_match and base_match,
        "last_failure_match": lf_match,
        "details": details,
    }


def run_scenario(server, scenario, timeout):
    """Run a single scenario against both BQ and PG, return results."""
    endpoint = scenario["endpoint"]
    params = list(scenario["params"])

    bq_params = params + [("dataSource", "bigquery")]
    bq_data, bq_time, bq_url, bq_err = api_get(server, endpoint, bq_params, timeout)

    pg_params = params + [("dataSource", "postgres")]
    pg_data, pg_time, pg_url, pg_err = api_get(server, endpoint, pg_params, timeout)

    result = {
        "name": scenario["name"],
        "category": scenario["category"],
        "endpoint": endpoint,
        "bq_time_s": round(bq_time, 2),
        "pg_time_s": round(pg_time, 2),
        "bq_error": bq_err,
        "pg_error": pg_err,
        "bq_url": bq_url,
        "pg_url": pg_url,
    }

    if bq_err or pg_err:
        result["comparison"] = None
        return result, bq_data, pg_data

    if endpoint == "/api/component_readiness/test_details":
        result["comparison"] = compare_test_detail_responses(bq_data, pg_data)
    else:
        result["comparison"] = compare_cr_responses(bq_data, pg_data)

    return result, bq_data, pg_data


def format_time(seconds):
    if seconds >= 60:
        return f"{seconds / 60:.1f}m"
    return f"{seconds:.1f}s"


def format_ratio(bq_time, pg_time):
    if pg_time == 0 or bq_time == 0:
        return "N/A"
    if pg_time < bq_time:
        return f"PG {bq_time / pg_time:.1f}x"
    return f"BQ {pg_time / bq_time:.1f}x"


def print_table_results(results, verbose):
    """Print results as formatted tables."""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    print(f"\n{'=' * 120}")
    print("CR PARITY CHECK RESULTS")
    print(f"{'=' * 120}")
    print(f"Date: {now}")
    print()

    name_width = 50
    line_width = 120

    # Performance comparison
    print("PERFORMANCE COMPARISON")
    print(f"{'─' * line_width}")
    header = f"{'Scenario':<{name_width}} {'BQ Time':>9} {'PG Time':>9} {'Faster':>12} {'Status':>10}"
    print(header)
    print(f"{'─' * line_width}")

    for r in results:
        if r.get("bq_error") or r.get("pg_error"):
            status = "ERROR"
            bq_t = r.get("bq_error", "OK")[:8] if r.get("bq_error") else format_time(r["bq_time_s"])
            pg_t = r.get("pg_error", "OK")[:8] if r.get("pg_error") else format_time(r["pg_time_s"])
            ratio = "N/A"
        else:
            bq_t = format_time(r["bq_time_s"])
            pg_t = format_time(r["pg_time_s"])
            ratio = format_ratio(r["bq_time_s"], r["pg_time_s"])
            status = "OK"

        name = r["name"][:name_width - 1]
        print(f"{name:<{name_width}} {bq_t:>9} {pg_t:>9} {ratio:>12} {status:>10}")

    print()

    # Parity comparison (only for CR reports, not test_details)
    cr_results = [r for r in results if r["endpoint"] == "/api/component_readiness" and r.get("comparison")]
    if cr_results:
        print("PARITY COMPARISON")
        print(f"{'─' * line_width}")
        header = f"{'Scenario':<{name_width}} {'BQ Rows':>8} {'PG Rows':>8} {'Matched':>8} {'Parity%':>8} {'Status':>10}"
        print(header)
        print(f"{'─' * line_width}")

        for r in cr_results:
            c = r["comparison"]
            name = r["name"][:name_width - 1]
            pct = f"{c['status_match_pct']:.0f}%"
            status = "MATCH" if c["status_match_pct"] == 100.0 else "MISMATCH"
            print(f"{name:<{name_width}} {c['bq_rows']:>8} {c['pg_rows']:>8} {c['matched_rows']:>8} {pct:>8} {status:>10}")

        print()

    # Regressed tests parity (only for CR reports with regressed test data)
    rt_results = [
        r for r in cr_results
        if r.get("comparison", {}).get("regressed_tests_total_cells", 0) > 0
    ]
    if rt_results:
        print("REGRESSED TESTS PARITY")
        print(f"{'─' * line_width}")
        header = f"{'Scenario':<{name_width}} {'Cells w/RT':>10} {'Match':>8} {'Mismatch':>10} {'Status':>10}"
        print(header)
        print(f"{'─' * line_width}")

        for r in rt_results:
            c = r["comparison"]
            name = r["name"][:name_width - 1]
            total = c["regressed_tests_total_cells"]
            match = c["regressed_tests_match_cells"]
            mismatch = c["regressed_tests_mismatch_cells"]
            status = "MATCH" if mismatch == 0 else "MISMATCH"
            print(f"{name:<{name_width}} {total:>10} {match:>8} {mismatch:>10} {status:>10}")

        print()

    # Test details comparison
    td_results = [r for r in results if r["endpoint"] == "/api/component_readiness/test_details" and r.get("comparison")]
    if td_results:
        print("TEST DETAILS COMPARISON")
        print(f"{'─' * line_width}")
        header = f"{'Scenario':<{name_width}} {'Status':>7} {'Counts':>7} {'LastFail':>9}"
        print(header)
        print(f"{'─' * line_width}")

        for r in td_results:
            c = r["comparison"]
            name = r["name"][:name_width - 1]
            if not c.get("has_data"):
                print(f"{name:<{name_width}} {'N/A':>7} {'N/A':>7} {'N/A':>9}")
                continue
            sm = "Match" if c["status_match"] else "Differ"
            cm = "Match" if c["count_match"] else "Differ"
            lf = "Match" if c.get("last_failure_match") else "Differ"
            print(f"{name:<{name_width}} {sm:>7} {cm:>7} {lf:>9}")

        print()

    # Overall mismatch summary by status pair
    total_pair_counts = defaultdict(int)
    for r in results:
        if r.get("comparison") and isinstance(r["comparison"].get("status_pair_counts"), dict):
            for pair, count in r["comparison"]["status_pair_counts"].items():
                total_pair_counts[pair] += count

    if total_pair_counts:
        total_mismatches = sum(total_pair_counts.values())
        print("MISMATCH SUMMARY")
        print(f"{'─' * line_width}")
        for pair, count in sorted(total_pair_counts.items(), key=lambda x: -x[1]):
            print(f"  {pair:<60} {count:>5}")
        print(f"  {'─' * 66}")
        print(f"  {'Total mismatches':<60} {total_mismatches:>5}")
        print()
        print("  Use cr-parity-analyze.py on --json output to investigate root causes.")
        print()

    # Regressed tests mismatch summary
    total_bq_only = 0
    total_pg_only = 0
    total_rt_status_mismatches = 0
    for r in results:
        for rt_m in r.get("comparison", {}).get("regressed_tests_mismatches", []):
            total_bq_only += len(rt_m.get("bq_only_tests", []))
            total_pg_only += len(rt_m.get("pg_only_tests", []))
            total_rt_status_mismatches += len(rt_m.get("status_mismatches", []))

    total_rt_discrepancies = total_bq_only + total_pg_only + total_rt_status_mismatches
    if total_rt_discrepancies > 0:
        print("REGRESSED TESTS MISMATCH SUMMARY")
        print(f"{'─' * line_width}")
        print(f"  {'BQ-only regressed tests (not regressed in PG)':<60} {total_bq_only:>5}")
        print(f"  {'PG-only regressed tests (not regressed in BQ)':<60} {total_pg_only:>5}")
        print(f"  {'Shared tests with different regression severity':<60} {total_rt_status_mismatches:>5}")
        print(f"  {'─' * 66}")
        print(f"  {'Total regressed test discrepancies':<60} {total_rt_discrepancies:>5}")
        print()

    # Verbose: per-row mismatch details
    if verbose:
        for r in results:
            if not r.get("comparison"):
                continue
            mismatches = r["comparison"].get("mismatch_details", [])
            if not mismatches:
                continue

            print(f"MISMATCH DETAILS: {r['name']}")
            print(f"{'─' * line_width}")
            for m in mismatches[:20]:
                variants_str = ", ".join(f"{k}={v}" for k, v in sorted(m.get("variants", {}).items()))
                print(f"  {m['component']}/{m['capability']}")
                if variants_str:
                    print(f"    Variants: {variants_str}")
                print(f"    BQ: {m['bq_status_name']} ({m['bq_status']})  PG: {m['pg_status_name']} ({m['pg_status']})")
            if len(mismatches) > 20:
                print(f"  ... and {len(mismatches) - 20} more")
            print()

    if verbose:
        for r in results:
            if not r.get("comparison"):
                continue
            rt_mismatches = r["comparison"].get("regressed_tests_mismatches", [])
            if not rt_mismatches:
                continue

            print(f"REGRESSED TESTS DETAILS: {r['name']}")
            print(f"{'─' * line_width}")
            for rt_m in rt_mismatches[:20]:
                variants_str = ", ".join(f"{k}={v}" for k, v in sorted(rt_m.get("variants", {}).items()))
                comp_cap = rt_m["component"]
                if rt_m.get("capability"):
                    comp_cap += f" / {rt_m['capability']}"
                print(f"  {comp_cap} ({variants_str})")
                status_label = "MATCH" if rt_m["cell_status_match"] else "MISMATCH"
                print(f"    Cell status: {status_label}  BQ regressed: {rt_m['bq_count']}  PG regressed: {rt_m['pg_count']}")

                for t in rt_m.get("bq_only_tests", []):
                    name = t["test_name"][:80] if t["test_name"] else t["test_id"][:40]
                    print(f"    BQ-only: {name} ({t['status_name']})")
                for t in rt_m.get("pg_only_tests", []):
                    name = t["test_name"][:80] if t["test_name"] else t["test_id"][:40]
                    print(f"    PG-only: {name} ({t['status_name']})")
                for t in rt_m.get("status_mismatches", []):
                    name = t["test_name"][:80] if t["test_name"] else t["test_id"][:40]
                    print(f"    Status diff: {name} (BQ={t['bq_status_name']} PG={t['pg_status_name']})")
            if len(rt_mismatches) > 20:
                print(f"  ... and {len(rt_mismatches) - 20} more")
            print()


def print_json_results(results):
    """Print results as JSON."""
    output = {
        "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "results": results,
    }
    print(json.dumps(output, indent=2, default=str))


def log(msg, args, **kwargs):
    """Print progress to stderr when in JSON mode, stdout otherwise."""
    dest = sys.stderr if args.json else sys.stdout
    print(msg, file=dest, **kwargs)


def main():
    args = parse_args()
    server = args.server.rstrip("/")
    force_refresh = not args.no_force_refresh
    releases = [r.strip() for r in args.releases.split(",")]
    scenario_types = set(args.scenarios.split(","))
    run_all = "all" in scenario_types

    log("CR Parity Check", args)
    log(f"Server: {server}", args)
    log(f"Releases: {', '.join(releases)}", args)
    log(f"Force refresh: {force_refresh}", args)
    log("", args)

    # Probe data availability
    available_dates = probe_data_availability(args)
    log("", args)

    # Build scenario list
    scenarios = []

    if run_all or "views" in scenario_types:
        scenarios.extend(build_view_scenarios(releases, force_refresh))

    if run_all or "cross-variant" in scenario_types:
        scenarios.extend(build_cross_variant_scenarios(releases, force_refresh))

    if run_all or "filtered" in scenario_types:
        scenarios.extend(build_filtered_scenarios(releases, force_refresh))

    if run_all or "date-range" in scenario_types:
        scenarios.extend(build_date_range_scenarios(releases, available_dates, force_refresh))

    if not scenarios:
        log("No scenarios to run (check --scenarios and --releases)", args)
        sys.exit(1)

    log(f"Running {len(scenarios)} scenarios...", args)
    log("", args)

    # Execute scenarios
    results = []
    test_detail_sources = {}

    for i, scenario in enumerate(scenarios):
        progress = f"[{i + 1}/{len(scenarios)}]"
        log(f"{progress} {scenario['name']}...", args, end="", flush=True)

        result, bq_data, pg_data = run_scenario(server, scenario, args.timeout)
        results.append(result)

        if result.get("bq_error") or result.get("pg_error"):
            errors = []
            if result.get("bq_error"):
                errors.append(f"BQ: {result['bq_error'][:50]}")
            if result.get("pg_error"):
                errors.append(f"PG: {result['pg_error'][:50]}")
            log(f" ERROR ({'; '.join(errors)})", args)
        else:
            comp = result.get("comparison", {})
            if scenario["endpoint"] == "/api/component_readiness":
                pct = comp.get("status_match_pct", 0)
                log(f" BQ={format_time(result['bq_time_s'])} PG={format_time(result['pg_time_s'])} parity={pct:.0f}%", args)

                # Save BQ response for test_details extraction
                if scenario["category"] == "views" and bq_data and scenario.get("view"):
                    test_detail_sources[scenario["view"]] = bq_data
            else:
                sm = "match" if comp.get("status_match") else "mismatch"
                log(f" BQ={format_time(result['bq_time_s'])} PG={format_time(result['pg_time_s'])} status={sm}", args)

    # Test details scenarios (built from collected CR responses)
    if run_all or "test-details" in scenario_types:
        td_scenarios = []
        for view_name in ["5.0-main", "4.22-main"]:
            cr_data = test_detail_sources.get(view_name)
            if cr_data:
                targets = extract_test_detail_targets(cr_data, view_name)
                td_scenarios.extend(build_test_detail_scenarios(targets, view_name, force_refresh))

        if td_scenarios:
            log(f"\nRunning {len(td_scenarios)} test_details scenarios...", args)
            for i, scenario in enumerate(td_scenarios):
                progress = f"[{i + 1}/{len(td_scenarios)}]"
                log(f"{progress} {scenario['name']}...", args, end="", flush=True)

                result, _, _ = run_scenario(server, scenario, args.timeout)
                results.append(result)

                if result.get("bq_error") or result.get("pg_error"):
                    log(" ERROR", args)
                else:
                    comp = result.get("comparison", {})
                    sm = "match" if comp.get("status_match") else "mismatch"
                    log(f" BQ={format_time(result['bq_time_s'])} PG={format_time(result['pg_time_s'])} status={sm}", args)

    # Output results
    log("", args)
    if args.json:
        print_json_results(results)
    else:
        print_table_results(results, args.verbose)


if __name__ == "__main__":
    main()
