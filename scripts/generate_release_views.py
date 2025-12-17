#!/usr/bin/env python3
"""
Generate new component readiness views for a target release by copying from a source release.

Usage:
    generate_release_views.py <source_release> <target_release> [--apply]

    --apply: Apply changes to config/views.yaml (default is preview mode)
"""
import sys
import copy
from ruamel.yaml import YAML

def increment_release(release):
    """Increment minor version (e.g., '4.20' -> '4.21')"""
    parts = release.split('.')
    if len(parts) != 2:
        return release
    major, minor = parts
    return f"{major}.{int(minor) + 1}"

def replace_ga_with_now(time_spec):
    """Replace 'ga' with 'now' in relative time specifications"""
    if isinstance(time_spec, str):
        return time_spec.replace('ga', 'now')
    return time_spec

def copy_and_update_view(view, source_release, target_release):
    """Create a copy of the view with updated releases"""
    new_view = copy.deepcopy(view)

    # Update name
    new_view['name'] = view['name'].replace(source_release, target_release)

    # Update sample release
    new_view['sample_release']['release'] = target_release

    # Update base release
    base_release = view['base_release']['release']
    if base_release == source_release:
        # Same-release comparison
        new_view['base_release']['release'] = target_release
    else:
        # Cross-release comparison - increment base
        new_base = increment_release(base_release)
        new_view['base_release']['release'] = new_base

        # If new base equals source release, replace 'ga' with 'now' in relative dates
        if new_base == source_release:
            if 'relative_start' in new_view['base_release']:
                new_view['base_release']['relative_start'] = \
                    replace_ga_with_now(new_view['base_release']['relative_start'])
            if 'relative_end' in new_view['base_release']:
                new_view['base_release']['relative_end'] = \
                    replace_ga_with_now(new_view['base_release']['relative_end'])

    return new_view

def main():
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        sys.exit(1)

    source_release = sys.argv[1]
    target_release = sys.argv[2]
    apply_changes = '--apply' in sys.argv
    config_file = 'config/views.yaml'

    # Use ruamel.yaml to preserve formatting
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.default_flow_style = False
    yaml.width = 4096  # Prevent line wrapping
    yaml.indent(mapping=2, sequence=2, offset=2)
    yaml.explicit_start = True  # Preserve the --- at the beginning
    # Preserve { } with space for empty dicts
    yaml.default_flow_style = None
    yaml.map_indent = 2
    yaml.sequence_indent = 2
    yaml.sequence_dash_offset = 0

    # Read YAML
    with open(config_file, 'r') as f:
        config = yaml.load(f)

    # Find and copy views
    new_views = []
    for view in config['component_readiness']:
        if view['sample_release']['release'] == source_release:
            new_view = copy_and_update_view(view, source_release, target_release)
            new_views.append(new_view)

    if not new_views:
        print(f"No views found with sample_release={source_release}", file=sys.stderr)
        sys.exit(1)

    # Preview
    print(f"Will create {len(new_views)} new views:")
    for view in new_views:
        print(f"  - {view['name']}")

    if not apply_changes:
        print("\nRun with --apply to add these views to config/views.yaml")
        return

    # Insert new views at the TOP of the list
    for i, new_view in enumerate(new_views):
        config['component_readiness'].insert(i, new_view)

    # Write back to file with formatting preservation
    import tempfile
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.yaml') as tmp:
        yaml.dump(config, tmp)
        tmp_name = tmp.name

    # Post-process to fix empty dict formatting: {} -> { }
    with open(tmp_name, 'r') as f:
        content = f.read()
    content = content.replace(': {}', ': { }')

    with open(config_file, 'w') as f:
        f.write(content)

    import os
    os.unlink(tmp_name)

    print(f"\nSuccessfully added {len(new_views)} new views to the top of {config_file}")

if __name__ == '__main__':
    main()
