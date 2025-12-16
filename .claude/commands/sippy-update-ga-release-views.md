---
name: sippy-update-ga-release-views [release]
description: Update base_release relative dates when a release goes GA
allowedTools:
  - Bash
  - Read
  - Edit
  - TodoWrite
  - AskUserQuestion
---

# Update GA Release

This command updates component readiness views when a release goes GA by converting 'now' references to 'ga' in base_release relative dates.

## Arguments (optional)

- `[release]`: Release version that just went GA (e.g., `4.21`)

If the argument is not provided, you will prompt the user interactively.

## Workflow

**IMPORTANT: Argument Parsing**
Before starting the workflow, check if the user provided the release argument:

1. Parse the command invocation to extract the release argument if provided
2. If the release is provided:
   - Validate the release format (must be X.Y, e.g., 4.21)
   - Use the provided value directly
3. If the release is not provided, follow the full interactive workflow below

You will guide the user through the following steps (skipping steps where argument was provided):

1. **Prompt for GA Release**: Ask the user which release just went GA (e.g., `4.21`)
   - Format must be X.Y (e.g., 4.21, 4.20)
   - This is the release that transitioned from development to GA

2. **Find Affected Views**: Search config/views.yaml to identify affected views:
   - Read `config/views.yaml`
   - Find all views where `base_release.release` equals the GA release
   - Show the user:
     - Number of views that will be affected
     - List of view names that will be updated
     - Example of the changes (before/after for relative dates)
   - Ask for confirmation before proceeding

3. **Apply Updates**: For each affected view, update the base_release section:
   - Replace 'now' with 'ga' in `relative_start` (e.g., `now-30d` → `ga-30d`)
   - Replace 'now' with 'ga' in `relative_end` (e.g., `now` → `ga`)
   - **IMPORTANT**: Preserve YAML formatting (double quotes, `{ }` spacing, indentation)
   - Only modify the base_release relative dates for matching views
   - **Rationale**: When a release goes GA, we want to reference the GA date as a stable point, not the current date

4. **Verify Output**: Show a diff of the changes made to views.yaml

5. **Run Validation Test**: Execute the production views configuration test to verify the changes:
   - Run: `go test -v -run TestProductionViewsConfiguration ./pkg/flags/`
   - This validates the views.yaml structure and regression tracking constraints
   - If the test fails, the views.yaml has errors that must be fixed before committing

6. **Check Current Branch**: Before offering to commit, verify the current branch:
   - Run: `git branch --show-current`
   - If the current branch is `main` or `master`, skip the commit offer and warn the user:
     - "Changes have been made but not committed. You are on the main/master branch. Please create a feature branch before committing these changes."
   - If on any other branch, proceed to offer commit

7. **Offer to Commit**: Ask the user if they want to commit the changes. If yes, commit with the message:
   ```
   Update base_release relative dates for GA release <release>

   Changed 'now' to 'ga' in base_release relative dates for views
   referencing release <release>, which just went GA.

   Affected views: <count>
   <list of affected view names, one per line>

   🤖 Generated with [Claude Code](https://claude.com/claude-code)

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
   ```

## Implementation Details

### Update Logic

When a release goes GA, views that reference it in base_release should use 'ga' instead of 'now':

1. **Find Views**: Identify all views where `base_release.release` equals the GA release
2. **Update Relative Dates**:
   - `relative_start: "now-30d"` → `relative_start: "ga-30d"`
   - `relative_start: "now-1d"` → `relative_start: "ga-1d"`
   - `relative_end: "now"` → `relative_end: "ga"`
3. **Preserve Other Fields**: Don't modify any other fields in the view

### Implementation Approach

Use the Read and Edit tools to update views in place:

1. Read `config/views.yaml`
2. Find each view where `base_release.release` equals the GA release
3. For each match, use the Edit tool to replace:
   - `relative_start: now-30d` with `relative_start: ga-30d`
   - `relative_start: now-1d` with `relative_start: ga-1d`
   - `relative_end: now` with `relative_end: ga`
4. This approach preserves all YAML formatting (quotes, spacing, indentation)

**IMPORTANT**: Use the Edit tool for each change to preserve exact formatting. Do not use Python yaml.dump() as it may alter formatting (quotes, empty dict spacing, etc.).

## Important Notes

- This command only affects views where `base_release.release` equals the GA release
- Sample release dates are not modified (they typically already use 'now')
- The script preserves all other view settings
- Always verify the diff to ensure only expected changes were made
- YAML formatting should be preserved using proper YAML libraries
- This is typically run once when a release goes GA (e.g., at release day)

## Examples

### Example 1: Updating when 4.21 goes GA

Command: `/sippy-update-ga-release-views 4.21`

This will find all views with `base_release.release: "4.21"` and update:
- `4.22-main` view:
  - Before: `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
  - After: `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`
- `4.22-hypershift-candidates` view:
  - Before: `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
  - After: `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`

### Example 2: No arguments

Command: `/sippy-update-ga-release-views`

Will prompt: "Which release just went GA? (e.g., 4.21)"

## Use Case

This command is part of the release lifecycle workflow:

1. **Before GA**: New release (e.g., 4.22) is created with views comparing to previous release (4.21)
   - Views use `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
   - This references current data from 4.21 which is still in development

2. **When 4.21 goes GA**: Run `/sippy-update-ga-release-views 4.21`
   - Changes to `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`
   - Now references the stable GA date as the baseline

3. **When 4.22 goes GA**: Eventually run `/sippy-update-ga-release-views 4.22` for 4.23 views
