# Sippy React Frontend

Sippy's frontend is written in React and [Material-UI](https://v4.mui.com/). This project was
bootstrapped with [Create React App](https://github.com/facebook/create-react-app).

In development, you can start Sippy as usual, and then run `npm start`
in the sippy-ng directory.  You can also just run `make build`, and use
the UI as embedded in the Sippy binary. This is how it's run in
production.

When developing on the UI, it's better to run the API and UI separately
for quicker feedback (Sippy will listen on `:8080`, and when run with
`npm start`, the UI defaults to `:3000`)

You can browse to your local UI at http://localhost:3000/sippy-ng.

If you are just working on the UI, you can have the UI connect to the production Sippy API server.
To do this, change the [sippy-ng/.env.development](.env.development) file to contain:

```
REACT_APP_API_URL="https://sippy.dptools.openshift.org"
```

If you are running tests and want to use the production Sippy API server, modify this line in [`setupTests.js`](src/setupTests.js):

```
process.env.REACT_APP_API_URL = 'http://localhost:8080'
```

to be:

```
process.env.REACT_APP_API_URL = 'https://sippy.dptools.openshift.org'
```
## Formattting requirements

Formatting requirements are enforce on the order of imports (alphabetically) and [prettier](https://prettier.io/docs/en/options.html).  The [prettier config](prettier.config.js) can be modified to change the formatting standards.  From the command line prettier formatting can be applied via
```
sippy/sippy-ng: $ npx prettier -w src/
```

For VScode, users install the [Prettier - Code formatter](https://marketplace.visualstudio.com/items?itemName=esbenp.prettier-vscode) plugin; when the you see errors tagged with `prettier/prettier`, use the CMD/CTRL + Shift + P keys
followed by "Format Document" to quickly fix them.

Imports must be sorted alphabetically.  If using a multi-line import (also sorted alphabetically) then the first entry in that import determines the sort order relative to the other import statements.
```
import { Link } from 'react-router-dom'
import { relativeTime, safeEncodeURIComponent } from '../helpers'
```
vs.
```
import { getReportStartDate, relativeTime, safeEncodeURIComponent } from '../helpers'
import { Link } from 'react-router-dom'
```

Helpers for each of these can be configured for on save actions or 'sort-imports'

## Installing dependencies

```bash
cd sippy-ng
npm install
```

## Testing

To run the tests run `npm test`.

For testing, we use [PollyJS](https://netflix.github.io/pollyjs) (to
record API responses) and [Jest](https://jestjs.io/).

Components that contact the Sippy API should have at a mimimum:

   1. A test that matches against a snapshot
   2. Checks some canary text exists
   3. Verifies the number of API calls being made (to detect React useEffect loops)

Non-API components should verify minimal functionality and also
take a snapshot.

The API recordings and snapshots are stored in `__recordings__/` and
`__snapshots__/` directories, respectively. Commit them to the repository
if they change.

**If you are prompted to update any snapshots, ensure the changes are
expected.**

### Managing time

When snapshotting pages that use date or time, the test must mock out
Date.now() to ensure consistent snapshots.  You can use jest's spy
functions:

```javascript
  Date.now = jest
    .spyOn(Date, 'now')
    .mockImplementation(() => new Date(1628691480000))
```

### Updating snapshots

When changing any of the UI, snapshots will need to be retaken. Run `npm
test` and *review the diff* to make sure any changes are expected. If
they are, `npm test -u` and commit the results.

### Updating API recordings

The API recordings were taken against the 4.8GA historical data, and
unless there's a specific reason to use something else, start sippy as
follows:

```
./sippy --server --local-data ./historical-data/common --release 4.8 --start-day=-1
```

And then run:

```
POLLY_MODE=record npm test
``````

For some reason (not yet determined), updating Polly recordings and the
snapshots doesn't work in the same run. First re-record the API results
(you'll get snapshot errors), then run the snapshot update as above.

If you need to move on to something other than 4.8GA, please update this
section.

## Available Scripts

In the project directory, you can run:

### `npm start`

Runs the app in the development mode.\
Open [http://localhost:3000](http://localhost:3000) to view it in the browser.

The page will reload if you make edits.\
You will also see any lint errors in the console.

### `npm test`

Launches the test runner in the interactive watch mode.\
See the section about [running tests](https://facebook.github.io/create-react-app/docs/running-tests) for more information.
