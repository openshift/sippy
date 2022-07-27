## What is this

rejected-payload is a python script used to categorize the reject-reason column of the release_tags table of the Sippy DB.
reject-reason column is used to indicate, for a particular rejected payload, why it was rejected.
This is intended to be used by the watcher as he monitors the status of each payload.
The goal is to provide us some statistics of payload rejection history.

Here is the list of known categories:

```
* "TEST_FLAKE",            # Payload was rejected because of flake tests
* "CLOUD_INFRA",           # Payload was rejected because of infrastructure failures from cloud providers
* "RH_INFRA",              # Payload was rejected because of other infrastructure failures
* "PRODUCT_REGRESSION",    # Payload was rejected because of real product regression
* "TEST_REGRESSION"        # Payload was rejected because of a test regression (typically from origin repo)
```

## Python Dependencies

Use pip to install the following dependencies:

`pip install argparse sqlalchemy`

Or if you are on Fedora:

`sudo dnf install python3-sqlalchemy+postgresql`

## Key Functions

### Categorize

Categorize is used to update the reject-reason in the DB.
It can be used to update a single release tag when -t or --release_tag is specified.
If no release_tag is specified, it will find all uncategorized release tags and provide an interactive prompt to update each release tag.

```
  -t RELEASE_TAG, --release_tag RELEASE_TAG     Specifies a release payload tag, like 4.11.0-0.nightly-2022-06-25-081133
  -r RELEASE, --release RELEASE                 Specifies a release, like 4.11
  -s STREAM, --stream STREAM                    Specifies a stream, like nightly or ci
  -a, --all                                     List all rejected payloads. If not specified , list only uncategorized ones.
```

To list most recent failed payloads in a stream, and select one to categorize interactively:

```
% python ./rejected-payloads.py -d "your-dsn" categorize -s ci -r 4.12
index     release tag                                       phase               reject reason
1         4.11.0-0.ci-2022-06-29-121424                     Rejected            None
2         4.11.0-0.ci-2022-06-28-211909                     Rejected            None
Select tag between 1 and 2 to categorize, enter q to exit: 1
Please choose the reject reason for tag 4.11.0-0.ci-2022-06-29-121424 from the following list:
         1:           TEST_FLAKE
         2:          CLOUD_INFRA
         3:             RH_INFRA
         4:   PRODUCT_REGRESSION
         5:      TEST_REGRESSION
Enter your selection between 1 and 5: 3
index     release tag                                       phase               reject reason
1         4.11.0-0.ci-2022-06-28-211909                     Rejected            None
Select tag between 1 and 1 to categorize, enter q to exit: q
```

If you already know the payload tag you'd like to categorize:

```
% python ./rejected-payloads.py -d "your-dsn" categorize -t 4.11.0-0.nightly-2022-06-28-111405
Please choose the reject reason for tag 4.11.0-0.nightly-2022-06-28-111405 from the following list:
         1:           TEST_FLAKE
         2:          CLOUD_INFRA
         3:             RH_INFRA
         4:   PRODUCT_REGRESSION
         5:      TEST_REGRESSION
Enter your selection between 1 and 5: 2
```

### List

List command can be used to list uncategorized release tags.
Additional options can be provided to limit the scope of the query.

```
  -r RELEASE, --release RELEASE    Specifies a release, like 4.11
  -s STREAM, --stream STREAM       Specifies a stream, like nightly or ci
  -a, --all                        List all rejected payloads. If not specified , list only uncategorized ones.
```

The following example lists all uncategorized release tags for 4.11 ci payloads:

```
% python ./rejected-payloads.py -d "your-dsn" list -s ci -r 4.11
index     release tag                                       phase               reject reason
1         4.11.0-0.ci-2022-06-29-121424                     Rejected            None
2         4.11.0-0.ci-2022-06-28-211909                     Rejected            None
```
