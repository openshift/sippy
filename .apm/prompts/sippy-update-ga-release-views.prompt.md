---
description: "Update component readiness views when a release goes GA"
input: [release]
---

# Update GA Release Views

This command updates component readiness views when a release goes GA by converting 'now' references to 'ga' in base_release relative dates and reducing sensitivity parameters.

## Arguments (optional)

- `[release]`: Release version that just went GA (e.g., `4.21`)

If the argument is not provided, you will prompt the user interactively.

## Workflow

1. **Prompt for GA Release**: Ask the user which release just went GA (e.g., `4.21`)
   - Format must be X.Y (e.g., 4.21, 4.20)

2. **Find Affected Views**: Search config/views.yaml to identify affected views:
   - Views where `base_release.release` equals the GA release (for date updates)
   - Views where `sample_release.release` equals the GA release (for sensitivity reductions)
   - Show the user the affected views and ask for confirmation

3. **Apply Updates**: For each affected view:

   **A. Update base_release dates** (views comparing newer releases to the GA release):
   - Replace 'now' with 'ga' in `relative_start` (e.g., `now-30d` -> `ga-30d`)
   - Replace 'now' with 'ga' in `relative_end` (e.g., `now` -> `ga`)

   **B. Disable automate_jira** (views of the GA release itself):
   - Change `automate_jira.enabled` from `true` to `false`

   **C. Disable multi-release analysis**:
   - Change `include_multi_release_analysis` from `true` to `false`

   **D. Increase pity_factor**:
   - Set `pity_factor` to `10` (from 5)

   **E. Increase minimum_failure**:
   - Set `minimum_failure` to `4` (from 3)

   **F. Decrease pass_rate_required_new_tests**:
   - Set `pass_rate_required_new_tests` to `90` (from 95)

4. **Verify Output**: Show a diff of the changes made to views.yaml

5. **Run Validation Test**:
   - Run: `go test -v -run TestProductionViewsConfiguration ./pkg/flags/`

6. **Offer to Commit**: Ask the user if they want to commit the changes (warn if on main/master).

## Use Case

This command is part of the release lifecycle workflow:

1. **Before GA**: New release (e.g., 4.22) is created with views comparing to previous release (4.21)
   - Views use `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
2. **When 4.21 goes GA**: Run this command
   - Changes to `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`
   - Reduces sensitivity for the GA release's own views

## Important Notes

- This command affects two sets of views:
  - Views where `base_release.release` equals the GA release (date updates)
  - Views where `sample_release.release` equals the GA release (sensitivity reductions)
- Use the Edit tool for each change to preserve exact YAML formatting
- Always verify the diff to ensure only expected changes were made
- This is typically run once when a release goes GA

**IMPORTANT**: Preserve YAML formatting (double quotes, `{ }` spacing, indentation)
