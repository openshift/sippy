Feature: Triage detail page
  As a Component Readiness maintainer
  I want to update, delete, or force-close the triage I opened
  So that the change applies to this record only, and only after I confirm it

  This is the triage detail page. It is not the regressed tests dialog on the main report, and it is not the triage form on the test details report.

  Scenario: Updating a triage keeps the other fields
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    And I update the triage description
    Then the updated triage description is shown

  Scenario: A triage cannot drop every regression
    Given a component readiness API
    And a triage with two regressions exists
    When I open triage 7
    And I try to remove every regression
    Then removing every regression is rejected

  Scenario: Removing a regression leaves the other member in place
    Given a component readiness API
    And a triage with two regressions exists
    When I open triage 7
    And I remove one regression
    Then one regression was removed

  Scenario: Delete requires confirmation
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    And I cancel deleting the triage
    Then the triage details stay open
    And no mutation requests were sent
    When I delete the triage
    Then the triage is deleted

  Scenario: A rejected delete does not claim the triage is gone
    Given a component readiness API
    And an unresolved triage exists
    And the next mutation is rejected
    When I open triage 7
    And I delete the triage
    Then the delete request failed
    And the triage details stay open

  Scenario: Force close requires a reason and targets the previewed regression
    Given a component readiness API
    And a resolved triage exists
    When I open triage 7
    And I open force close
    Then the force-close preview shows the fixture test
    When I force close without a reason
    Then a force-close reason is required
    And force close was not submitted
    When I force close with a reason
    Then force close succeeds

  Scenario: An empty force-close preview cannot be submitted
    Given a component readiness API
    And a resolved triage exists
    And the force-close preview is empty
    When I open triage 7
    And I open force close
    Then there is nothing to force close
    And force close was not submitted

  Scenario: Cancel leaves force close unsubmitted
    Given a component readiness API
    And a resolved triage exists
    When I open triage 7
    And I open force close
    And I cancel force close
    Then force close was not submitted

  Scenario: An unresolved triage cannot force close
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    Then force close is disabled until the triage is resolved

  Scenario: Read-only mode hides destructive triage controls
    Given a read-only component readiness API
    And an unresolved triage exists
    When I open triage 7
    Then delete and force close are hidden
    And no mutation requests were sent
