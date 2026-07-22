# Sippy React Frontend

Sippy's frontend is written in React and [Material-UI](https://v4.mui.com/). The
dev server and production build are powered by [Vite](https://vite.dev/).

In development, you can start Sippy as usual, and then run `npm start`
in the sippy-ng directory. You can also just run `make build`, and use
the UI as embedded in the Sippy binary. This is how it's run in
production.

When developing on the UI, it's better to run the API and UI separately
for quicker feedback (Sippy will listen on `:8080`, and the Vite dev
server defaults to `:3000`)

You can browse to your local UI at http://localhost:3000/sippy-ng.

If you are just working on the UI, you can have the UI connect to the production Sippy API server.
To do this, change the [sippy-ng/.env.development](.env.development) file to contain:

```
VITE_API_URL="https://sippy.dptools.openshift.org"
```

This also works as a runtime setting:

```
sippy/sippy-ng: $ VITE_API_URL="https://sippy.dptools.openshift.org" npm start
```

If you are running tests and want to use the production Sippy API server, modify this line in [`setupTests.jsx`](src/setupTests.jsx):

```
import.meta.env.VITE_API_URL = 'http://localhost:8080'
```

to be:

```
import.meta.env.VITE_API_URL = 'https://sippy.dptools.openshift.org'
```

## Formatting requirements

Formatting requirements are enforced on the order of imports (alphabetically) and [prettier](https://prettier.io/docs/en/options.html). The [prettier config](prettier.config.js) can be modified to change the formatting standards. From the command line prettier formatting can be applied via

```
sippy/sippy-ng: $ npx prettier -w src/
```

For VScode, users install the [Prettier - Code formatter](https://marketplace.visualstudio.com/items?itemName=esbenp.prettier-vscode) plugin; when the you see errors tagged with `prettier/prettier`, use the CMD/CTRL + Shift + P keys
followed by "Format Document" to quickly fix them.

Imports must be sorted alphabetically. If using a multi-line import (also sorted alphabetically) then the first entry in that import determines the sort order relative to the other import statements.

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

We recommend disabling scripts globally for npm as this is a common
attack vector, and not needed for any sippy dependencies:

To install the deps in the node_modules directory:

```bash
cd sippy-ng
npm install --ignore-scripts
```

## Dependency vulnerabilities

To see if any of our NPM dependencies have an active CVE, run:

```
npm audit --production
```

This command is also run by `lint`, to make sure we don't ignore CVE's.

All audit commands should be given the flags `--production` or
`--omit=dev`. We only audit production, as that's what we deploy.

In the ideal case, the only thing you'll need to run is

```
npm audit fix --production
```

Unfortunately it's somewhat common that you'll end up with challenges in
dependency management. `npm` will tell you the problematic packages,
like in the example below:

```
 cookie  <0.7.0
cookie accepts cookie name, path, and domain with out of bounds characters - https://github.com/advisories/GHSA-pxg6-pf52-xh8x
fix available via `npm audit fix --force`
Will install react-cookie@1.0.5, which is a breaking change
node_modules/cookie
  universal-cookie  *
  Depends on vulnerable versions of cookie
  node_modules/react-cookie/node_modules/universal-cookie
    react-cookie  >=2.0.1
    Depends on vulnerable versions of universal-cookie
    node_modules/react-cookie
3 low severity vulnerabilities
To address all issues (including breaking changes), run:
  npm audit fix --force
```

This output is telling us:

- cookie less than 0.70 is vulernable to the linked CVE

- we have at least one package that won't let us upgrade to 0.7.x

- we can't fix it cleanly, but `--force` could by rolling back to react-cookie 1.0.5

You could try to force the fix with `npm audit fix --force --production`, and
review the results but it's usually not successful. In this case it
wasn't, downgrading from react-cookie 6.x to 1.0.5 is not the appropriate
way to go. It's a red herring that it's trying to suggest such a path,
most likely because 1.0.5 has loose depenedencies. You should avoid
`--force`.

You can ask npm for more details about how we are requiring cookie:

```
$ npm why cookie
cookie@0.6.0
node_modules/cookie
  cookie@"^0.6.0" from universal-cookie@7.2.0
  node_modules/universal-cookie
    universal-cookie@"^7.0.0" from react-cookie@7.2.0
    node_modules/react-cookie
      react-cookie@"^7.2.0" from the root project
```

universal-cookie requires `^0.6.0`, but the fix for our CVE is in 0.7.0.
You can look at their source repo to see if they have an issue open for
the CVE. Likely, it's just a matter of waiting a few days for a fixed
release. If you figure out which package needs to get updated, you can
file an issue if there isn't one already to try to expedite the process.

If the particular problematic package isn't going to be fixed -- most
commonly because we're on an older version and changes won't be
backported, we need to update and deal with the breaking changes. In
this case universal-cookie 7.2.0 is the latest major release, so it
was just a matter of having them allow cookie 0.7.x.

## Testing

To run the tests run `npm test`.

Tests use [Vitest](https://vitest.dev/) (Vite's native test runner) and
[React Testing Library](https://testing-library.com/docs/react-testing-library/intro/).

### Managing time

When testing pages that use date or time, the test must mock out
Date.now() to ensure consistent results. You can use Vitest's spy
functions:

```javascript
vi.spyOn(Date, 'now').mockImplementation(() => new Date(1628691480000))
```

## Available Scripts

In the project directory, you can run:

### `npm start`

Starts the Vite dev server on [http://localhost:3000](http://localhost:3000).
The page will reload if you make edits via hot module replacement (HMR).

### `npm run build`

Builds the app for production into the `build/` folder.

### `npm test`

Runs the Vitest test suite.
