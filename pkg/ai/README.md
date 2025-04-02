# Sippy AI

This package contains tools for working with Generative AI.  An example is provided 
with job runs, which uses a system prompt to prime the AI with instructions about how 
it should operate.  In this case, it's provided some data about a job run like it's
overall result and failed tests, and instructed to provide a summary.

The data provided to the AI is limited in that you wouldn't be able to provide a whole
build log, but you can provide relevant snippets.  For example, it would be a good 
idea to expand the job run data to include basic information about machines, nodes, and
cluster operators.

Sippy AI can be used with any OpenAI-compatible API. For local development, you can 
consider Ollama or vLLM.

If you need to specify an auth key, set the environment variable `OPENAI_API_KEY`.

## Example usage

Ollama example.  Install and launch Ollama:

```
ollama serve
```

Then simply launch Sippy, specifying the API endpoint and model.

```
./sippy serve --mode ocp \
    --listen-metrics="" \
    --log-level=debug  \
    --ai-endpoint=http://127.0.0.1:11434/v1/ \
    --ai-model mistral
```

### Fetching job run summary

```
$ curl localhost:8080/api/ai/job_run?prow_job_run_id=1904667807946117120
The CI job, "periodic-ci-openshift-hypershift-release-4.19-periodics-e2e-aws-ovn-conformance," failed due to various
test failures, including DNS resolution issues for Prometheus and Thanos services. This resulted in multiple test
cases failing, contributing to the overall job failure.
```

```
$ curl localhost:8080/api/ai/job_run?prow_job_run_id=1904793689171955712
The CI job 'periodic-ci-openshift-release-master-nightly-4.19-e2e-aws-ovn-fips' successfully
completed with no test failures.
```

## How else could I use this?

Some other ideas on how to use generative AI with Sippy:

- Generate a bug title/description from a failed test
- Examine all failed jobs on a payload, and ask the AI to find common themes
- Examine a single test failure, and ask the AI to provide debugging steps, and a summary of what went wrong

