---
description: "Go backend coding standards for Sippy"
applyTo: "**/*.go"
---

* When adding or updating APIs, **use HATEOAS** in responses to support discoverability and consistent client interaction.
* Follow idiomatic Go practices.
* After making changes, always run `gofmt -w` on modified files to ensure proper formatting.
* When modifying any data provider (BigQuery or PostgreSQL), ensure **parity between both implementations**. Changes to query logic, filtering, or returned data in one provider must be reflected in the other.
