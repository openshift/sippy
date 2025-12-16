---
name: sippy-generate-release-views [source-release] [target-release]
description: Generate new component readiness views for a new release
allowedTools:
  - Bash
  - Read
  - Write
  - TodoWrite
  - AskUserQuestion
---

# Generate Release Views

This command generates new component readiness views for a new release by copying and updating existing views from a previous release.

## Arguments (all optional)

- `[source-release]`: Source release version to copy views from (e.g., `4.21`)
- `[target-release]`: Target release version to create views for (e.g., `4.22`)

If any arguments are not provided, you will prompt the user interactively.

## Workflow

**IMPORTANT: Argument Parsing**
Before starting the workflow, check if the user provided any arguments:

1. Parse the command invocation to extract any provided arguments
2. If both arguments (source-release, target-release) are provided:
   - Validate the release format (must be X.Y, e.g., 4.21)
   - Use the provided values directly
3. If some arguments are missing, prompt only for the missing ones
4. If no arguments are provided, follow the full interactive workflow below

You will guide the user through the following steps (skipping steps where arguments were provided):

1. **Prompt for Source Release**: Ask the user for the source release version (e.g., `4.21`)
   - Format must be X.Y (e.g., 4.21, 4.20)
   - This is the release whose views will be copied

2. **Prompt for Target Release**: Ask the user for the target release version (e.g., `4.22`)
   - Format must be X.Y (e.g., 4.22, 4.23)
   - This is the new release for which views will be created

3. **Generate New Views**: Create a Python script to process the views.yaml file:
   - Read `config/views.yaml`
   - Find all views where `sample_release.release` equals the source release
   - For each matching view, create a new view with:
     - **Name**: Replace source release with target release (e.g., `4.21-main` → `4.22-main`)
     - **Sample Release**: Update `sample_release.release` to target release
     - **Base Release**:
       - If `base_release.release` equals source release (same-release comparison), update it to target release
       - If `base_release.release` is different from source release (cross-release comparison), increment it by one minor version
       - Example: If source=4.21 and base=4.20, then target=4.22 and base becomes 4.21
       - Example: If source=4.21 and base=4.21, then target=4.22 and base becomes 4.22
       - **IMPORTANT**: When base_release is updated to equal the source release, replace 'ga' with 'now' in relative_start and relative_end
       - Example: `relative_start: ga-30d` becomes `relative_start: now-30d`
       - Example: `relative_end: ga` becomes `relative_end: now`
       - Rationale: 'ga' refers to the GA date of the release, but source release is not GA when target starts development

4. **Preview Changes**: Show the user:
   - Number of views that will be created
   - List of view names that will be created
   - Ask for confirmation before proceeding

5. **Write Updated Config**: Append the new views to `config/views.yaml`
   - Preserve YAML formatting and structure
   - Add a comment before the new views: `# Generated views for release <target-release>`

6. **Verify Output**: Show a diff of the changes made to views.yaml

7. **Run Validation Test**: Execute the production views configuration test to verify the changes:
   - Run: `go test -v -run TestProductionViewsConfiguration ./pkg/flags/`
   - This validates the views.yaml structure and regression tracking constraints
   - If the test fails, the views.yaml has errors that must be fixed before committing

8. **Check Current Branch**: Before offering to commit, verify the current branch:
   - Run: `git branch --show-current`
   - If the current branch is `main` or `master`, skip the commit offer and warn the user:
     - "Changes have been made but not committed. You are on the main/master branch. Please create a feature branch before committing these changes."
   - If on any other branch, proceed to offer commit

9. **Offer to Commit**: Ask the user if they want to commit the changes. If yes, commit with the message:
   ```
   Add component readiness views for release <target-release>

   Generated <count> new views by copying from release <source-release>:
   <list of new view names, one per line>

   🤖 Generated with [Claude Code](https://claude.com/claude-code)

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
   ```

## Implementation Details

### View Update Logic

When copying a view from source release to target release:

1. **Name Update**: Replace all occurrences of source release in the name
   - `4.21-main` → `4.22-main`
   - `4.21-x86-vs-multi-arm` → `4.22-x86-vs-multi-arm`

2. **Sample Release Update**: Always set to target release
   - `sample_release.release: "4.21"` → `sample_release.release: "4.22"`

3. **Base Release Update**: Depends on the original relationship
   - **Same-release views** (base = sample): Both become target
     - Example: `4.21-x86-vs-multi-arm` has base=4.21, sample=4.21
     - Result: base=4.22, sample=4.22
   - **Cross-release views** (base ≠ sample): Base increments by one
     - Example: `4.21-main` has base=4.20, sample=4.21
     - Result: base=4.21, sample=4.22
     - **CRITICAL**: When base becomes source release, replace 'ga' → 'now' in relative dates
     - Why: 'ga' refers to GA date, but source is GA while target is in development
     - `relative_start: "ga-30d"` → `relative_start: "now-30d"`
     - `relative_end: "ga"` → `relative_end: "now"`

### Python Script Template

```python
#!/usr/bin/env python3
import sys
import yaml
import re

def increment_release(release):
    """Increment minor version (e.g., '4.20' -> '4.21')"""
    parts = release.split('.')
    if len(parts) != 2:
        return release
    major, minor = parts
    return f"{major}.{int(minor) + 1}"

def copy_and_update_view(view, source_release, target_release):
    """Create a copy of the view with updated releases"""
    import copy
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
                    new_view['base_release']['relative_start'].replace('ga', 'now')
            if 'relative_end' in new_view['base_release']:
                new_view['base_release']['relative_end'] = \
                    new_view['base_release']['relative_end'].replace('ga', 'now')

    return new_view

def main():
    source_release = sys.argv[1]
    target_release = sys.argv[2]
    config_file = 'config/views.yaml'

    # Read YAML
    with open(config_file, 'r') as f:
        config = yaml.safe_load(f)

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

    return new_views

if __name__ == '__main__':
    main()
```

## Important Notes

- The script preserves all other view settings (variant_options, advanced_options, etc.)
- Release versions must be in format X.Y (e.g., 4.21)
- The script only copies views where the source release is the sample_release
- Views that already use the source release as base_release will have their base_release incremented
- YAML formatting should be preserved using proper YAML libraries
- Always verify the diff before committing

## Examples

### Example 1: Creating 4.22 views from 4.21

Command: `/sippy-generate-release-views 4.21 4.22`

This will find all views with `sample_release: "4.21"` and create new views:
- `4.21-main` (base: 4.20, sample: 4.21) → `4.22-main` (base: 4.21, sample: 4.22)
  - Original: `base_release: {release: "4.20", relative_start: "ga-30d", relative_end: "ga"}`
  - Updated: `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
  - Note: 'ga' changed to 'now' because base now references source release (4.21) which is GA, not target (4.22)
- `4.21-x86-vs-multi-arm` (base: 4.21, sample: 4.21) → `4.22-x86-vs-multi-arm` (base: 4.22, sample: 4.22)
  - Original: `base_release: {release: "4.21", relative_start: "now-7d", relative_end: "now"}`
  - Updated: `base_release: {release: "4.22", relative_start: "now-7d", relative_end: "now"}`
  - Note: No 'ga' to replace in this case

### Example 2: No arguments

Command: `/sippy-generate-release-views`

Will prompt:
1. "Which release should we copy views from? (e.g., 4.21)"
2. "Which release should we create views for? (e.g., 4.22)"
