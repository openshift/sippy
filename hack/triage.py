import argparse
import json
import subprocess
import tempfile
import requests
import yaml
import os

API_BEARER_TOKEN = os.getenv("API_BEARER_TOKEN")
HEADERS = {}
if API_BEARER_TOKEN:
    HEADERS["Authorization"] = f"Bearer {API_BEARER_TOKEN}"

DEFAULT_BASE_URL = "http://127.0.0.1:8080"
PROD_BASE_URL = "https://sippy-auth.dptools.openshift.org"

def fetch_json(url):
    response = requests.get(url, headers=HEADERS)
    response.raise_for_status()
    return response.json()

def put_json(url, data):
    response = requests.put(url, json=data, headers=HEADERS)
    response.raise_for_status()
    return response.json()

def post_json(url, data):
    response = requests.post(url, json=data, headers=HEADERS)
    response.raise_for_status()
    return response.json()

def edit_yaml_in_nvim(data):
    with tempfile.NamedTemporaryFile(suffix=".yaml", mode="w+", delete=False) as temp_file:
        yaml.dump(data, temp_file, default_flow_style=False, sort_keys=False)
        temp_file.flush()
        temp_filename = temp_file.name

    subprocess.run(["nvim", temp_filename])

    with open(temp_filename, "r") as temp_file:
        edited_data = yaml.safe_load(temp_file)

    return edited_data

def update_triage(triage_id, base_url):
    if not isinstance(triage_id, int):
        sys.exit(1)

    url = f"{base_url}/api/component_readiness/triages/{triage_id}"
    print(f"Fetching data from {url}...")
    data = fetch_json(url)

    print("Opening in nvim for editing...")
    edited_data = edit_yaml_in_nvim(data)

    print(f"Updating {url} with new data...")
    put_json(url, edited_data)
    print("Update successful.")

def create_triage(base_url):
    url = f"{base_url}/api/component_readiness/triages"
    template = {
        "url": "https://issues.redhat.com/browse/OCPBUGS-54222",
        "type": "ci-infra",  # Options: ci-infra, product-infra, product, test
        "regressions": [
            {"id": 1241}
        ]
    }

    print("Opening template in nvim for editing...")
    edited_data = edit_yaml_in_nvim(template)

    print(f"Creating new triage entry at {url}...")
    response = post_json(url, edited_data)
    print("Creation successful.", response)

def get_triage(triage_id, base_url):
    if not isinstance(triage_id, int):
        sys.exit(1)
    url = f"{base_url}/api/component_readiness/triages/{triage_id}"
    print(f"Fetching triage entry {triage_id}...")
    data = fetch_json(url)
    print(yaml.dump(data, default_flow_style=False, sort_keys=False))

def list_triages(base_url):
    url = f"{base_url}/api/component_readiness/triages"
    print("Fetching all triage entries...")
    data = fetch_json(url)
    print(yaml.dump(data, default_flow_style=False, sort_keys=False))

def main():
    parser = argparse.ArgumentParser(description="Triage tool")
    parser.add_argument("--prod", action="store_true", help="Use production API URL")
    subparsers = parser.add_subparsers(dest="command", required=True)

    update_parser = subparsers.add_parser("update", help="Update a triage entry")
    update_parser.add_argument("ID", type=int, help="Triage ID")

    create_parser = subparsers.add_parser("create", help="Create a new triage entry")

    get_parser = subparsers.add_parser("get", help="Get a triage entry")
    get_parser.add_argument("ID", type=int, help="Triage ID")

    list_parser = subparsers.add_parser("list", help="List all triage entries")

    args = parser.parse_args()
    base_url = PROD_BASE_URL if args.prod else DEFAULT_BASE_URL

    if args.command == "update":
        update_triage(args.ID, base_url)
    elif args.command == "create":
        create_triage(base_url)
    elif args.command == "get":
        get_triage(args.ID, base_url)
    elif args.command == "list":
        list_triages(base_url)

if __name__ == "__main__":
    main()
