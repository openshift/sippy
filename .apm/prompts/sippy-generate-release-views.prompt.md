---
description: "Generate new component readiness views for a new release"
input: [source-release, target-release]
---

# Generate Release Views

> **IMPORTANT**: When base_release becomes the sample release (not yet GA), replace 'ga' with 'now' in `relative_start` and `relative_end`

This command generates new component readiness views for a new release by copying and updating existing views from a previous release.

## Arguments (all optional)

- `[source-release]`: Source release version to copy views from (e.g., `4.21`)
- `[target-release]`: Target release version to create views for (e.g., `4.22`)

If any arguments are not provided, you will prompt the user interactively.

## Workflow

1. **Prompt for Source Release**: Ask the user for the source release version (e.g., `4.21`)
   - Format must be X.Y (e.g., 4.21, 4.20)

2. **Prompt for Target Release**: Ask the user for the target release version (e.g., `4.22`)
   - Format must be X.Y (e.g., 4.22, 4.23)

3. **Preview Changes**: Run the script in preview mode:
   - Execute: `python3 scripts/generate_release_views.py <source_release> <target_release>`
   - Ask for confirmation before proceeding

4. **Apply Changes**: If confirmed, run with --apply flag:
   - Execute: `python3 scripts/generate_release_views.py <source_release> <target_release> --apply`
   - The script will:
     - Read `config/views.yaml`
     - Find all views where `sample_release.release` equals the source release
     - Create new views with updated releases and add them to the TOP of the views list
     - **Name**: Replace source release with target release
     - **Sample Release**: Update to target release
     - **Base Release**:
       - If base = sample (same-release comparison), both become target
       - If base != sample (cross-release comparison), increment base by one minor version
       - When base_release becomes the sample release (not yet GA), replace 'ga' with 'now' in `relative_start` and `relative_end`

5. **Verify Output**: Show a diff of the changes made to views.yaml

6. **Run Validation Test**:
   - Run: `go test -v -run TestProductionViewsConfiguration ./pkg/flags/`
   - If the test fails, the views.yaml has errors that must be fixed before committing

7. **Offer to Commit**: Ask the user if they want to commit the changes (warn if on main/master).

## View Update Logic

When copying a view from source release to target release:

1. **Name Update**: Replace all occurrences of source release in the name
2. **Sample Release Update**: Always set to target release
3. **Base Release Update**: Depends on the original relationship
   - **Same-release views** (base = sample): Both become target
   - **Cross-release views** (base != sample): Base increments by one
     - When base_release becomes the sample release (not yet GA), replace 'ga' with 'now' in `relative_start` and `relative_end`

## Examples

### Creating 4.22 views from 4.21

- `4.21-main` (base: 4.20, sample: 4.21) -> `4.22-main` (base: 4.21, sample: 4.22)
  - `relative_start: "ga-30d"` -> `relative_start: "now-30d"` (because base now references 4.21)
- `4.21-x86-vs-multi-arm` (base: 4.21, sample: 4.21) -> `4.22-x86-vs-multi-arm` (base: 4.22, sample: 4.22)

## Important Notes

- The script preserves all other view settings (variant_options, advanced_options, etc.)
- Release versions must be in format X.Y (e.g., 4.21)
- YAML formatting should be preserved using proper YAML libraries
- Always verify the diff before committing
