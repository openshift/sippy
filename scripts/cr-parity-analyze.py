#!/usr/bin/env python3
"""
CR Parity Analyze: Investigate the root cause of BQ/PG parity mismatches.

Takes JSON output from cr-parity-check.py and drills into the test_details
API to compare per-job run lists between providers. Cross-references job
names against config/openshift.yaml and reports every contributing factor
per cell (a cell often has more than one):
  - job_scope           BQ-only jobs not in config (TRT-2861)
  - ingestion_gap_bq    job in config, BQ has runs, PG none (e.g. ROSA, TRT-2912)
  - ingestion_gap_pg    PG has runs, BQ none
  - run_count_divergence shared jobs, different totals (dedup, TRT-2804);
                        net direction (BQ-PG) is reported
  - outcome_divergence  same total runs, different pass/fail/flake
  - pg_capability_empty PG returns no capability/test rows for a component
                        that BQ populates (TRT-2911)

Grid-level mismatches (component x variant cells) have no test_id. This
script resolves test_ids by drilling into the CR grid for BOTH providers,
matching tests across them, and picking a representative test that actually
reflects the cell's status flip (rather than an arbitrary first test).

Usage:
    python scripts/cr-parity-check.py --json 2>/dev/null > parity.json
    python scripts/cr-parity-analyze.py parity.json

Examples:
    # Analyze up to 5 mismatches per scenario
    python scripts/cr-parity-analyze.py parity.json --max-mismatches 5

    # Use a different config file
    python scripts/cr-parity-analyze.py parity.json --config path/to/openshift.yaml

    # Verbose: show individual job run details
    python scripts/cr-parity-analyze.py parity.json --verbose

    # JSON output
    python scripts/cr-parity-analyze.py parity.json --json
"""

import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import defaultdict

DEFAULT_SERVER = "https://sippy-staging.dptools.openshift.org"
DEFAULT_CONFIG = "config/openshift.yaml"
DEFAULT_TIMEOUT = 120


def parse_args():
    p = argparse.ArgumentParser(
        description="Analyze root causes of BQ/PG parity mismatches"
    )
    p.add_argument(
        "input_file",
        help="JSON output from cr-parity-check.py --json",
    )
    p.add_argument(
        "--server",
        help="Sippy server URL (default: inferred from input JSON)",
    )
    p.add_argument(
        "--config",
        default=DEFAULT_CONFIG,
        help=f"Path to config/openshift.yaml (default: {DEFAULT_CONFIG})",
    )
    p.add_argument(
        "--max-mismatches",
        type=int,
        default=10,
        help="Max mismatches to analyze per scenario (default: 10)",
    )
    p.add_argument(
        "--timeout",
        type=int,
        default=DEFAULT_TIMEOUT,
        help=f"HTTP request timeout in seconds (default: {DEFAULT_TIMEOUT})",
    )
    p.add_argument(
        "--verbose",
        action="store_true",
        help="Show per-job run details for each mismatch",
    )
    p.add_argument(
        "--json",
        action="store_true",
        help="Output results as JSON",
    )
    return p.parse_args()


def log(msg, args, **kwargs):
    dest = sys.stderr if args.json else sys.stdout
    print(msg, file=dest, **kwargs)


def load_config_jobs(config_path):
    """Parse config/openshift.yaml and extract the job set per release.

    Uses a simple line-by-line parser that handles the flat
    'job-name: true' structure under each release's jobs: block.
    """
    jobs_by_release = {}
    try:
        import yaml
        with open(config_path) as f:
            config = yaml.safe_load(f)
        for release, rdata in config.get("releases", {}).items():
            jobs_by_release[str(release)] = set(rdata.get("jobs", {}).keys())
        return jobs_by_release
    except ImportError:
        pass

    # Fallback: minimal line parser for the flat jobs map
    jobs_by_release = {}
    current_release = None
    in_jobs = False

    try:
        with open(config_path) as f:
            for line in f:
                stripped = line.strip()
                if not stripped or stripped.startswith("#"):
                    continue

                indent = len(line) - len(line.lstrip())

                if indent == 4 and stripped.startswith('"') and stripped.endswith('":'):
                    current_release = stripped.strip('"').rstrip(":")
                    in_jobs = False
                    if current_release not in jobs_by_release:
                        jobs_by_release[current_release] = set()
                elif indent == 8 and stripped == "jobs:":
                    in_jobs = True
                elif indent == 8 and stripped != "jobs:" and not stripped.endswith(": true"):
                    in_jobs = False
                elif indent == 12 and in_jobs and current_release:
                    job_name = stripped.split(":")[0].strip()
                    jobs_by_release[current_release].add(job_name)
    except FileNotFoundError:
        print(f"Warning: config file not found: {config_path}", file=sys.stderr)

    return jobs_by_release


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def fetch_json(url, timeout):
    """Fetch JSON from a full URL. Returns (data, elapsed, error)."""
    req = urllib.request.Request(url)
    req.add_header("Accept", "application/json")
    start = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            elapsed = time.monotonic() - start
            body = resp.read().decode("utf-8")
            return json.loads(body), elapsed, None
    except urllib.error.HTTPError as e:
        elapsed = time.monotonic() - start
        body = ""
        try:
            body = e.read().decode("utf-8", errors="replace")[:200]
        except Exception:
            pass
        return None, elapsed, f"HTTP {e.code}: {body}"
    except urllib.error.URLError as e:
        return None, time.monotonic() - start, f"URL error: {e.reason}"
    except TimeoutError:
        return None, time.monotonic() - start, f"Timeout after {timeout}s"


def api_get(server, path, params, timeout):
    """Make a GET request and return (json, elapsed, url, error)."""
    query_parts = []
    for key, value in params:
        encoded_val = urllib.parse.quote(str(value), safe="")
        query_parts.append(f"{key}={encoded_val}")
    query_string = "&".join(query_parts)

    url = f"{server.rstrip('/')}{path}"
    if query_string:
        url += f"?{query_string}"

    data, elapsed, err = fetch_json(url, timeout)
    return data, elapsed, url, err


# ---------------------------------------------------------------------------
# URL and variant helpers
# ---------------------------------------------------------------------------

def add_url_param(url, key, value):
    """Append a query parameter to a URL."""
    separator = "&" if "?" in url else "?"
    encoded_value = urllib.parse.quote(str(value), safe="")
    return f"{url}{separator}{key}={encoded_value}"


def make_variant_key(variants):
    """Create a hashable key from a variant dict, filtering empty values."""
    filtered = {k: v for k, v in variants.items() if v != ""}
    return tuple(sorted(filtered.items()))


def extract_view_from_url(url):
    """Extract the view parameter from a parity check URL."""
    parsed = urllib.parse.urlparse(url)
    params = urllib.parse.parse_qs(parsed.query)
    views = params.get("view", [])
    return views[0] if views else None


def extract_release_from_view(view_name):
    """Extract the release version from a view name like '5.0-main'."""
    if not view_name:
        return None
    parts = view_name.split("-", 1)
    return parts[0] if parts else None


# ---------------------------------------------------------------------------
# Test ID resolution for grid-level mismatches
# ---------------------------------------------------------------------------

def index_cell_tests(data):
    """Index a component-drilled grid response as vkey -> {test_id: test_info}.

    Each test_info carries the full variant set and per-test status so callers
    can match tests across providers and pick the one driving a cell's flip.
    """
    cols = defaultdict(dict)
    if not data:
        return cols
    for row in data.get("rows", []):
        for col in row.get("columns", []):
            vkey = make_variant_key(col.get("variants", {}))
            for t in col.get("all_tests", []) + col.get("regressed_tests", []):
                tid = t.get("test_id", "")
                full_variants = t.get("variants", {})
                if tid and full_variants:
                    cols[vkey][tid] = {
                        "test_id": tid,
                        "test_name": t.get("test_name", ""),
                        "capability": t.get("capability", ""),
                        "full_variants": full_variants,
                        "status": t.get("status", 0),
                    }
    return cols


def choose_representative(bq_tests, pg_tests, mismatch):
    """Pick the test that best explains a cell-level status mismatch.

    Prefers a test present in BOTH providers whose per-test status flip matches
    the cell's flip; then any shared test whose status differs; then any shared
    test; then a BQ test matching the cell's BQ status; then any available test.
    Picking the first test (the old behavior) frequently landed on a test with
    no sample data, mis-attributing the cause.
    """
    cell_bq = mismatch.get("bq_status")
    cell_pg = mismatch.get("pg_status")
    shared = set(bq_tests) & set(pg_tests)

    for tid in shared:
        if bq_tests[tid]["status"] == cell_bq and pg_tests[tid]["status"] == cell_pg:
            return bq_tests[tid]
    for tid in shared:
        if bq_tests[tid]["status"] != pg_tests[tid]["status"]:
            return bq_tests[tid]
    if shared:
        return bq_tests[next(iter(shared))]
    for t in bq_tests.values():
        if t["status"] == cell_bq:
            return t
    if bq_tests:
        return next(iter(bq_tests.values()))
    if pg_tests:
        return next(iter(pg_tests.values()))
    return None


def resolve_test_ids(scenario_result, mismatches, timeout, args):
    """Drill into the CR grid (BOTH providers) to find test_ids for cell mismatches.

    Grid-level mismatches have empty test_id because they represent aggregated
    component x variant cells. This drills each component in BQ and PG, matches
    tests across providers, and picks a representative test that actually
    reflects the cell's status flip. When PG returns no capability/test rows for
    a component that BQ populates, the cell is flagged pg_drill_empty (TRT-2911).
    """
    bq_url = scenario_result.get("bq_url", "")
    pg_url = scenario_result.get("pg_url", "")
    if not bq_url:
        return

    by_component = defaultdict(list)
    for m in mismatches:
        if not m.get("test_id"):
            by_component[m["component"]].append(m)

    if not by_component:
        return

    for component, comp_mismatches in by_component.items():
        bq_drill = add_url_param(add_url_param(bq_url, "component", component), "includeAllTests", "true")
        pg_drill = ""
        if pg_url:
            pg_drill = add_url_param(add_url_param(pg_url, "component", component), "includeAllTests", "true")

        short_name = component.split(" / ")[-1] if " / " in component else component
        log(f"    Resolving tests for {short_name}...", args, end="", flush=True)

        bq_data, elapsed, err = fetch_json(bq_drill, timeout)
        if err or not bq_data:
            log(f" error: {err}", args)
            continue

        pg_data = None
        if pg_drill:
            pg_data, _, _ = fetch_json(pg_drill, timeout)

        bq_cols = index_cell_tests(bq_data)
        pg_cols = index_cell_tests(pg_data) if pg_data is not None else {}

        resolved = 0
        pg_empty = 0
        for m in comp_mismatches:
            vkey = make_variant_key(m.get("variants", {}))
            bq_tests = bq_cols.get(vkey, {})
            pg_tests = pg_cols.get(vkey, {})

            # PG returns no capability/test rows where BQ does (TRT-2911)
            if pg_data is not None and bq_tests and not pg_tests:
                m["pg_drill_empty"] = True
                pg_empty += 1

            chosen = choose_representative(bq_tests, pg_tests, m)
            if chosen:
                m["resolved_test_id"] = chosen["test_id"]
                m["resolved_test_name"] = chosen["test_name"]
                m["resolved_variants"] = chosen["full_variants"]
                if chosen.get("capability"):
                    m["resolved_capability"] = chosen["capability"]
                resolved += 1

        suffix = f" [{pg_empty} PG-empty]" if pg_empty else ""
        log(f" {resolved}/{len(comp_mismatches)} resolved{suffix} ({elapsed:.1f}s)", args)


# ---------------------------------------------------------------------------
# Test details analysis
# ---------------------------------------------------------------------------

def build_test_details_params(mismatch, view_name):
    """Build query params for a test_details request from a mismatch entry.

    The test_details endpoint requires ALL dbGroupBy variant dimensions
    as query params (e.g., Architecture, FeatureSet, Installer, etc.).
    Uses resolved_variants (full set from grid drill-down) when available,
    falling back to the mismatch's column-level variants.
    """
    params = []
    if view_name:
        params.append(("view", view_name))

    params.append(("testId", mismatch["test_id"]))
    params.append(("component", mismatch["component"]))
    capability = mismatch.get("resolved_capability") or mismatch.get("capability")
    if capability:
        params.append(("capability", capability))

    variants = mismatch.get("resolved_variants") or mismatch.get("variants", {})
    for key, value in sorted(variants.items()):
        if value:
            params.append((key, value))

    return params


def extract_job_stats(test_details_data):
    """Extract per-job sample and base run info from a test_details response."""
    if not test_details_data:
        return {}, {}

    analyses = test_details_data.get("analyses", [])
    if not analyses:
        return {}, {}

    analysis = analyses[0]
    sample_jobs = {}
    base_jobs = {}

    for js in analysis.get("job_stats", []):
        sample_name = js.get("sample_job_name", "")
        base_name = js.get("base_job_name", "")
        sample_stats = js.get("sample_stats", {})
        base_stats = js.get("base_stats", {})

        sample_runs = (
            sample_stats.get("success_count", 0)
            + sample_stats.get("failure_count", 0)
            + sample_stats.get("flake_count", 0)
        )
        base_runs = (
            base_stats.get("success_count", 0)
            + base_stats.get("failure_count", 0)
            + base_stats.get("flake_count", 0)
        )

        if sample_name and sample_runs > 0:
            sample_jobs[sample_name] = {
                "runs": sample_runs,
                "success": sample_stats.get("success_count", 0),
                "failure": sample_stats.get("failure_count", 0),
                "flake": sample_stats.get("flake_count", 0),
            }
        if base_name and base_runs > 0:
            base_jobs[base_name] = {
                "runs": base_runs,
                "success": base_stats.get("success_count", 0),
                "failure": base_stats.get("failure_count", 0),
                "flake": base_stats.get("flake_count", 0),
            }

    return sample_jobs, base_jobs


def analyze_mismatch(server, mismatch, view_name, config_jobs, release, timeout):
    """Analyze a single mismatch by comparing test_details job lists."""
    params = build_test_details_params(mismatch, view_name)

    bq_params = params + [("dataSource", "bigquery")]
    bq_data, bq_time, _, bq_err = api_get(
        server, "/api/component_readiness/test_details", bq_params, timeout
    )

    pg_params = params + [("dataSource", "postgres")]
    pg_data, pg_time, _, pg_err = api_get(
        server, "/api/component_readiness/test_details", pg_params, timeout
    )

    result = {
        "test_id": mismatch["test_id"],
        "component": mismatch["component"],
        "capability": mismatch.get("capability", ""),
        "variants": mismatch.get("variants", {}),
        "bq_status": mismatch["bq_status"],
        "pg_status": mismatch["pg_status"],
        "bq_status_name": mismatch["bq_status_name"],
        "pg_status_name": mismatch["pg_status_name"],
        "bq_time_s": round(bq_time, 2),
        "pg_time_s": round(pg_time, 2),
    }

    if mismatch.get("resolved_test_name"):
        result["resolved_test_name"] = mismatch["resolved_test_name"]

    if bq_err or pg_err:
        result["error"] = bq_err or pg_err
        result["cause"] = "error"
        return result

    bq_sample, bq_base = extract_job_stats(bq_data)
    pg_sample, pg_base = extract_job_stats(pg_data)

    release_jobs = config_jobs.get(release, set())

    # Analyze sample period job differences
    sample_analysis = analyze_job_diff(bq_sample, pg_sample, release_jobs)
    base_analysis = analyze_job_diff(bq_base, pg_base, release_jobs)

    result["sample"] = sample_analysis
    result["base"] = base_analysis

    # A cell can have several contributing factors at once. Report all of them
    # (result["causes"]) and pick a primary in priority order for summaries.
    bq_only_not_in_config = (
        sample_analysis["bq_only_not_in_config_runs"]
        + base_analysis["bq_only_not_in_config_runs"]
    )
    bq_only_in_config = (
        sample_analysis["bq_only_in_config_runs"]
        + base_analysis["bq_only_in_config_runs"]
    )
    pg_only_runs = sample_analysis["pg_only_runs"] + base_analysis["pg_only_runs"]
    shared_diff_runs = (
        sample_analysis["shared_run_diff"] + base_analysis["shared_run_diff"]
    )
    content_diff = (
        len(sample_analysis["shared_content_divergent"])
        + len(base_analysis["shared_content_divergent"])
    )
    net_diff = sample_analysis["net_run_diff"] + base_analysis["net_run_diff"]

    result["net_run_diff"] = net_diff
    result["run_diff_direction"] = (
        "BQ>PG" if net_diff > 0 else ("PG>BQ" if net_diff < 0 else "even")
    )

    causes = []
    if mismatch.get("pg_drill_empty"):
        # PG has no capability/test rows for this component; the representative
        # test is BQ-only by construction, so job-diff factors would be noise.
        causes = ["pg_capability_empty"]
    else:
        if bq_only_not_in_config > 0:
            causes.append("job_scope")
        if bq_only_in_config > 0:
            causes.append("ingestion_gap_bq")
        if pg_only_runs > 0:
            causes.append("ingestion_gap_pg")
        if shared_diff_runs > 0:
            causes.append("run_count_divergence")
        if content_diff > 0:
            causes.append("outcome_divergence")
        if not causes:
            if not bq_sample and not pg_sample and not bq_base and not pg_base:
                causes.append("no_job_stats")
            else:
                causes.append("unknown")

    result["causes"] = causes
    result["cause"] = causes[0]

    return result


def analyze_job_diff(bq_jobs, pg_jobs, config_jobs):
    """Compare job sets between BQ and PG, cross-reference against config."""
    bq_names = set(bq_jobs.keys())
    pg_names = set(pg_jobs.keys())

    bq_only = bq_names - pg_names
    pg_only = pg_names - bq_names
    shared = bq_names & pg_names

    bq_only_in_config = []
    bq_only_not_in_config = []
    for name in sorted(bq_only):
        entry = {"name": name, **bq_jobs[name]}
        if name in config_jobs:
            entry["in_config"] = True
            bq_only_in_config.append(entry)
        else:
            entry["in_config"] = False
            bq_only_not_in_config.append(entry)

    pg_only_list = [{"name": name, **pg_jobs[name]} for name in sorted(pg_only)]

    shared_divergent = []          # total run count differs
    shared_content_divergent = []  # same total, different success/failure/flake
    shared_run_diff = 0            # absolute magnitude
    net_run_diff = 0              # signed (BQ - PG): direction of the divergence
    for name in sorted(shared):
        b = bq_jobs[name]
        p = pg_jobs[name]
        bq_runs = b["runs"]
        pg_runs = p["runs"]
        if bq_runs != pg_runs:
            shared_divergent.append({
                "name": name,
                "bq_runs": bq_runs,
                "pg_runs": pg_runs,
                "diff": bq_runs - pg_runs,
                "bq_success": b["success"], "pg_success": p["success"],
                "bq_failure": b["failure"], "pg_failure": p["failure"],
                "bq_flake": b["flake"], "pg_flake": p["flake"],
            })
            shared_run_diff += abs(bq_runs - pg_runs)
            net_run_diff += bq_runs - pg_runs
        elif (b["success"], b["failure"], b["flake"]) != (p["success"], p["failure"], p["flake"]):
            shared_content_divergent.append({
                "name": name,
                "runs": bq_runs,
                "bq_success": b["success"], "pg_success": p["success"],
                "bq_failure": b["failure"], "pg_failure": p["failure"],
                "bq_flake": b["flake"], "pg_flake": p["flake"],
            })

    return {
        "bq_only_not_in_config": bq_only_not_in_config,
        "bq_only_not_in_config_runs": sum(j["runs"] for j in bq_only_not_in_config),
        "bq_only_in_config": bq_only_in_config,
        "bq_only_in_config_runs": sum(j["runs"] for j in bq_only_in_config),
        "pg_only": pg_only_list,
        "pg_only_runs": sum(j["runs"] for j in pg_only_list),
        "shared_divergent": shared_divergent,
        "shared_run_diff": shared_run_diff,
        "net_run_diff": net_run_diff,
        "shared_content_divergent": shared_content_divergent,
        "bq_job_count": len(bq_names),
        "pg_job_count": len(pg_names),
        "shared_job_count": len(shared),
    }


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

CAUSE_LABELS = {
    "pg_capability_empty": "PG drill-down empty (no capability/test rows in PG)",
    "job_scope": "Job scope (BQ-only jobs not in config)",
    "job_scope_primary": "Job scope (primary) + other factors",
    "ingestion_gap": "Ingestion gap (job in config but missing from one provider)",
    "ingestion_gap_bq": "Ingestion gap (job in config, BQ has runs, PG none)",
    "ingestion_gap_pg": "Ingestion gap (PG has runs, BQ none)",
    "run_count_divergence": "Run count divergence (shared jobs, different counts)",
    "outcome_divergence": "Outcome divergence (same runs, different pass/fail/flake)",
    "no_job_stats": "No job stats available",
    "no_test_id": "Could not resolve test ID from grid",
    "unknown": "Unknown",
    "error": "API error",
}


def print_table_results(scenario_analyses, verbose):
    """Print analysis results as formatted tables."""
    line_width = 100

    print(f"\n{'=' * line_width}")
    print("CR PARITY GAP ANALYSIS")
    print(f"{'=' * line_width}")
    print()

    overall_causes = defaultdict(int)
    overall_factors = defaultdict(int)
    net_direction = {"BQ>PG": 0, "PG>BQ": 0, "even": 0}
    overall_bq_only_not_in_config_jobs = defaultdict(lambda: {"mismatches": 0, "runs": 0})

    for scenario in scenario_analyses:
        name = scenario["scenario_name"]
        analyses = scenario["analyses"]
        total = scenario["total_mismatches"]
        analyzed = len(analyses)

        print(f"ANALYSIS: {name} ({total} mismatches, {analyzed} analyzed)")
        print(f"{'─' * line_width}")

        cause_counts = defaultdict(int)
        for a in analyses:
            cause = a.get("cause", "unknown")
            cause_counts[cause] += 1
            overall_causes[cause] += 1
            for factor in a.get("causes", [cause]):
                overall_factors[factor] += 1
            direction = a.get("run_diff_direction")
            if direction in net_direction:
                net_direction[direction] += 1

            # Collect BQ-only jobs across all analyses
            for period in ("sample", "base"):
                period_data = a.get(period, {})
                for job in period_data.get("bq_only_not_in_config", []):
                    key = job["name"]
                    overall_bq_only_not_in_config_jobs[key]["mismatches"] += 1
                    overall_bq_only_not_in_config_jobs[key]["runs"] += job["runs"]

        header = f"  {'Cause':<55} {'Count':>6}"
        print(header)
        print(f"  {'─' * 62}")
        for cause, count in sorted(cause_counts.items(), key=lambda x: -x[1]):
            label = CAUSE_LABELS.get(cause, cause)
            print(f"  {label:<55} {count:>6}")
        print()

        if verbose:
            for a in analyses:
                comp_cap = a["component"]
                if a.get("capability"):
                    comp_cap += f"/{a['capability']}"
                print(f"  {comp_cap}")

                if a.get("resolved_test_name"):
                    print(f"    Representative test: {a['resolved_test_name']}")

                variants_str = ", ".join(
                    f"{k}={v}" for k, v in sorted(a.get("variants", {}).items())
                )
                if variants_str:
                    print(f"    Variants: {variants_str}")
                factors = a.get("causes", [a.get("cause", "unknown")])
                factor_str = ", ".join(CAUSE_LABELS.get(f, f) for f in factors)
                print(
                    f"    Status: BQ={a['bq_status_name']} PG={a['pg_status_name']}  "
                    f"Factors: {factor_str}"
                )
                if a.get("net_run_diff"):
                    print(
                        f"    Run-count net (BQ-PG): {a['net_run_diff']:+d} "
                        f"({a.get('run_diff_direction', '')})"
                    )

                for period in ("sample", "base"):
                    pd = a.get(period, {})
                    if not pd:
                        continue
                    bq_only_nc = pd.get("bq_only_not_in_config", [])
                    bq_only_ic = pd.get("bq_only_in_config", [])
                    pg_only = pd.get("pg_only", [])
                    shared_div = pd.get("shared_divergent", [])
                    content_div = pd.get("shared_content_divergent", [])

                    if bq_only_nc or bq_only_ic or pg_only or shared_div or content_div:
                        print(f"    {period.title()} period:")
                        print(
                            f"      Jobs: BQ={pd['bq_job_count']} PG={pd['pg_job_count']} "
                            f"shared={pd['shared_job_count']}"
                        )
                    for j in bq_only_nc:
                        print(
                            f"      BQ-only (NOT in config): {j['name']} "
                            f"({j['runs']} runs)"
                        )
                    for j in bq_only_ic:
                        print(
                            f"      BQ-only (IN config): {j['name']} "
                            f"({j['runs']} runs)"
                        )
                    for j in pg_only:
                        print(
                            f"      PG-only: {j['name']} ({j['runs']} runs)"
                        )
                    for j in shared_div:
                        print(
                            f"      Shared divergent: {j['name']} "
                            f"(BQ={j['bq_runs']} PG={j['pg_runs']} diff={j['diff']:+d})"
                        )
                    for j in content_div:
                        print(
                            f"      Same runs, diff outcome: {j['name']} "
                            f"(runs={j['runs']} BQ s/f/fl={j['bq_success']}/{j['bq_failure']}/{j['bq_flake']} "
                            f"PG s/f/fl={j['pg_success']}/{j['pg_failure']}/{j['pg_flake']})"
                        )
                print()

    # Overall summary
    total_analyzed = sum(overall_causes.values())
    if total_analyzed > 0:
        print(f"{'=' * line_width}")
        print("OVERALL SUMMARY")
        print(f"{'─' * line_width}")
        header = f"  {'Cause':<55} {'Count':>6} {'%':>6}"
        print(header)
        print(f"  {'─' * 68}")
        for cause, count in sorted(overall_causes.items(), key=lambda x: -x[1]):
            label = CAUSE_LABELS.get(cause, cause)
            pct = count / total_analyzed * 100
            print(f"  {label:<55} {count:>6} {pct:>5.0f}%")
        print(f"  {'─' * 68}")
        print(f"  {'Total analyzed (by primary cause)':<55} {total_analyzed:>6}")
        print()

        # A cell often has more than one contributing factor; the primary-cause
        # table above counts each cell once, so it can understate factors that
        # co-occur (e.g. run-count divergence alongside an ingestion gap).
        print("CONTRIBUTING FACTORS (co-occurring; a cell may count in several)")
        print(f"{'─' * line_width}")
        for factor, count in sorted(overall_factors.items(), key=lambda x: -x[1]):
            label = CAUSE_LABELS.get(factor, factor)
            pct = count / total_analyzed * 100
            print(f"  {label:<55} {count:>6} {pct:>5.0f}%")
        print()

        directional = sum(net_direction.values())
        if directional:
            print("RUN-COUNT DIVERGENCE DIRECTION (net BQ-PG per cell)")
            print(f"{'─' * line_width}")
            print(f"  {'BQ has more runs':<55} {net_direction['BQ>PG']:>6}")
            print(f"  {'PG has more runs':<55} {net_direction['PG>BQ']:>6}")
            print(f"  {'even':<55} {net_direction['even']:>6}")
            print()

    # Top BQ-only jobs not in config
    if overall_bq_only_not_in_config_jobs:
        print("BQ-ONLY JOBS (not in config/openshift.yaml)")
        print(f"{'─' * line_width}")
        header = f"  {'Job Name':<65} {'Mismatches':>10} {'Runs':>8}"
        print(header)
        print(f"  {'─' * 84}")
        for name, data in sorted(
            overall_bq_only_not_in_config_jobs.items(),
            key=lambda x: -x[1]["mismatches"],
        )[:20]:
            display_name = name if len(name) <= 64 else name[:61] + "..."
            print(f"  {display_name:<65} {data['mismatches']:>10} {data['runs']:>8}")
        if len(overall_bq_only_not_in_config_jobs) > 20:
            print(f"  ... and {len(overall_bq_only_not_in_config_jobs) - 20} more")
        print()


def print_json_output(scenario_analyses):
    output = {
        "scenario_analyses": scenario_analyses,
    }
    print(json.dumps(output, indent=2, default=str))


def main():
    args = parse_args()

    with open(args.input_file) as f:
        parity_data = json.load(f)

    results = parity_data.get("results", [])
    if not results:
        log("No results found in input file.", args)
        sys.exit(1)

    # Infer server from input data
    server = args.server
    if not server:
        for r in results:
            url = r.get("bq_url", "")
            if url:
                parsed = urllib.parse.urlparse(url)
                server = f"{parsed.scheme}://{parsed.netloc}"
                break
    if not server:
        server = DEFAULT_SERVER

    log(f"CR Parity Gap Analysis", args)
    log(f"Input: {args.input_file}", args)
    log(f"Server: {server}", args)
    log(f"Config: {args.config}", args)
    log(f"Max mismatches per scenario: {args.max_mismatches}", args)
    log("", args)

    # Load config jobs
    config_jobs = load_config_jobs(args.config)
    total_config_jobs = sum(len(v) for v in config_jobs.values())
    log(f"Loaded {total_config_jobs} jobs across {len(config_jobs)} releases from config", args)
    log("", args)

    # Find scenarios with mismatches
    cr_results = [
        r for r in results
        if r["endpoint"] == "/api/component_readiness"
        and r.get("comparison")
        and r["comparison"].get("mismatch_details")
    ]

    if not cr_results:
        log("No mismatches found to analyze.", args)
        sys.exit(0)

    total_mismatches = sum(
        len(r["comparison"]["mismatch_details"]) for r in cr_results
    )
    log(f"Found {len(cr_results)} scenarios with {total_mismatches} total mismatches", args)
    log("", args)

    scenario_analyses = []

    for r in cr_results:
        name = r["name"]
        mismatches = r["comparison"]["mismatch_details"]
        view_name = extract_view_from_url(r.get("bq_url", ""))
        release = extract_release_from_view(view_name)

        to_analyze = mismatches[: args.max_mismatches]

        log(f"Analyzing {name} ({len(to_analyze)}/{len(mismatches)} mismatches)...", args)

        # Resolve test_ids for grid-level mismatches (empty test_id)
        grid_level = [m for m in to_analyze if not m.get("test_id")]
        if grid_level:
            log(f"  Resolving test IDs for {len(grid_level)} grid-level mismatches...", args)
            resolve_test_ids(r, grid_level, args.timeout, args)

        log(f"  Fetching test details...", args)
        analyses = []
        for i, mismatch in enumerate(to_analyze):
            test_id = mismatch.get("resolved_test_id") or mismatch.get("test_id")

            label = mismatch["component"]
            if mismatch.get("capability"):
                label += f"/{mismatch['capability']}"
            log(
                f"    [{i + 1}/{len(to_analyze)}] {label}...",
                args,
                end="",
                flush=True,
            )

            if not test_id:
                log(f" could not resolve test ID", args)
                analyses.append({
                    "component": mismatch["component"],
                    "capability": mismatch.get("capability", ""),
                    "variants": mismatch.get("variants", {}),
                    "bq_status": mismatch["bq_status"],
                    "pg_status": mismatch["pg_status"],
                    "bq_status_name": mismatch["bq_status_name"],
                    "pg_status_name": mismatch["pg_status_name"],
                    "cause": "no_test_id",
                })
                continue

            mismatch_for_analysis = dict(mismatch)
            mismatch_for_analysis["test_id"] = test_id
            if mismatch.get("resolved_variants"):
                mismatch_for_analysis["resolved_variants"] = mismatch["resolved_variants"]
            if mismatch.get("resolved_capability"):
                mismatch_for_analysis["resolved_capability"] = mismatch["resolved_capability"]

            analysis = analyze_mismatch(
                server, mismatch_for_analysis, view_name, config_jobs, release, args.timeout
            )

            if mismatch.get("resolved_test_id"):
                analysis["resolved_test_id"] = mismatch["resolved_test_id"]
            if mismatch.get("resolved_test_name"):
                analysis["resolved_test_name"] = mismatch["resolved_test_name"]

            analyses.append(analysis)

            cause = analysis.get("cause", "unknown")
            log(f" {CAUSE_LABELS.get(cause, cause)}", args)

        scenario_analyses.append({
            "scenario_name": name,
            "total_mismatches": len(mismatches),
            "analyzed_count": len(analyses),
            "analyses": analyses,
        })

    log("", args)

    if args.json:
        print_json_output(scenario_analyses)
    else:
        print_table_results(scenario_analyses, args.verbose)


if __name__ == "__main__":
    main()
