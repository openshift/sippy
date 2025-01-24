# Notes:
# Rscript <filename> will run this.  It only uses the standard library.

# To detect problems faster can:
# * We can use smaller priors so new data has a stronger influence.
# * Calculate posterior predictive distributions to detect deviations from what we expect.

# This is where we mix in the historical and recent data.
posterior_params <- function(alpha_prior, beta_prior, k_recent, n_recent) {
  alpha_posterior <- alpha_prior + k_recent
  beta_posterior <- beta_prior + n_recent
  return(list(alpha = alpha_posterior, beta = beta_posterior))
}

posterior_predictive <- function(alpha_posterior, beta_posterior, k, n) {
  prob <- dnbinom(x = k, size = alpha_posterior, prob = beta_posterior / (beta_posterior + n))
  return(prob)
}

analyze_scenario <- function(alpha_prior, beta_prior, k_recent, n_recent, k_pr, n_pr) {
  posterior <- posterior_params(alpha_prior, beta_prior, k_recent, n_recent)
  prob <- posterior_predictive(posterior$alpha, posterior$beta, k_pr, n_pr)

  cat("--- Results ---\n")
  cat("Posterior Mean Failure Rate:", posterior$alpha / posterior$beta, "\n")
  cat("Posterior Predictive Probability of", k_pr, "failures in", n_pr, "tests:", prob, "\n\n")
}

#alpha_prior = Historical failures
#beta_prior = Historical total tests
#k_recent = Recent failures
#n_recent = Recent total tests
#k_pr = Pull request failures
#n_pr = Pull request total tests
print("Significant historical, limited mixed results in PR, possible on-going issue")
analyze_scenario(
  alpha_prior = 10,
  beta_prior = 1000,
  k_recent = 7,
  n_recent = 27,
  k_pr = 1,
  n_pr = 2
)

print("Limited historical, limited mixed results in PR")
analyze_scenario(
  alpha_prior = 1,
  beta_prior = 30,
  k_recent = 0,
  n_recent = 5,
  k_pr = 1,
  n_pr = 2
)

print("Limited historical, unlikely regression in PR")
analyze_scenario(
  alpha_prior = 1,
  beta_prior = 30,
  k_recent = 0,
  n_recent = 20,
  k_pr = 1,
  n_pr = 10
)

print("Limited historical, obvious regression in PR")
analyze_scenario(
  alpha_prior = 1,
  beta_prior = 30,
  k_recent = 0,
  n_recent = 20,
  k_pr = 10,
  n_pr = 15
)

print("Strong high pass rate historical data, but this test is failing outside our PR in recent runs")
analyze_scenario(
  alpha_prior = 0,
  beta_prior = 1000,
  k_recent = 20,
  n_recent = 30,
  k_pr = 1,
  n_pr = 3
)

# https://sippy.dptools.openshift.org/sippy-ng/component_readiness/test_details?Aggregation=none&Architecture=amd64&Architecture=amd64&FeatureSet=default&FeatureSet=default&Installer=ipi&Installer=ipi&LayeredProduct=none&Network=ovn&Network=ovn&NetworkAccess=default&Platform=gcp&Platform=gcp&Procedure=none&Scheduler=default&SecurityMode=default&Suite=unknown&Suite=unknown&Topology=ha&Topology=ha&Upgrade=none&Upgrade=none&baseEndTime=2024-12-27%2023%3A59%3A59&baseRelease=4.18&baseStartTime=2025-01-17%2000%3A00%3A00&capability=Other&columnGroupBy=Architecture%2CNetwork%2CPlatform%2CTopology&component=Installer%20%2F%20openshift-installer&confidence=95&dbGroupBy=Platform%2CArchitecture%2CNetwork%2CTopology%2CFeatureSet%2CUpgrade%2CSuite%2CInstaller&environment=amd64%20default%20ipi%20ovn%20gcp%20unknown%20ha%20none&flakeAsFailure=0&ignoreDisruption=1&ignoreMissing=0&includeMultiReleaseAnalysis=1&includeVariant=Architecture%3Aamd64&includeVariant=CGroupMode%3Av2&includeVariant=ContainerRuntime%3Acrun&includeVariant=ContainerRuntime%3Arunc&includeVariant=FeatureSet%3Adefault&includeVariant=Installer%3Aipi&includeVariant=Installer%3Aupi&includeVariant=Network%3Aovn&includeVariant=Owner%3Aeng&includeVariant=Platform%3Aaws&includeVariant=Platform%3Aazure&includeVariant=Platform%3Agcp&includeVariant=Platform%3Ametal&includeVariant=Platform%3Avsphere&includeVariant=Topology%3Aha&includeVariant=Topology%3Amicroshift&minFail=3&passRateAllTests=0&passRateNewTests=95&pity=5&sampleEndTime=2025-01-24%2023%3A59%3A59&samplePRNumber=&samplePROrg=&samplePRRepo=&sampleRelease=4.18&sampleStartTime=2025-01-17%2000%3A00%3A00&testBasisRelease=4.17&testId=cluster%20install%3A3e14279ba2c202608dd9a041e5023c4c&testName=install%20should%20succeed%3A%20infrastructure&view=
print("Slight Regression found from CR recently and then 3 failures out of 10 in a PR.")
analyze_scenario(
  alpha_prior = 0,
  beta_prior = 418,
  k_recent = 8,
  n_recent = 106,
  k_pr = 3,
  n_pr = 10
)
