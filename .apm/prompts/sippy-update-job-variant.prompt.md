---
description: "Interactively update variants for a CI job"
input: [job-pattern, category, value]
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

3. **Select New Value**: Based on the chosen category, extract possible values programmatically and present as a numbered list:
   - For most categories, extract values from the snapshot: `grep "^    <CategoryName>:" pkg/variantregistry/snapshot.yaml | cut -d: -f2 | sed 's/^ //' | sed 's/"//g' | sort -u`
   - Present the unique values as a numbered list and ask the user to select by number
   - For Release-related variants (Release, FromRelease, FromReleaseMajor, FromReleaseMinor, ReleaseMajor, ReleaseMinor), allow free-text input instead of showing a numbered list

4. **Modify the Go Code**: Update `pkg/variantregistry/ocp.go` to add the pattern matching logic:
   - Find the appropriate setter function for the selected variant category (e.g., `setPlatform`, `setArchitecture`, `setTopology`, etc.)
   - Add an entry to the pattern matching logic in that function with the pattern and new value
   - Follow the existing code style and pattern structure (see examples in the file)
   - Pay special attention to pattern ordering! The functions use early return, so the first matching pattern wins.
     - More specific patterns come before more generic patterns
     - Example: In `setPlatform`, "-rosa" must come before "-aws" because ROSA jobs contain "aws"
     - Example: In `setOwner`, "-perfscale" must come before "-qe" because perfscale jobs may contain "qe"
   - Before adding the new pattern, analyze existing patterns in the function to determine the correct insertion point
   - Check if the new pattern might overlap with existing patterns and ensure correct precedence

5. **Preview Changes with Test**: Run the variant snapshot test to see what will change:
   - Execute: `go test -v -run TestVariantsSnapshot ./pkg/variantregistry 2>&1 | grep -A 200 "Summary of changes:"`
   - Parse this output to extract the variant category changed, old/new values, affected jobs, and total count
   - Display the summary to the user

6. **Apply Changes**: Execute `make update-variants` to regenerate `pkg/variantregistry/snapshot.yaml`

7. **Verify Unintended Changes** (optional manual step):
   - Suggest the user review `git diff pkg/variantregistry/snapshot.yaml` to ensure only expected jobs changed

8. **Offer to Commit**: Ask the user if they want to commit the changes. If yes, commit both the Go code and the regenerated snapshot.

## Important Notes

- The variant logic is defined in `pkg/variantregistry/ocp.go` in setter functions
- Each variant category has its own setter function with a pattern matching list
- The `make update-variants` command runs `./sippy variants snapshot --config ./config/openshift.yaml`
- This regenerates `pkg/variantregistry/snapshot.yaml` based on the Go code logic
- Always commit both the Go code changes AND the regenerated snapshot.yaml
- Pattern matching is done with `strings.Contains(jobNameLower, pattern)`

## Helper Commands

### Extracting Variant Categories
```bash
grep -E "^    [A-Z]" pkg/variantregistry/snapshot.yaml | cut -d: -f1 | sed 's/^    //' | sort -u
```

### Extracting Values for a Specific Category
```bash
grep "^    <CategoryName>:" pkg/variantregistry/snapshot.yaml | cut -d: -f2 | sed 's/^ //' | sed 's/"//g' | sort -u
```

### Previewing Changes
```bash
go test -v -run TestVariantsSnapshot ./pkg/variantregistry 2>&1 | grep -A 200 "Summary of changes:"
```

### Finding Setter Functions
```bash
grep -E "^func set[A-Z]" pkg/variantregistry/ocp.go | sed 's/func set//' | sed 's/(.*//' | sort
```

### Pattern Ordering is CRITICAL
- **The setter functions use early return - the FIRST matching pattern wins**
- More specific patterns MUST appear before more generic patterns
- Common examples to learn from:
  - `-rosa` before `-aws` (ROSA jobs contain "aws")
  - `-azure-aro-hcp` before `-azure` (ARO jobs contain "azure")
  - `-osd-ccs-gcp` before `-gcp` (OSD GCP jobs contain "gcp")
  - `-perfscale` before `-qe` (perfscale jobs may contain "qe")
- Always verify the diff doesn't show unintended changes to other jobs
