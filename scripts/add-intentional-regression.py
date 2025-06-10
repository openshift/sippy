import sys
import requests
import json
import os

def process_regression_data(url, regression_id_to_match, reason_to_allow, output_file):
    """
    HTTP GETs a JSON endpoint, scans for a matching regression ID, and appends
    the relevant data to a local JSON file.
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

    # Load existing data from the output file, if it exists
    existing_data = []
    if os.path.exists(output_file):
        try:
            with open(output_file, 'r') as f:
                existing_data = json.load(f)
            if not isinstance(existing_data, list):
                print(f"Warning: {output_file} does not contain a JSON list. Starting with an empty list.")
                existing_data = []
        except json.JSONDecodeError as e:
            print(f"Error decoding JSON from {output_file}: {e}. Starting with an empty list.")
            existing_data = []

    found_regression = False
    for row in data.get("rows", []):
        component = row.get("component")
        for column in row.get("columns", []):
            if "regressed_tests" in column:
                for test in column["regressed_tests"]:
                    if "regression" in test and test["regression"].get("id") == regression_id_to_match:
                        found_regression = True
                        jira_bug_url = ""
                        if test.get("regression") and test["regression"].get("triages"):
                            for triage in test["regression"]["triages"]:
                                if triage.get("url"):
                                    jira_bug_url = triage["url"]
                                    break

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
                        existing_data.append(new_record)

    if not found_regression:
        print(f"No regression with ID {regression_id_to_match} found in the fetched data.")
        return

    try:
        with open(output_file, 'w') as f:
            json.dump(existing_data, f, indent=2)
        print(f"Successfully updated {output_file} with data for regression ID {regression_id_to_match}.")
    except IOError as e:
        print(f"Error writing to file {output_file}: {e}")

if __name__ == "__main__":
    if len(sys.argv) != 5:
        print("Usage: python script.py <json_endpoint_url> <regression_id_to_match> <reason_to_allow_instead_of_fix> <output_json_filename>")
        sys.exit(1)

    endpoint_url = sys.argv[1]
    try:
        regression_id = int(sys.argv[2])
    except ValueError:
        print("Error: The second argument (regression_id_to_match) must be an integer.")
        sys.exit(1)
    reason_to_allow = sys.argv[3]
    output_filename = sys.argv[4]

    process_regression_data(endpoint_url, regression_id, reason_to_allow, output_filename)
