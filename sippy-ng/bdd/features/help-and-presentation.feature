Feature: Help and presentation modes
  As a Component Readiness maintainer
  I want the FAQ one click from the report, and themes that keep status readable
  So that I can look up how to read a cell, and read it without relying on color

  Scenario: Help opens the FAQ and the breadcrumb returns to the report
    Given a component readiness API
    When I open the component readiness report
    And I open the component readiness help
    Then the component readiness FAQ is shown
    When I return to the report from the help breadcrumb
    Then I see component "Networking"
    And the page URL includes "component_readiness/main"

  Scenario: Dark mode changes the theme but not the status text
    Given a component readiness API
    And the browser prefers a light color scheme
    When I open the component readiness report
    Then the page uses the light theme
    When I open the component readiness help
    And I switch to dark mode
    Then the page uses the dark theme
    When I return to the report from the help breadcrumb
    Then the page uses the dark theme
    And the "Networking" cell assessment is "SignificantRegression"
    And the "Networking" cell tooltip says "-400: SignificantRegression detected"
    When I reload the report
    Then the page uses the dark theme

  Scenario: Accessibility mode swaps the status icon but keeps its text
    Given a component readiness API
    When I open the component readiness report
    Then the "Networking" cell uses the standard regression icon
    When I turn on accessibility mode
    Then the "Networking" cell uses the accessible regression icon
    And the "Networking" cell assessment is "SignificantRegression"
    And the "Networking" cell tooltip says "-400: SignificantRegression detected"
    And accessibility mode can be turned off
