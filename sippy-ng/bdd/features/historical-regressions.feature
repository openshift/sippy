Feature: Historical test details for closed regressions
  As a developer investigating a resolved regression
  I want a link to the test results that led to it after it closes
  So that I do not have to rebuild a historical query by hand

  Closed regressions roll off the current report. The triage table keeps a
  link to a historical report that ends on the regression's last failure date.

  Scenario: A closed regression keeps a historical link after rolling off the report
    Given a component readiness API
    And a triage whose only regression closed exists
    When I open triage 7
    Then the "5.0-main" status column is shown
    And the closed regression links to its historical test details
    And the historical link can be reached from the keyboard

  Scenario: Open and closed regressions in one triage are shown separately
    Given a component readiness API
    And a triage with open and closed regressions exists
    When I open triage 7
    Then the open regression keeps its severity icon
    And the closed regression links to its historical test details
    And the regression closed only in this view says so
    And the regression without a last failure explains that history is unavailable

  Scenario: Sorting a view's status column orders open, historical, then unavailable rows
    Given a component readiness API
    And a triage with open and closed regressions exists
    When I open triage 7
    And I sort by the "5.0-main" status column
    Then the "5.0-main" status column is sorted ascending
    And the "5.0-main" rows are ordered open, historical, then unavailable
    When I sort by the "5.0-main" status column
    Then the "5.0-main" status column is sorted descending
    And the "5.0-main" rows start with unavailable history and end with the open row

  Scenario: A historical link identifies the closed regression on test details
    Given a component readiness API
    And the regression API returns the closed regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the closed regression notice shows its closure details
    And the test details assessment is a significant regression
    And the test details request keeps the historical window
    And the regression ID row shows the closed regression
    And the closed regression's triage is listed

  Scenario: Generating the report keeps the historical window and regression
    Given a component readiness API
    And the regression API returns the closed regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the sample date controls show the historical window
    When I generate the report from test details
    Then the page keeps the historical window and regression

  Scenario: A regression that does not match the report is not shown as its context
    Given a component readiness API
    And the regression API returns a regression for another test
    And test details no longer track the regression
    When I open the historical test details link
    Then the notice says the regression does not match this report

  Scenario: A regression closed only in this view is described as such
    Given a component readiness API
    And the regression API returns a regression closed only in this view
    And test details no longer track the regression
    When I open the historical test details link for the regression closed only in this view
    Then the notice says the regression closed only in this view

  Scenario: Without a view, closure of an open regression is not claimed
    Given a component readiness API
    And the regression API returns a regression closed only in this view
    And test details no longer track the regression
    When I open the historical test details link for the regression closed only in this view without a view
    Then the notice says closure in a specific view is unknown

  Scenario: A failed regression lookup is reported instead of claiming closure
    Given a component readiness API
    And the regression API fails
    And test details no longer track the regression
    When I open the historical test details link
    Then the notice says the regression could not be loaded

  Scenario: A missing regression is reported instead of claiming closure
    Given a component readiness API
    And the regression API cannot find the regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the notice says the regression was not found

  Scenario: Closure details remain when the historical window has no data
    Given test details are empty
    And the regression API returns the closed regression
    When I open the historical test details link
    Then the closed regression notice explains there is no historical data
