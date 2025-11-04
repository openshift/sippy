---
name: sippy-update-job-variant [job-substring] [category] [value]
description: Interactively update variants for a CI job
allowedTools:
  - Bash
  - Read
  - Edit
  - TodoWrite
  - AskUserQuestion
---

# Update Job Variant

This command provides an interactive workflow to update variant assignments for CI jobs by modifying the variant registry Go code.

## Arguments (all optional)

- `[job-pattern]`: Pattern (substring or full job name) that identifies the CI job(s) to update (e.g., `-hypershift-`, `-metal-ipi-`, or full job name like `periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn`)
- `[category]`: Variant category to update (e.g., Platform, Architecture, Topology)
- `[value]`: New value to set for the variant category

If any arguments are not provided, you will prompt the user interactively.

## Workflow

**IMPORTANT: Argument Parsing**
Before starting the workflow, check if the user provided any arguments:

1. Parse the command invocation to extract any provided arguments
2. If all three arguments (job-pattern, category, value) are provided:
   - Validate the category exists in the snapshot file
   - Skip the corresponding prompting steps
   - Use the provided values directly
3. If some arguments are missing, prompt only for the missing ones
4. If no arguments are provided, follow the full interactive workflow below

You will guide the user through the following steps (skipping steps where arguments were provided):

1. **Prompt for a Job Pattern**: Ask the user to enter a pattern that identifies the CI job(s) they want to update. This can be either a full job name or a substring. This will be used to add pattern matching logic to the Go code.
   - Example full job name: `periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn`
   - Example substrings: `-hypershift-`, `-metal-ipi-`, `-fips-`
   - **Important**: Both full job names and substrings are acceptable. Accept whatever the user provides without asking for clarification.
   - Advise the user that more specific patterns avoid unintended matches (e.g., `-metal-ipi-ovn-ipv6-` instead of just `-metal-`)

2. **Select Variant Category**: Extract variant categories programmatically and present as a numbered list:
   - Run: `grep -E "^    [A-Z]" pkg/variantregistry/snapshot.yaml | cut -d: -f1 | sed 's/^    //' | sort -u`
   - This extracts all unique variant categories from the snapshot file
   - Present the list with numbers (1, 2, 3, etc.) and ask the user to select by number
   - Categories typically include: Platform, Architecture, Network, NetworkStack, Topology, Installer, SecurityMode, FeatureSet, Owner, JobTier, Suite, Upgrade, NetworkAccess, Aggregation, ContainerRuntime, CGroupMode, OS, Scheduler, LayeredProduct, Procedure, and Release-related variants

3. **Select New Value**: Based on the chosen category, extract possible values programmatically and present as a numbered list:
   - For most categories, extract values from the snapshot: `grep "^    <CategoryName>:" pkg/variantregistry/snapshot.yaml | cut -d: -f2 | sed 's/^ //' | sed 's/"//g' | sort -u`
   - Present the unique values as a numbered list and ask the user to select by number
   - For Release-related variants (Release, FromRelease, FromReleaseMajor, FromReleaseMinor, ReleaseMajor, ReleaseMinor), allow free-text input instead of showing a numbered list

4. **Modify the Go Code**: Update `pkg/variantregistry/ocp.go` to add the pattern matching logic:
   - Find the appropriate setter function for the selected variant category (e.g., `setPlatform`, `setArchitecture`, `setTopology`, etc.)
   - Add an entry to the pattern matching logic in that function with the pattern and new value
   - Follow the existing code style and pattern structure (see examples in the file)
   - **CRITICAL**: Pay special attention to pattern ordering! The functions use early return, so the first matching pattern wins.
     - More specific patterns MUST come before more generic patterns
     - Example: In `setPlatform`, "-rosa" must come before "-aws" because ROSA jobs contain "aws"
     - Example: In `setOwner`, "-perfscale" must come before "-qe" because perfscale jobs may contain "qe"
   - Before adding the new pattern, analyze existing patterns in the function to determine the correct insertion point
   - Check if the new pattern might overlap with existing patterns and ensure correct precedence
   - If uncertain about ordering, examine the existing pattern list for similar examples (e.g., how "-rosa" is placed before "-aws")

5. **Preview Changes with Test**: Run the variant snapshot test to see what will change:
   - Execute: `go test -v -run TestVariantsSnapshot ./pkg/variantregistry 2>&1 | grep -A 200 "Summary of changes:"`
   - The test output shows a nicely formatted summary with:
     - Each unique change type (e.g., "Changed JobTier (candidate -> blocking)")
     - List of all affected jobs under each change type
   - Parse this output to extract:
     - The variant category changed
     - Old value → new value
     - All affected job names
     - Total count of affected jobs
   - Display the summary to the user with the format from the test output

6. **Apply Changes**: Execute `make update-variants` to regenerate `pkg/variantregistry/snapshot.yaml`:
   - Run: `make update-variants`
   - This applies the changes previewed in step 5
   - No need to parse output - we already showed the summary from the test

7. **Verify Unintended Changes** (optional manual step):
   - Suggest the user review `git diff pkg/variantregistry/snapshot.yaml` to ensure only expected jobs changed
   - Look for jobs that don't match the pattern - this could indicate incorrect pattern ordering
   - If unintended jobs were affected, the pattern placement is likely wrong and needs to be adjusted

8. **Offer to Commit**: Ask the user if they want to commit the changes. If yes, commit both the Go code and the regenerated snapshot with the message:
   ```
   Update <VariantCategory> variant for jobs matching "<pattern>"

   Set <VariantCategory> to "<new-value>" for jobs containing "<pattern>"

   Affected jobs: <count>

   <list of affected job names with their changes, one per line>

   🤖 Generated with [Claude Code](https://claude.com/claude-code)

   Co-Authored-By: Claude <noreply@anthropic.com>
   ```

## Important Notes

- The variant logic is defined in `pkg/variantregistry/ocp.go` in setter functions
- Each variant category has its own setter function with a pattern matching list
- The `make update-variants` command runs `./sippy variants snapshot --config ./config/openshift.yaml`
- This regenerates `pkg/variantregistry/snapshot.yaml` based on the Go code logic
- Always commit both the Go code changes AND the regenerated snapshot.yaml
- Use `git diff` to see exactly which jobs were affected
- Pattern matching is done with `strings.Contains(jobNameLower, pattern)`
- The TestVariantsSnapshot test should have `logrus.SetLevel(logrus.ErrorLevel)` at the start to reduce output noise

### Pattern Ordering is CRITICAL
- **The setter functions use early return - the FIRST matching pattern wins**
- More specific patterns MUST appear before more generic patterns
- If a pattern overlaps with an existing one, carefully consider the order
- Common examples to learn from:
  - `-rosa` before `-aws` (ROSA jobs contain "aws")
  - `-azure-aro-hcp` before `-azure` (ARO jobs contain "azure")
  - `-osd-ccs-gcp` before `-gcp` (OSD GCP jobs contain "gcp")
  - `-perfscale` before `-qe` (perfscale jobs may contain "qe")
- Always verify the diff doesn't show unintended changes to other jobs

## Implementation Notes & Helper Scripts

### Parsing Command Arguments

When the command is invoked, check the message content for arguments after `/sippy-update-job-variant`:
- Arguments are space-separated
- Extract them in order: [job-pattern] [category] [value]
- Example: `/sippy-update-job-variant -metal-ipi- Platform metal` provides all three arguments
- Example: `/sippy-update-job-variant periodic-ci-openshift-hypershift-release-4.16-periodics-e2e-aws-ovn` provides only job-pattern, prompt for category and value
- Example: `/sippy-update-job-variant` provides no arguments, prompt for all three
- Note: Full job names and substrings are both acceptable as job-pattern

**Validation when arguments are provided:**
1. For category: verify it exists by running the category extraction command
2. For value: if not a Release-related variant, verify the value exists for that category
3. If validation fails, notify the user and ask them to provide a correct value

### Extracting Variant Categories
Use this command to get all available variant categories:
```bash
grep -E "^    [A-Z]" pkg/variantregistry/snapshot.yaml | cut -d: -f1 | sed 's/^    //' | sort -u
```

### Extracting Values for a Specific Category
Replace `<CategoryName>` with the actual category (e.g., Platform, Architecture):
```bash
grep "^    <CategoryName>:" pkg/variantregistry/snapshot.yaml | cut -d: -f2 | sed 's/^ //' | sed 's/"//g' | sort -u
```

Example for Platform values:
```bash
grep "^    Platform:" pkg/variantregistry/snapshot.yaml | cut -d: -f2 | sed 's/^ //' | sed 's/"//g' | sort -u
```

### Previewing Changes with Test Output
Before running `make update-variants`, preview the changes using the test:
```bash
go test -v -run TestVariantsSnapshot ./pkg/variantregistry 2>&1 | grep -A 200 "Summary of changes:"
```

This provides a nicely formatted summary showing:
- Change type (e.g., "Changed JobTier (candidate -> blocking)")
- Complete list of affected jobs grouped by change type
- Much cleaner output than parsing make or git diff

### Finding Setter Functions
To see all setter functions and their variant categories:
```bash
grep -E "^func set[A-Z]" pkg/variantregistry/ocp.go | sed 's/func set//' | sed 's/(.*//' | sort
```

This will show functions like: `Aggregation`, `Architecture`, `Platform`, `Topology`, etc.
