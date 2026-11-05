Feature: Stable regression links
  As a Component Readiness maintainer
  I want a regression link to open that regression's test details
  So that links in Jira and chat land on the right test, view, and variants

  Scenario: A regression link opens test details for the main view
    Given a component readiness API
    When I open the stable link for regression 101
    Then the regression lookup was for regression 101
    And I am on the test details page for the fixture test
    And the page URL includes "view=5.0-main"
    And the page URL includes "sampleRelease=5.0"
    And the page URL includes "baseRelease=4.22"
    And the page URL query "includeVariant" is "Platform:aws"

  Scenario: A missing regression is reported
    Given a component readiness API
    And the regression does not exist
    When I open the stable link for regression 101
    Then the page shows "Failed to load regression: API server returned 404"
    And the page stays on the link for regression 101

  Scenario: A regression without a test details link is reported
    Given a component readiness API
    And the regression has no test details link
    When I open the stable link for regression 101
    Then the page shows "No test details link available for this regression."
    And the page stays on the link for regression 101

  Scenario: A link outside Component Readiness is refused
    Given a component readiness API
    And the regression links outside Component Readiness
    When I open the stable link for regression 101
    Then the page shows "Unexpected redirect path."
    And the page stays on the link for regression 101
