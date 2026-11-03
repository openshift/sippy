Feature: Test details report
  As a Component Readiness maintainer
  I want this test's assessment, runs, and history on the test details report
  So that I can tell a real regression from a passing or missing result before I triage it

  These scenarios cover the test details report only. They do not start a triage.
  Triage from this page, from the regressed tests dialog, and from the triage detail page are separate.

  Scenario: Test details show stats, assessment, lifecycle, and history
    Given a component readiness API
    When I open the test details deep link
    Then the test details show the sample and basis statistics
    And the test details assessment is a significant regression
    And the informing lifecycle chip is shown
    And the historical release tabs are shown
    And the stale report warning links to the latest report
    And the failed run link refers to the failed run
    When I show only failed runs
    Then the failed run link refers to the failed run
    And the passing run link is hidden

  Scenario: Historical basis tabs show the matching comparison
    Given a component readiness API
    When I open the test details deep link
    And I open the "4.22 Basis" historical tab
    Then the historical comparison shows basis release "4.22"
    And the header assessment stays a significant regression

  Scenario: The stale warning opens the latest report
    Given a component readiness API
    When I open the test details deep link
    Then the latest report link opens that report

  Scenario: Empty test details are not shown as an assessment
    Given test details are empty
    When I open the test details deep link
    Then test details say no data was found

  Scenario: Failed test details are shown as a failure
    Given test details fail
    When I open the test details deep link
    Then test details show a load failure
