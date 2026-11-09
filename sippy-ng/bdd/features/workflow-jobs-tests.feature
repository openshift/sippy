@workflows
Feature: Jobs and tests retain selected diagnostic evidence
  Background:
    Given deterministic Sippy workflow evidence

  Scenario: Search, sort, paginate and analyze all jobs
    When I browse all jobs and change the result ordering and page
    Then the jobs request and visible rows reflect my choices
    When I search for the synthetic job and open its analysis
    Then job analysis requests exactly the selected job
    When I open matching job runs
    Then the selected run links to its own Prow evidence

  Scenario: Permafailing jobs exclude healthy near matches
    When I open the Permafailing jobs tab
    Then both reporting periods require zero passing results

  Scenario: All job runs support pagination, sorting and filtering
    When I browse all job runs and change page and sort
    Then run requests contain the requested pagination and sort
    When I search and filter job runs to failures
    Then only the failed selected run and its Prow link remain

  Scenario Outline: Exact URL-sensitive test selection preserves variant and period
    When I select the URL-sensitive test from "<tab>"
    Then every analysis request identifies the exact test and variants
    When I reload the selected test analysis
    Then every analysis request identifies the exact test and variants
    When I open failed runs for the selected test
    Then run evidence retains test, variant, period and failure filters
    Examples:
      | tab              |
      | All tests        |
      | Tests by variant |
