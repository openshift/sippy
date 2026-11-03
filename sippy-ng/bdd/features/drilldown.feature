Feature: Component readiness drill-down
  As a Component Readiness maintainer
  I want to follow a regressed component down to its test
  So that the test details I open are the ones for that component and capability

  Scenario: Component-wide path reaches test details
    Given a component readiness API
    When I open the component readiness report
    And I open the component-wide drill-down
    Then I see capability "sdn"
    And the page keeps the generated view context
    When I open capability "sdn"
    Then I see test "test-sdn-pod-network"
    When I open test "test-sdn-pod-network"
    And I open the test details from the test row
    Then I am on the test details page for the fixture test
    And the test details request keeps the view context

  Scenario: Environment path keeps the variant identity
    Given a component readiness API
    When I open the component readiness report
    And I open the environment drill-down
    Then the environment heading keeps the variant identity
    And the page keeps the generated view context
    And the page keeps the environment variants
    And I see capability "sdn"
    When I open capability "sdn"
    And I open test "test-sdn-pod-network"
    And I open the test details from the test row
    Then I am on the test details page for the fixture test
    And the test details request keeps the view context

  Scenario: A test details deep link preserves identity
    Given a component readiness API
    When I open the test details deep link
    Then I am on the test details page for the fixture test
