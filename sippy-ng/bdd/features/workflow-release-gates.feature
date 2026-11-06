@workflows
Feature: Release health and feature gates link to diagnostic evidence
  Background:
    Given deterministic Sippy workflow evidence

  Scenario: Release summary links to the selected install and upgrade evidence
    When I open the synthetic release overview
    Then the release indicators show fixture-backed health values
    When I follow the install summary
    Then install values and infrastructure links match the selected release
    When I open the synthetic upgrade summary
    Then the upgrade test links to the exact test analysis

  Scenario Outline: Empty release health requests are understandable
    Given the "<endpoint>" workflow response is "empty"
    When I open workflow page "<page>"
    Then I see the workflow empty state "<message>"
    Examples:
      | endpoint     | page                  | message          |
      | /api/health  | release/5.0           | No data found    |
      | /api/install | install/5.0/operators | No Results.      |
      | /api/upgrade | upgrade/5.0/operators | No Results.      |

  Scenario Outline: Failed release requests do not appear healthy
    Given the "<endpoint>" workflow response is "error"
    When I open workflow page "<page>"
    Then I see a workflow error containing "<message>"
    Examples:
      | endpoint     | page                  | message                           |
      | /api/health  | release/5.0           | Could not retrieve release        |
      | /api/install | install/5.0/operators | Could not retrieve install        |
      | /api/upgrade | upgrade/5.0/operators | Could not retrieve release        |

  Scenario: A selected feature gate retains release and promotion context
    When I filter and select the synthetic feature gate
    Then its details and promotion view retain the selected gate and release
