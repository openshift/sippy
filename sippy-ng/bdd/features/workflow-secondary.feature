@workflows
Feature: Cluster, repository, pull request and chat navigation
  Background:
    Given deterministic Sippy workflow evidence

  Scenario: Cluster overview opens evidence for the selected cluster
    When I select the synthetic build cluster from the overview
    Then the cluster health and periodic runs match the selected cluster

  Scenario Outline: Empty and failed cluster health differ from healthy results
    Given the "/api/health/build_cluster" workflow response is "<mode>"
    And the "/api/health/build_cluster/analysis" workflow response is "<mode>"
    When I open workflow page "build_clusters"
    Then cluster health is "<mode>" without healthy fixture rows
    Examples:
      | mode  |
      | empty |
      | error |

  Scenario: Repository search and detail evidence preserve release and repository
    When I search for and select the synthetic repository
    Then repository detail requests and job links retain the repository filters
    When I follow repository jobs
    Then the jobs page shows only the selected repository evidence

  Scenario: Pull request filtering keeps the selected repository evidence
    When I filter pull requests to the selected number and repository
    Then the selected pull request links to its own GitHub and Prow history

  Scenario: Chat navigation presents the configured transition
    When I follow the chat navigation
    Then the Slack transition destination is configured without contacting Slack

  Scenario: Database capability controls local navigation
    Given the workflow has no local database capability
    When I open workflow page "chat"
    Then local database navigation is hidden

  Scenario Outline: Cluster detail distinguishes missing and failed run evidence
    Given the "/api/jobs/runs" workflow response is "<mode>"
    When I open workflow page "build_clusters/build-synthetic-a"
    Then the selected cluster run evidence is "<mode>"
    Examples:
      | mode  |
      | empty |
      | error |

  Scenario: Repository merged pull requests retain the selected repository
    When I search for and select the synthetic repository
    Then repository detail requests and job links retain the repository filters
    When I follow repository pull requests
    Then merged pull request evidence retains the repository and release
