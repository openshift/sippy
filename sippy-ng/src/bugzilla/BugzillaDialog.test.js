import { act } from '@testing-library/react'
import { mount } from 'enzyme'
import { withoutMuiID } from '../setupTests'
import BugzillaDialog from './BugzillaDialog'
import React from 'react'

const item = {
  name: '[sig-arch][Late] operators should not create watch channels very often [Suite:openshift/conformance/parallel]',
  bugs: [
    {
      id: 1979966,
      status: 'NEW',
      last_change_time: '2021-08-02T18:09:41Z',
      summary:
        'workers-rhel7 job is permanently failing [periodic-ci-openshift-release-master-nightly-4.8-e2e-aws-workers-rhel7]',
      affects_versions: ['4.11', '4.12'],
      fix_versions: ['4.13.0'],
      components: ['Installer'],
      url: 'https://bugzilla.redhat.com/show_bug.cgi?id=1979966',
    },
  ],
}

describe(BugzillaDialog, () => {
  let wrapper

  beforeEach(async () => {
    await act(async () => {
      wrapper = mount(
        <BugzillaDialog
          release="4.8"
          isOpen={true}
          close={() => {}}
          item={item}
        />
      )
    })
    wrapper.update()
  })

  it('shows the name of the test', async () => {
    expect(wrapper.find('h5').text()).toContain(item.name)
  })

  it('shows linked bugs', async () => {
    expect(
      wrapper
        .find('a[href="https://bugzilla.redhat.com/show_bug.cgi?id=1979966"]')
        .exists()
    ).toBeTruthy()
  })

  it('has a link to open a new bug', async () => {
    expect(
      wrapper.find('a[href*="bugzilla.redhat.com"] > .MuiButton-label').text()
    ).toContain('Open a new bug')
  })
})
