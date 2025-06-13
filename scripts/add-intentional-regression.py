import sys
import requests
import json
import os
from urllib.parse import urlparse, unquote

# Define the base directory for all regressions
# This path is relative to where you expect the script to be run
# e.g., if running from the sippy repository root, this path will be correct.
BASE_REGRESSION_DIR = os.path.join('pkg', 'regressionallowances', 'regressions')

def get_jira_id_from_url(url):
    """Extracts the Jira ID (e.g., 'OCPBUGS-12345') from a Jira URL."""
    if not url:
        return ""
    parsed_url = urlparse(url)
    path_segments = parsed_url.path.split('/')
    if path_segments and path_segments[-1]:
        # Decode URL-encoded characters (like spaces becoming %20)
        return unquote(path_segments[-1])
    return ""

def process_regression_data(url, regression_id_to_match, reason_to_allow, release_version):
    """
    HTTP GETs a JSON endpoint, scans for a matching regression ID, and saves
    the relevant data to a JSON file within the specified directory structure,
    using release version, JiraBug, and JiraComponent in the path/filename.
    """
    try:
        response = requests.get(url)
        response.raise_for_status()  # Raise an exception for bad status codes
        data = response.json()
    except requests.exceptions.RequestException as e:
        print(f"Error fetching data from {url}: {e}")
        return
    except json.JSONDecodeError as e:
        print(f"Error decoding JSON from {url}: {e}")
        return

    # Construct the target directory for this release
    target_release_dir = os.path.join(BASE_REGRESSION_DIR, release_version)

    # Ensure the output directory exists
    try:
        os.makedirs(target_release_dir, exist_ok=True)
    except OSError as e:
        print(f"Error creating directory {target_release_dir}: {e}")
        return

    found_any_regression_match = False # Tracks if any regression with the ID was found
    processed_at_least_one_valid_regression = False # Tracks if any regression was successfully processed and written

    for row in data.get("rows", []):
        component = row.get("component")
        for column in row.get("columns", []):
            if "regressed_tests" in column:
                for test in column["regressed_tests"]:
                    if "regression" in test and test["regression"].get("id") == regression_id_to_match:
                        found_any_regression_match = True
                        jira_bug_url = ""
                        if test.get("regression") and test["regression"].get("triages"):
                            for triage in test["regression"]["triages"]:
                                if triage.get("url"):
                                    jira_bug_url = triage["url"]
                                    break

                        # --- NEW ERROR CHECK FOR MISSING JIRA BUG ---
                        if not jira_bug_url:
                            print(f"ERROR: Regression ID {regression_id_to_match} for test '{test.get('test_name', 'Unknown Test')}' (Component: {component}) has no associated Jira bug URL. This entry will be skipped.")
                            continue # Skip this specific test if no Jira bug is found
                        # --- END NEW ERROR CHECK ---

                        previous_successes = 0
                        previous_failures = 0
                        previous_flakes = 0
                        if "base_stats" in test:
                            previous_successes = test["base_stats"].get("success_count", 0)
                            previous_failures = test["base_stats"].get("failure_count", 0)
                            previous_flakes = test["base_stats"].get("flake_count", 0)

                        regressed_successes = 0
                        regressed_failures = 0
                        regressed_flakes = 0
                        if "sample_stats" in test:
                            regressed_successes = test["sample_stats"].get("success_count", 0)
                            regressed_failures = test["sample_stats"].get("failure_count", 0)
                            regressed_flakes = test["sample_stats"].get("flake_count", 0)

                        new_record = {
                            "JiraComponent": component,
                            "TestID": test.get("test_id"),
                            "TestName": test.get("test_name"),
                            "JiraBug": jira_bug_url,
                            "ReasonToAllowInsteadOfFix": reason_to_allow,
                            "variant": {
                                "variants": test.get("variants", {})
                            },
                            "PreviousSuccesses": previous_successes,
                            "PreviousFailures": previous_failures,
                            "PreviousFlakes": previous_flakes,
                            "RegressedSuccesses": regressed_successes,
                            "RegressedFailures": regressed_failures,
                            "RegressedFlakes": regressed_flakes
                        }

                        # --- FILENAME AND PATH GENERATION ---
                        jira_id = get_jira_id_from_url(jira_bug_url)
                        # Sanitize component name for filename if necessary (e.g., replace slashes)
                        sanitized_component = component.replace('/', '_').replace(' ', '_') if component else 'unknown_component'

                        # Example filename: OCPBUGS-12345_Networking.json
                        # Fallback if jira_id is somehow still empty despite the check (shouldn't happen with the new check)
                        if jira_id:
                            output_file_name = f"{jira_id}_{sanitized_component}.json"
                        else:
                            output_file_name = f"regression_{regression_id_to_match}_{sanitized_component}.json" # Fallback, though an error should occur before this.

                        output_filepath = os.path.join(target_release_dir, output_file_name)

                        # Load existing data from this specific output file, if it exists
                        existing_data = []
                        if os.path.exists(output_filepath):
                            try:
                                with open(output_filepath, 'r') as f:
                                    existing_data = json.load(f)
                                if not isinstance(existing_data, list):
                                    print(f"Warning: {output_filepath} does not contain a JSON list. Starting with an empty list.")
                                    existing_data = []
                            except json.JSONDecodeError as e:
                                print(f"Error decoding JSON from {output_filepath}: {e}. Starting with an empty list.")
                                existing_data = []

                        # Check if this exact record already exists to avoid duplicates if running multiple times
                        if new_record not in existing_data:
                            existing_data.append(new_record)
                            try:
                                with open(output_filepath, 'w') as f:
                                    json.dump(existing_data, f, indent=2)
                                print(f"Successfully updated {output_filepath} with data for regression ID {regression_id_to_match}.")
                                processed_at_least_one_valid_regression = True
                            except IOError as e:
                                print(f"Error writing to file {output_filepath}: {e}")
                        else:
                            print(f"Record for regression ID {regression_id_to_match} already exists in {output_filepath}. Skipping append.")
                        # --- END FILENAME AND PATH GENERATION ---

    if not found_any_regression_match:
        print(f"No regression with ID {regression_id_to_match} found in the fetched data.")
    elif not processed_at_least_one_valid_regression and found_any_regression_match:
        print(f"WARNING: Regression ID {regression_id_to_match} was found, but no valid records could be processed (e.g., all lacked Jira bugs or were duplicates).")


if __name__ == "__main__":
    if len(sys.argv) != 5:
        print("Usage: python add-intentional-regression.py <json_endpoint_url> <regression_id_to_match> <reason_to_allow_instead_of_fix> <release_version>")
        print("Example: python add-intentional-regression.py 'https://sippy.example.com/api/some_endpoint' 12345 'Known bug in external component' '4.19'")
        sys.exit(1)

    endpoint_url = sys.argv[1]
    try:
        regression_id = int(sys.argv[2])
    except ValueError:
        print("Error: The second argument (regression_id_to_match) must be an integer.")
        sys.exit(1)
    reason_to_allow = sys.argv[3]
    release_version = sys.argv[4] # The new argument for the release subdirectory name

    process_regression_data(endpoint_url, regression_id, reason_to_allow, release_version)
