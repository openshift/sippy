Feature: Component readiness main report
  As a Component Readiness maintainer
  I want the main report to show the view and filters I generate
  So that I am looking at the regressions for the release I am checking

  Scenario: A named view shows regressed and healthy components
    Given a component readiness API
    When I open the component readiness report
    Then the regressed components tab shows 1
    And I see component "Networking"
    And the all components tab shows "Storage"

  Scenario: Regressed and healthy cells show the expected assessments
    Given a component readiness API
    When I open the component readiness report
    Then the "Networking" cell assessment is "SignificantRegression"
    When I open the all components tab
    Then the "Storage" cell assessment is "NotSignificant"
    And the "Networking" cell assessment is "SignificantRegression"

  Scenario: Column search hides environments that do not match
    Given a component readiness API
    When I open the component readiness report
    And I search columns for "gcp"
    Then the column "Platform:aws Architecture:amd64" is hidden
    And the report says there are no regressions
    When I clear the report search
    Then the column "Platform:aws Architecture:amd64" is shown
    And I see component "Networking"

  Scenario: Search and red only can be cleared without losing the regression
    Given a component readiness API
    When I open the component readiness report
    And I search rows for "zzz"
    Then the report says there are no regressions
    When I clear the report search
    Then I see component "Networking"
    When I show only red rows
    Then I see component "Networking"
    And I do not see component "Storage"

  Scenario: Generate report waits for the button and sends the edited release
    Given a component readiness API
    When I open the component readiness report
    And I change the sample release to "5.1" without generating
    Then I see component "Networking"
    And no report request includes "sampleRelease=5.1"
    When I generate the report
    Then the latest report request includes "sampleRelease=5.1"
    And the latest report request does not include "view="
    And I see component "Console"

  Scenario: Reloading a generated report keeps the sample release
    Given a component readiness API
    When I open the component readiness report
    And I change the sample release to "5.1" without generating
    And I generate the report
    And I reload the report
    Then the page URL includes "sampleRelease=5.1"
    And I see component "Console"

  Scenario: A pull request comparison is included in the generated query
    Given a component readiness API
    When I open the component readiness report
    And I set a sample pull request and generate
    Then the latest report request includes "samplePROrg=openshift"
    And the latest report request includes "samplePRRepo=sippy"
    And the latest report request includes "samplePRNumber=4130"

  Scenario: Payload tags are included in the generated query
    Given a component readiness API
    When I open the component readiness report
    And I set sample payload tags and generate
    Then the latest report request includes "samplePayloadTag=5.1.0-0.ci-2026-10-01-120000"

  Scenario: Group-by and advanced options are sent only after generate
    Given a component readiness API
    When I open the component readiness report
    And I drop the Platform group-by and generate
    Then the latest report request includes "columnGroupBy=Architecture"
    When I set advanced options and generate
    Then the latest report request includes "passRateNewTests=90"
    And the latest report request includes "flakeAsFailure=true"

  Scenario: Changing the basis release is sent only after generate
    Given a component readiness API
    When I open the component readiness report
    And I change the basis release to "5.1" without generating
    Then no report request includes "baseRelease=5.1"
    When I generate the report
    Then the latest report request includes "baseRelease=5.1"

  Scenario: Sample and basis dates are included in the generated query
    Given a component readiness API
    When I open the component readiness report
    And I set sample and basis dates without generating
    Then no report request includes "2026-01-15"
    And no report request includes "2026-03-01"
    When I generate the report
    Then the latest report request includes "sampleStartTime=2026-01-15"
    And the latest report request includes "baseStartTime=2026-03-01"

  Scenario: Variant selections are included in the generated query
    Given a component readiness API
    When I open the component readiness report
    And I include the gcp variant without generating
    Then no report request includes "includeVariant=Platform:gcp"
    When I generate the report
    Then the latest report request includes "includeVariant=Platform:gcp"

  Scenario: A lifecycle filter is sent with the generated report
    Given a component readiness API
    When I open the component readiness report
    And I filter by the blocking lifecycle without generating
    Then no report request includes "testLifecycles=blocking"
    When I generate the report
    Then the latest report request includes "testLifecycles=blocking"

  Scenario: Copy link restores search and red-only after open
    Given a component readiness API
    When I open the component readiness report
    And I search rows for "Networking"
    And I search columns for "aws"
    And I show only red rows
    And I copy the search link and open it
    Then the page URL includes "searchRow=Networking"
    And the page URL includes "searchColumn=aws"
    And the page URL includes "redOnly=1"
    When I open the all components tab
    Then I see component "Networking"
    And I do not see component "Storage"

  Scenario: A capability filter is sent with the generated report
    Given a component readiness API
    When I open the component readiness report
    And I filter by the sdn capability and generate
    Then the latest report request includes "testCapabilities=sdn"

  Scenario: An empty report is not shown as a healthy result
    Given the API returns an empty report
    When I open the component readiness report directly
    Then the report says no data was found

  Scenario: A failed report is shown as a failure
    Given the API returns an error for the report
    When I open the component readiness report directly
    Then the report shows a load failure

  Scenario: A cancelled report is shown as cancelled
    Given the API returns a cancelled report
    When I open the component readiness report directly
    Then the report says the operation was cancelled

  Scenario: A slow older report does not replace a newer selection
    Given a component readiness API
    And the current report is slow
    When I open the loading component readiness report
    And I change the sample release to "5.1" without generating
    And I generate the report
    Then I see component "Console"
    When I release the held report
    Then I still see the generated component

  Scenario: Read-only mode hides triage controls and sends no mutations
    Given a read-only component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    Then triage write controls are hidden
    And no mutation requests were sent
