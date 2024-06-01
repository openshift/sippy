import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Grid,
  Typography,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { QuestionAnswer } from '@mui/icons-material'
import Breadcrumbs from '@mui/material/Breadcrumbs'
import React, { Fragment } from 'react'

const faqs = [
  {
    question: "How does Component Readiness determine there's a regression?",
    answer: `Component Readiness conducts a <a href="https://en.wikipedia.org/wiki/Fisher%27s_exact_test"><u>statistical test</u></a>
    to compare the sample release to the historical basis, and determines if there is a statistically significant difference in pass rates. Regressions are indicated
    by red squares, with the exploded red square indicating an extreme regression that is a 15% or more difference in pass rates.`,
  },
  {
    question: 'What if a test name changes?',
    answer: `If a test is renamed, it's still possible for component readiness to analyze the full history of the test under all names, if the rename is handled in the <a href="https://github.com/openshift-eng/ci-test-mapping/">
    <u>test mapping repository</u></a>. See the README in that repository for information and examples.`,
  },
  {
    question: 'Why are some of the green squares incomplete or dim?',
    answer: `This indicates that there is no history for at least one test during the selected time period. This can occur for a number of reasons, including that the test is new,
      the test was removed, or the test was renamed. Test renames or removals should be handled through the <a href="https://github.com/openshift-eng/ci-test-mapping/">
      <u>test mapping repository</u></a>.`,
  },
  {
    question: 'Why are basis runs sometimes missing when I click on a Test Details Report?',
    answer: `This most commonly happens for two reasons:
      <ul>
        <li>No job for that test existed the release you are comparing against.</li>
        <li>The job existed, but in a slightly different form.</li>
      </ul>
      That second option is the most common scenario.  Most of the time users only notice basis runs are missing because there is a problem they are trying to investigate.  If the job is totally new, it usually has clear ownership and if there are major problems it will simply be hidden from Component Readiness until it's ready.  Renames are slightly more common.  This is usually the result of a feature being deprecated and jobs shifting around accordingly.  In rare cases you may notice a regression on a Test Details Report that is only slightly over the default threshold&mdash;and it's the "new job" that is tipping it over the edge.  In that case you should still investigate the problems in the failing job.  It's also wise to look in <a href="https://github.com/openshift/release">the release repository</a> to find out more background on when the job was introduced and why.`,
  },
  {
    question:
      "How do I change a test's assignment to a particular component or capability?",
    answer: `Test mappings can be handled through the <a href="https://github.com/openshift-eng/ci-test-mapping/">
      <u>test mapping repository</u></a>.`,
  },
  {
    question: 'How do I report a bug or feature request?',
    answer: `Feature requests or bugs can be reported through <a href="https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?priority=10200&pid=12323832&issuetype=17&description=Describe%20your%20feature%20request%20or%20bug%3A%0A%0A%0A%20%20%20%20%0A%20%20%20%20Relevant%20Sippy%20URL%3A%0A%0A%20%20%20%20http%3A%2F%2Flocalhost%3A3000%2Fsippy-ng%2Fcomponent_readiness%2Fhelp%0A%0A"><u>Jira</u></a>.`,
  },
  {
    question: 'How much data history does Component Readiness keep?',
    answer: `The BigQuery backing data stores are set to retain information for approximately two years, however the
    earliest data in these tables is from April 2023.`,
  },
]

const style = {
  bgColor: 'background.paper',
  boxShadow: 24,
  p: 4,
}

export default function ComponentReadinessHelp(props) {
  return (
    <Fragment>
      <Grid className="component-readiness-help-dialog">
        <Breadcrumbs aria-label="help-breadcrumbs">
          <Link to="/component_readiness/main">Component Readiness</Link>
          <Typography>Help</Typography>
        </Breadcrumbs>
        <Typography sx={{ marginBottom: 5, textAlign: 'center' }} variant="h4">
          Frequently Asked Questions
        </Typography>
        {faqs.map((faq, index) => (
          <Accordion expanded key={index}>
            <AccordionSummary>
              <Typography sx={{ fontWeight: 'bold' }}>
                <QuestionAnswer /> {faq.question}
              </Typography>
            </AccordionSummary>
            <AccordionDetails>
              <Typography dangerouslySetInnerHTML={{ __html: faq.answer }} />
            </AccordionDetails>
          </Accordion>
        ))}
      </Grid>
    </Fragment>
  )
}
