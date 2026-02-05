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
   - Find all views where `base_release.release` equals the GA release (for date updates)
   - Find all views where `sample_release.release` equals the GA release AND have `automate_jira.enabled: true` (for JIRA automation disabling)
   - Show the user:
     - Number of views that will be affected by each type of change
     - List of view names that will be updated
     - Example of the changes (before/after for relative dates and automate_jira)
   - Ask for confirmation before proceeding

3. **Apply Updates**: For each affected view, perform two types of updates:

   **A. Update base_release dates** (for views comparing newer releases to the GA release):
   - Find views where `base_release.release` equals the GA release
   - Replace 'now' with 'ga' in `relative_start` (e.g., `now-30d` → `ga-30d`)
   - Replace 'now' with 'ga' in `relative_end` (e.g., `now` → `ga`)
   - **Rationale**: When a release goes GA, we want to reference the GA date as a stable point, not the current date

   **B. Disable automate_jira** (for views of the GA release itself):
   - Find views where `sample_release.release` equals the GA release
   - If `automate_jira.enabled` is set to `true`, change it to `false`
   - **Rationale**: When a release goes GA, automated JIRA ticket creation should stop for that release, as the focus shifts to newer releases

   - **IMPORTANT**: Preserve YAML formatting (double quotes, `{ }` spacing, indentation)

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
   Update views for GA release <release>

   Changed 'now' to 'ga' in base_release relative dates for views
   referencing release <release>, which just went GA.

   Also disabled automate_jira for views of release <release>, as
   automated JIRA ticket creation should focus on newer releases.

   Base release date updates: <count> views
   <list of view names, one per line>

   Automate JIRA disabled: <count> views
   <list of view names, one per line>

   🤖 Generated with [Claude Code](https://claude.com/claude-code)

   Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
   ```

## Implementation Details

### Update Logic

When a release goes GA, two types of updates should be applied:

**Type A: Update base_release dates (for views comparing newer releases to the GA release)**

1. **Find Views**: Identify all views where `base_release.release` equals the GA release
2. **Update Relative Dates**:
   - `relative_start: "now-30d"` → `relative_start: "ga-30d"`
   - `relative_start: "now-1d"` → `relative_start: "ga-1d"`
   - `relative_end: "now"` → `relative_end: "ga"`

**Type B: Disable automate_jira (for views of the GA release itself)**

1. **Find Views**: Identify all views where `sample_release.release` equals the GA release AND `automate_jira.enabled: true`
2. **Update automate_jira**:
   - `automate_jira:\n    enabled: true` → `automate_jira:\n    enabled: false`

**General Rule**: Preserve all other fields in the view

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

- This command affects two types of views:
  - Views where `base_release.release` equals the GA release (date updates)
  - Views where `sample_release.release` equals the GA release with automate_jira enabled (JIRA automation disabling)
- Sample release dates are not modified (they typically already use 'now')
- The script preserves all other view settings
- Always verify the diff to ensure only expected changes were made
- YAML formatting should be preserved using the Edit tool for precise replacements
- This is typically run once when a release goes GA (e.g., at release day)

## Examples

### Example 1: Updating when 4.21 goes GA

Command: `/sippy-update-ga-release-views 4.21`

This will make two types of updates:

**A. Base release date updates** (views with `base_release.release: "4.21"`):
- `4.22-main` view:
  - Before: `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
  - After: `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`
- `4.22-hypershift-candidates` view:
  - Before: `base_release: {release: "4.21", relative_start: "now-30d", relative_end: "now"}`
  - After: `base_release: {release: "4.21", relative_start: "ga-30d", relative_end: "ga"}`

**B. Automate JIRA disabling** (views with `sample_release.release: "4.21"` and `automate_jira.enabled: true`):
- `4.21-main` view:
  - Before: `automate_jira:\n    enabled: true`
  - After: `automate_jira:\n    enabled: false`

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
