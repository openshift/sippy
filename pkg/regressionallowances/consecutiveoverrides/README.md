An intentional regression for the same test, in two consecutive releases, requires special
approval. 

In this scenario you can add an override to overrides.json in this directory:

```json
{
  "4.18": {
    "openshift-tests:mytestid": true
  }
}
```

The above example indicates that this test ID was regressed in both 4.17 and 4.18.

After this change is added to your PR please reach out to deads@redhat.com or deads2k on slack
to discuss approvals for the change in this directory.
